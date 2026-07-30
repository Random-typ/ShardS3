package chunker

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"shards3/services/shards3/internal/modules/storage/compression"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

// testStorageDir mirrors the fixed relative directory used by the
// file-based backend implementation (interfaces.FileService).
const testStorageDir = "./testdata"

// setupTest wires config, a fresh temporary SQLite database and KMS so
// ChunkStream/CollectChunks can run through the full real pipeline
// (compression, encryption, sharding) against the local file backend.
func setupTest(t *testing.T) {
	t.Helper()

	if err := config.LoadConfig(); err != nil {
		t.Fatalf("config.LoadConfig() error: %v", err)
	}

	tempDir := t.TempDir()
	config.Cfg.SQLitePath = filepath.Join(tempDir, "chunker_test.db")
	config.Cfg.SQLiteBusyTimeoutMS = 5000
	config.Cfg.SQLiteMaxOpenConns = 1
	config.Cfg.KMSPasswordKeyPath = filepath.Join(tempDir, "kms.key")

	database, err := db.New()
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
	})

	if err := encryption.ConfigureKMS(database); err != nil {
		t.Fatalf("encryption.ConfigureKMS() error: %v", err)
	}

	// Start from a clean testdata directory so glob-based assertions in
	// tests (e.g. "no shard files remain") aren't polluted by files left
	// over from earlier/interrupted test runs.
	if err := os.RemoveAll(testStorageDir); err != nil {
		t.Fatalf("failed to clean test storage directory: %v", err)
	}
	if err := os.MkdirAll(testStorageDir, 0o755); err != nil {
		t.Fatalf("failed to create test storage directory: %v", err)
	}
}

func randomData(seed int64, size int) []byte {
	data := make([]byte, size)
	rand.New(rand.NewSource(seed)).Read(data)
	return data
}

func shardLocations(chunks []Chunk) []string {
	var locations []string
	for _, c := range chunks {
		for _, s := range c.Shards {
			locations = append(locations, s.Location)
		}
	}
	return locations
}

func assertShardFilesRemoved(t *testing.T, locations []string) {
	t.Helper()
	for _, loc := range locations {
		path := filepath.Join(testStorageDir, loc)
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("expected shard file to be removed at %q, got stat error: %v", path, statErr)
		}
	}
}

// TestChunkStream_RoundTrip exercises ChunkStream -> CollectChunks across
// payload sizes around the chunk-size boundary (using a small overridden
// ChunkSize for test speed), including an empty object, to make sure
// streaming chunking/reassembly (each chunk independently
// compressed/encrypted) round-trips correctly and cleans up its shards.
func TestChunkStream_RoundTrip(t *testing.T) {
	setupTest(t)

	interfaces.SetAvailableBackends([]interfaces.BackendType{interfaces.File, interfaces.File2, interfaces.File3})
	config.Cfg.FailureTolerance = 1
	config.Cfg.ChunkSize = 64 * 1024

	compressionMeta := compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel}

	sizeCases := []struct {
		name       string
		size       int
		wantChunks int
	}{
		{"Empty", 0, 0},
		{"SubChunk", config.Cfg.ChunkSize / 2, 1},
		{"ExactlyOneChunk", config.Cfg.ChunkSize, 1},
		{"PartialFinalChunk", config.Cfg.ChunkSize*3 + config.Cfg.ChunkSize/2, 4},
		{"ExactMultiple", config.Cfg.ChunkSize * 3, 3},
	}

	for _, sc := range sizeCases {
		t.Run(sc.name, func(t *testing.T) {
			data := randomData(1, sc.size)

			chunks, totalRead, _, err := ChunkStream(bytes.NewReader(data), encryption.AES_256_GCM, interfaces.GetAvailableBackends(), 4)
			if err != nil {
				t.Fatalf("ChunkStream() error: %v", err)
			}
			if totalRead != int64(len(data)) {
				t.Fatalf("totalRead mismatch: got=%d want=%d", totalRead, len(data))
			}
			if len(chunks) != sc.wantChunks {
				t.Fatalf("chunk count mismatch: got=%d want=%d", len(chunks), sc.wantChunks)
			}

			got, err := CollectChunks(chunks, compressionMeta)
			if err != nil {
				t.Fatalf("CollectChunks() error: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("round-tripped data does not match original data")
			}

			locations := shardLocations(chunks)
			if err := DeleteChunks(chunks); err != nil {
				t.Fatalf("DeleteChunks() error: %v", err)
			}
			assertShardFilesRemoved(t, locations)
		})
	}
}

