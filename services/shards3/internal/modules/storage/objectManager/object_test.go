package objectManager

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"shards3/services/shards3/internal/modules/storage/chunker"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

// testStorageDir mirrors the fixed relative directory used by the file-based
// backend implementation (interfaces.FileService).
const testStorageDir = "./testdata"

// setupTest wires config, a fresh temporary SQLite database, KMS and the
// metadata package so PutObject/GetObject/UpdateObject/DeleteObject can run
// through the full real pipeline (chunking, compression, encryption,
// sharding, metadata persistence).
func setupTest(t *testing.T) {
	t.Helper()

	if err := config.LoadConfig(); err != nil {
		t.Fatalf("config.LoadConfig() error: %v", err)
	}

	tempDir := t.TempDir()
	config.Cfg.SQLitePath = filepath.Join(tempDir, "objectmanager_test.db")
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
	metadata.Configure(database)

	if err := os.MkdirAll(testStorageDir, 0o755); err != nil {
		t.Fatalf("failed to create test storage directory: %v", err)
	}
}

// randomData generates deterministic, effectively incompressible data of the
// given size so that compressed chunk sizes stay close to the original size
// - this keeps chunk-boundary tests meaningful.
func randomData(seed int64, size int) []byte {
	data := make([]byte, size)
	rand.New(rand.NewSource(seed)).Read(data)
	return data
}

