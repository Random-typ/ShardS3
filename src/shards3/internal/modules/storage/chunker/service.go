package chunker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"shards3/internal/modules/storage/compression"
	"shards3/internal/modules/storage/encryption"
	"shards3/internal/modules/storage/interfaces"
	"shards3/internal/modules/storage/shard"
	"shards3/internal/platform/config"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type Encryption encryption.Encryption

type Chunk struct {
	Id   uuid.UUID
	Size int64

	EncodedShardSize    int
	EncodedDataShards   int
	EncodedParityShards int
	Encryption          Encryption

	Shards []shard.Shard
}

// ChunkData compresses, chunks, encrypts and shards data, returning the
// resulting chunks. It is a convenience wrapper around ChunkStream for
// callers that already have the whole object in memory.
func ChunkData(data []byte, encryptionMethod encryption.EncryptionType, compression compression.Compression, backends []interfaces.BackendType) ([]Chunk, error) {
	chunks, _, _, err := ChunkStream(bytes.NewReader(data), encryptionMethod, compression, backends, 1)
	return chunks, err
}

// ChunkStream reads data from r, splitting it into config.Cfg.ChunkSize
// pieces. Each chunk is compressed independently as its own self-contained
// unit (rather than one continuous compression stream across the whole
// object), then encrypted and sharded. Up to `concurrency` chunks may be
// compressed/encrypted/sharded concurrently while the next chunk's raw bytes
// are still being read from r, bounding peak memory usage to roughly
// concurrency * config.Cfg.ChunkSize instead of requiring the whole object to
// be buffered in RAM up front.
//
// Chunks are returned in their original stream order regardless of the order
// in which concurrent workers finish. If reading from r fails, or any chunk
// fails to process, any chunks that had already completed successfully are
// cleaned up (their shards deleted) before the error is returned, so a failed
// upload doesn't leave orphaned shards behind.
func ChunkStream(r io.Reader, encryptionMethod encryption.EncryptionType, compression compression.Compression, backends []interfaces.BackendType, concurrency int) ([]Chunk, int64, uint64, error) {
	if concurrency < 1 {
		concurrency = 1
	}

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrency)

	var (
		mu        sync.Mutex
		completed = make(map[int]Chunk)
	)

	var (
		totalRead int64
		ordinal   int
		readErr   error
	)

	digest := xxhash.New()
	for ctx.Err() == nil {
		buf := make([]byte, config.Cfg.ChunkSize)
		n, err := io.ReadFull(r, buf)

		if n > 0 {
			buf = buf[:n]
			totalRead += int64(n)
			chunkOrdinal := ordinal
			ordinal++

			digest.Write(buf)

			g.Go(func() error {
				chunk, err := processChunk(buf, encryptionMethod, compression, backends)
				if err != nil {
					return fmt.Errorf("process chunk %d: %w", chunkOrdinal, err)
				}
				mu.Lock()
				completed[chunkOrdinal] = chunk
				mu.Unlock()
				return nil
			})
		}

		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		readErr = fmt.Errorf("read object data: %w", err)
		break
	}

	if err := g.Wait(); err != nil {
		cleanupChunks(completed)
		return nil, 0, 0, err
	}
	if readErr != nil {
		cleanupChunks(completed)
		return nil, 0, 0, readErr
	}

	chunks := make([]Chunk, ordinal)
	for i := 0; i < ordinal; i++ {
		chunks[i] = completed[i]
	}

	hash := digest.Sum64()
	return chunks, totalRead, hash, nil
}

// cleanupChunks best-effort deletes the shards of every already-completed
// chunk. Used when ChunkStream fails partway through, to avoid leaving
// orphaned shards on the storage backends for chunks that will never be
// referenced by any object.
func cleanupChunks(completed map[int]Chunk) {
	for _, chunk := range completed {
		_ = DeleteChunk(chunk)
	}
}