// TestChunkStream_ConcurrencyOrdering verifies that chunks are always
// reassembled in their original stream order and produce identical
// round-tripped data, regardless of how many chunks are processed
// concurrently - proving the ordinal-based reassembly is race-free.
func TestChunkStream_ConcurrencyOrdering(t *testing.T) {
	setupTest(t)

	interfaces.SetAvailableBackends([]interfaces.BackendType{interfaces.File, interfaces.File2})
	config.Cfg.FailureTolerance = 0
	config.Cfg.ChunkSize = 32 * 1024

	compressionMeta := compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel}
	data := randomData(7, config.Cfg.ChunkSize*6+123)

	for _, concurrency := range []int{1, 2, 8} {
		t.Run(fmt.Sprintf("Concurrency%d", concurrency), func(t *testing.T) {
			chunks, totalRead, _, err := ChunkStream(bytes.NewReader(data), encryption.None, interfaces.GetAvailableBackends(), concurrency)
			if err != nil {
				t.Fatalf("ChunkStream() error: %v", err)
			}
			if totalRead != int64(len(data)) {
				t.Fatalf("totalRead mismatch: got=%d want=%d", totalRead, len(data))
			}

			got, err := CollectChunks(chunks, compressionMeta)
			if err != nil {
				t.Fatalf("CollectChunks() error: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("concurrency=%d: round-tripped data does not match original", concurrency)
			}

			if err := DeleteChunks(chunks); err != nil {
				t.Fatalf("DeleteChunks() error: %v", err)
			}
		})
	}
}

// TestChunkStream_BackendFailurePropagatesError verifies that a failing
// backend causes ChunkStream to return an error (and no chunks), rather than
// silently producing an incomplete/corrupt result.
func TestChunkStream_BackendFailurePropagatesError(t *testing.T) {
	setupTest(t)

	interfaces.SetAvailableBackends([]interfaces.BackendType{interfaces.File1Fail})
	config.Cfg.FailureTolerance = 0
	config.Cfg.ChunkSize = 16 * 1024

	data := randomData(5, config.Cfg.ChunkSize)

	chunks, _, _, err := ChunkStream(bytes.NewReader(data), encryption.None, interfaces.GetAvailableBackends(), 2)
	if err == nil {
		t.Fatal("expected an error from ChunkStream when the only backend fails")
	}
	if chunks != nil {
		t.Fatalf("expected no chunks on failure, got %+v", chunks)
	}
}

// errAfterReader returns the bytes in data, then fails with err once data is
// exhausted - simulating a client/network read failure partway through an
// upload (e.g. a dropped connection), after some chunks have already been
// durably processed and uploaded.
type errAfterReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// TestChunkStream_ReaderErrorCleansUpCompletedChunks verifies that if
// reading the source data fails partway through a multi-chunk stream, any
// chunks that had already been fully processed and uploaded are cleaned up
// (their shards deleted) rather than left as orphaned data with nothing in
// the metadata store ever referencing them.
func TestChunkStream_ReaderErrorCleansUpCompletedChunks(t *testing.T) {
	setupTest(t)

	interfaces.SetAvailableBackends([]interfaces.BackendType{interfaces.File, interfaces.File2})
	config.Cfg.FailureTolerance = 0
	config.Cfg.ChunkSize = 16 * 1024

	const goodChunks = 3
	data := randomData(11, config.Cfg.ChunkSize*goodChunks)
	reader := &errAfterReader{data: data, err: errors.New("simulated read failure")}

	// concurrency=1 keeps chunk processing strictly ordered relative to
	// reading, so by the time the reader error is encountered all
	// `goodChunks` chunks are guaranteed to have already completed and had
	// their shards written to disk.
	chunks, _, _, err := ChunkStream(reader, encryption.None, interfaces.GetAvailableBackends(), 1)
	if err == nil {
		t.Fatal("expected an error from ChunkStream when the reader fails")
	}
	if chunks != nil {
		t.Fatalf("expected no chunks on failure, got %+v", chunks)
	}

	remaining, err := filepath.Glob(filepath.Join(testStorageDir, "shard_*"))
	if err != nil {
		t.Fatalf("Glob() error: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected all shard files to be cleaned up after the read failure, found %v", remaining)
	}
}