func collectShardLocations(chunks []chunker.Chunk) []string {
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

// TestObjectLifecycle_MultipleBackends exercises Put -> Get -> Update -> Get
// -> Delete across several file-backend topologies (varying backend count
// and failure tolerance), using interfaces.SetAvailableBackends to restrict
// to local file backends only.
func TestObjectLifecycle_MultipleBackends(t *testing.T) {
	const payloadSize = 2 * 1024 * 1024 // 2 MiB, well below the chunk size.

	backendConfigs := []struct {
		name      string
		count     int
		tolerance int
	}{
		{"SingleBackend", 1, 0},
		{"TwoBackends", 2, 1},
		{"FiveBackends", 5, 2},
	}

	for _, bc := range backendConfigs {
		t.Run(bc.name, func(t *testing.T) {
			setupTest(t)
			interfaces.SetAvailableBackends(interfaces.RegisterFileTestBackends(bc.count))
			config.Cfg.FailureTolerance = bc.tolerance

			location := object.ObjectLocation{Bucket: object.Bucket{Name: "lifecycle-bucket"}, Key: "lifecycle-" + bc.name}
			data := randomData(1, payloadSize)

			if _, err := PutObject(location, data); err != nil {
				t.Fatalf("PutObject() error: %v", err)
			}

			obj, err := GetObject(location)
			if err != nil {
				t.Fatalf("GetObject() error: %v", err)
			}
			if obj.Size != int64(len(data)) {
				t.Fatalf("Size mismatch: got=%d want=%d", obj.Size, len(data))
			}

			got, err := obj.GetData(0, 0)
			if err != nil {
				t.Fatalf("GetData() error: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("round-tripped data does not match original data")
			}

			updatedData := randomData(2, payloadSize/2)
			if _, err := UpdateObject(location, updatedData); err != nil {
				t.Fatalf("UpdateObject() error: %v", err)
			}

			updatedObj, err := GetObject(location)
			if err != nil {
				t.Fatalf("GetObject() after update error: %v", err)
			}
			if updatedObj.Size != int64(len(updatedData)) {
				t.Fatalf("Size after update mismatch: got=%d want=%d", updatedObj.Size, len(updatedData))
			}

			gotUpdated, err := updatedObj.GetData(0, 0)
			if err != nil {
				t.Fatalf("GetData() after update error: %v", err)
			}
			if !bytes.Equal(gotUpdated, updatedData) {
				t.Fatal("round-tripped data after update does not match updated data")
			}

			shardLocations := collectShardLocations(updatedObj.Chunks)

			if err := DeleteObject(location); err != nil {
				t.Fatalf("DeleteObject() error: %v", err)
			}
			if _, err := GetObject(location); err == nil {
				t.Fatal("expected error getting deleted object")
			}

			assertShardFilesRemoved(t, shardLocations)
		})
	}
}

// TestObjectLifecycle_ChunkBoundarySizes exercises the full object lifecycle
// with payload sizes around the chunk size boundary (config.Cfg.ChunkSize is
// 128 MiB by default), including sizes strictly between one and two chunks,
// to make sure multi-chunk objects are chunked, sharded, reconstructed and
// cleaned up correctly.
func TestObjectLifecycle_ChunkBoundarySizes(t *testing.T) {
	setupTest(t)

	backends := interfaces.RegisterFileTestBackends(3)
	interfaces.SetAvailableBackends(backends)
	config.Cfg.FailureTolerance = 1

	chunkSize := config.Cfg.ChunkSize

	sizeCases := []struct {
		name string
		size int
	}{
		{"JustUnderOneChunk", chunkSize - 1024},
		{"ExactlyOneChunk", chunkSize},
		{"JustOverOneChunk", chunkSize + 1024*1024},
		{"BetweenOneAndTwoChunks", chunkSize + chunkSize/2},
		{"ExactlyTwoChunks", chunkSize * 2},
	}

	for _, sc := range sizeCases {
		t.Run(sc.name, func(t *testing.T) {
			location := object.ObjectLocation{Bucket: object.Bucket{Name: "chunk-bucket"}, Key: "chunk-" + sc.name}
			data := randomData(int64(sc.size), sc.size)

			if _, err := PutObject(location, data); err != nil {
				t.Fatalf("PutObject() error: %v", err)
			}

			obj, err := GetObject(location)
			if err != nil {
				t.Fatalf("GetObject() error: %v", err)
			}
			if obj.Size != int64(len(data)) {
				t.Fatalf("Size mismatch: got=%d want=%d", obj.Size, len(data))
			}

			// Sizes clearly larger than a single chunk (allowing margin for
			// compression/encryption overhead) must produce multiple chunks.
			if sc.size > chunkSize+chunkSize/10 && len(obj.Chunks) < 2 {
				t.Fatalf("expected multiple chunks for payload size %d, got %d chunk(s)", sc.size, len(obj.Chunks))
			}

			got, err := obj.GetData(0, 0)
			if err != nil {
				t.Fatalf("GetData() error: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("round-tripped data does not match original data")
			}

			shardLocations := collectShardLocations(obj.Chunks)

			if err := DeleteObject(location); err != nil {
				t.Fatalf("DeleteObject() error: %v", err)
			}
			if _, err := GetObject(location); err == nil {
				t.Fatal("expected error getting deleted object")
			}

			assertShardFilesRemoved(t, shardLocations)
		})
	}
}

// TestObjectLifecycle_Streaming exercises PutObjectStream/UpdateObjectStream
// end-to-end via an io.Reader instead of a []byte, across a payload spanning
// multiple chunks, verifying the streamed byte count and full round-trip.
func TestObjectLifecycle_Streaming(t *testing.T) {
	setupTest(t)

	interfaces.SetAvailableBackends(interfaces.RegisterFileTestBackends(3))
	config.Cfg.FailureTolerance = 1
	config.Cfg.ChunkConcurrency = 4

	payloadSize := config.Cfg.ChunkSize + config.Cfg.ChunkSize/2 // 1.5 chunks
	data := randomData(42, payloadSize)

	location := object.ObjectLocation{Bucket: object.Bucket{Name: "stream-bucket"}, Key: "stream-object"}

	obj, err := PutObjectStream(location, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PutObjectStream() error: %v", err)
	}
	if obj.Size != int64(len(data)) {
		t.Fatalf("Size mismatch: got=%d want=%d", obj.Size, len(data))
	}
	if len(obj.Chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(obj.Chunks))
	}

	got, err := obj.GetData(0, 0)
	if err != nil {
		t.Fatalf("GetData() error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("round-tripped data does not match original data")
	}

	updatedData := randomData(43, payloadSize/2)
	updatedObj, err := UpdateObjectStream(location, bytes.NewReader(updatedData))
	if err != nil {
		t.Fatalf("UpdateObjectStream() error: %v", err)
	}
	if updatedObj.Size != int64(len(updatedData)) {
		t.Fatalf("Size after update mismatch: got=%d want=%d", updatedObj.Size, len(updatedData))
	}

	gotUpdated, err := updatedObj.GetData(0, 0)
	if err != nil {
		t.Fatalf("GetData() after update error: %v", err)
	}
	if !bytes.Equal(gotUpdated, updatedData) {
		t.Fatal("round-tripped data after update does not match updated data")
	}

	shardLocations := collectShardLocations(updatedObj.Chunks)

	if err := DeleteObject(location); err != nil {
		t.Fatalf("DeleteObject() error: %v", err)
	}

	assertShardFilesRemoved(t, shardLocations)
}

func TestObjectDownloadStreaming_RangeAndFull(t *testing.T) {
	setupTest(t)

	interfaces.SetAvailableBackends(interfaces.RegisterFileTestBackends(3))
	config.Cfg.FailureTolerance = 1
	config.Cfg.ChunkConcurrency = 4

	originalChunkSize := config.Cfg.ChunkSize
	config.Cfg.ChunkSize = 64 * 1024
	t.Cleanup(func() {
		config.Cfg.ChunkSize = originalChunkSize
	})

	payloadSize := 3*config.Cfg.ChunkSize + 123
	data := randomData(99, payloadSize)

	location := object.ObjectLocation{Bucket: object.Bucket{Name: "stream-range-bucket"}, Key: "stream-range-object"}

	if _, err := PutObjectStream(location, bytes.NewReader(data)); err != nil {
		t.Fatalf("PutObjectStream() error: %v", err)
	}

	obj, err := GetObject(location)
	if err != nil {
		t.Fatalf("GetObject() error: %v", err)
	}

	fullReader, err := obj.GetDataStream(0, 0)
	if err != nil {
		t.Fatalf("GetDataStream(full) error: %v", err)
	}
	defer fullReader.Close()

	fullData, err := io.ReadAll(fullReader)
	if err != nil {
		t.Fatalf("io.ReadAll(full stream) error: %v", err)
	}
	if !bytes.Equal(fullData, data) {
		t.Fatal("full streamed data does not match original")
	}

	start := int64(config.Cfg.ChunkSize / 2)
	end := int64(config.Cfg.ChunkSize*2 + 77)

	rangeReader, err := obj.GetDataStream(start, end)
	if err != nil {
		t.Fatalf("GetDataStream(range) error: %v", err)
	}
	defer rangeReader.Close()

	rangeData, err := io.ReadAll(rangeReader)
	if err != nil {
		t.Fatalf("io.ReadAll(range stream) error: %v", err)
	}
	expected := data[start:end]
	if !bytes.Equal(rangeData, expected) {
		t.Fatal("range streamed data does not match expected slice")
	}
}