// processChunk compresses, encrypts and shards a single chunk of raw bytes.
func processChunk(data []byte, encryptionMethod encryption.EncryptionType, compressionMetadata compression.Compression, backends []interfaces.BackendType) (Chunk, error) {
	compressedData, err := compression.Compress(data, compressionMetadata)
	if err != nil {
		return Chunk{}, err
	}

	// Encrypt the chunk
	encryptedData, keyId, err := encryption.Encrypt(compressedData, encryptionMethod)
	if err != nil {
		return Chunk{}, err
	}

	// Shard the encrypted chunk
	shards, shardCount, encodedShardSize, err := shard.ShardData(encryptedData, backends)
	if err != nil {
		return Chunk{}, err
	}

	// Determine the parity shard count from the highest encoded shard
	// index actually produced, since ShardData does not return it directly.
	maxShardIndex := 0
	for _, s := range shards {
		if s.Last > maxShardIndex {
			maxShardIndex = s.Last
		}
	}
	parityShardCount := maxShardIndex + 1 - shardCount

	return Chunk{
		Id:                  uuid.New(),
		Size:                int64(len(encryptedData)),
		EncodedShardSize:    encodedShardSize,
		EncodedDataShards:   shardCount,
		EncodedParityShards: parityShardCount,
		Encryption:          Encryption{Type: encryptionMethod, KeyId: keyId},
		Shards:              shards,
	}, nil
}

func CollectChunks(chunks []Chunk, compressionMetadata compression.Compression) ([]byte, error) {
	var data []byte
	for _, chunk := range chunks {
		// Collect the shards and reconstruct the encrypted chunk
		encryptedData, err := shard.CollectShards(chunk.Shards, chunk.EncodedShardSize, chunk.EncodedDataShards, chunk.EncodedParityShards)
		if err != nil {
			return nil, err
		}

		// Reconstructed data is padded up to a multiple of EncodedShardSize;
		// trim it back down to the exact encrypted chunk length before
		// decrypting, otherwise the trailing padding breaks AEAD auth.
		if int64(len(encryptedData)) > chunk.Size {
			encryptedData = encryptedData[:chunk.Size]
		}

		// Decrypt the chunk
		decryptedData, err := encryption.Decrypt(encryptedData, encryption.Encryption{
			Type:  chunk.Encryption.Type,
			KeyId: chunk.Encryption.KeyId,
		})
		if err != nil {
			return nil, err
		}

		// Each chunk was compressed independently, so it must be decompressed
		// independently too - unlike encryption/sharding, compression state
		// does not span multiple chunks.
		decompressedChunk, err := compression.Decompress(decryptedData, compressionMetadata)
		if err != nil {
			return nil, err
		}

		data = append(data, decompressedChunk...)
	}

	return data, nil
}

// CollectChunksStream reconstructs chunks and streams their decompressed bytes
// in order through a reader, avoiding full-object buffering.
func CollectChunksStream(chunks []Chunk, compressionMetadata compression.Compression) io.ReadCloser {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		for _, chunk := range chunks {
			// Collect the shards and reconstruct the encrypted chunk.
			encryptedData, err := shard.CollectShards(chunk.Shards, chunk.EncodedShardSize, chunk.EncodedDataShards, chunk.EncodedParityShards)
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}

			// Reconstructed data is padded up to a multiple of EncodedShardSize;
			// trim it back down to the exact encrypted chunk length before
			// decrypting, otherwise the trailing padding breaks AEAD auth.
			if int64(len(encryptedData)) > chunk.Size {
				encryptedData = encryptedData[:chunk.Size]
			}

			// Decrypt the chunk.
			decryptedData, err := encryption.Decrypt(encryptedData, encryption.Encryption{
				Type:  chunk.Encryption.Type,
				KeyId: chunk.Encryption.KeyId,
			})
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}

			// Each chunk was compressed independently, so it must be decompressed
			// independently too.
			decompressedChunk, err := compression.Decompress(decryptedData, compressionMetadata)
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}

			if _, err := pw.Write(decompressedChunk); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}
	}()

	return pr
}

func DeleteChunk(chunk Chunk) error {
	return shard.DestroyShards(chunk.Shards)
}

func DeleteChunks(chunks []Chunk) error {
	for _, chunk := range chunks {
		err := DeleteChunk(chunk)
		if err != nil {
			return err
		}
	}
	return nil
}
