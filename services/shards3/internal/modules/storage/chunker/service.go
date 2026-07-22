package chunker

import (
	"shards3/services/shards3/internal/modules/storage/compression"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/shard"
	"shards3/services/shards3/internal/platform/config"

	"github.com/google/uuid"
)

type Encryption encryption.Encryption

type Chunk struct {
	Id   uuid.UUID
	Size int64

	EncodedShardSize  int
	EncodedDataShards int
	Encryption        Encryption

	Shards []shard.Shard
}

// Compresses, chunks, encrypts and then shards the data, returning the resulting chunks.
func ChunkData(data []byte, encryptionMethod encryption.EncryptionType, backends []interfaces.BackendType) ([]Chunk, error) {
	// Compress the data
	compressedData, err := compression.Compress(data, compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel})
	if err != nil {
		return nil, err
	}
	// Chunk the compressed data, encrypt each chunk and then shard the encrypted chunk
	var chunks []Chunk
	for i := 0; i < len(compressedData); i += config.Cfg.ChunkSize {
		end := i + config.Cfg.ChunkSize
		end = min(end, len(compressedData))

		// Encrypt the chunk
		chunkData := compressedData[i:end]
		encryptedData, keyId, err := encryption.Encrypt(chunkData, encryptionMethod)
		if err != nil {
			return nil, err
		}
		// Shard the encrypted chunk
		shards, shardCount, encodedShardSize, err := shard.ShardData(encryptedData, backends)
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, Chunk{
			Id:                uuid.New(),
			Size:              int64(len(encryptedData)),
			EncodedShardSize:  encodedShardSize,
			EncodedDataShards: shardCount,
			Encryption:        Encryption{Type: encryptionMethod, KeyId: keyId},
			Shards:            shards,
		})
	}

	return chunks, nil
}

func CollectChunks(chunks []Chunk, compressionMetadata compression.Compression) ([]byte, error) {
	var compressedData []byte
	for _, chunk := range chunks {
		// Collect the shards and reconstruct the encrypted chunk
		encryptedData, err := shard.CollectShards(chunk.Shards, chunk.EncodedShardSize, chunk.EncodedDataShards, int(chunk.Size/int64(chunk.EncodedShardSize)-int64(chunk.EncodedDataShards)))
		if err != nil {
			return nil, err
		}

		// Decrypt the chunk
		decryptedData, err := encryption.Decrypt(encryptedData, encryption.Encryption{
			Type:  chunk.Encryption.Type,
			KeyId: chunk.Encryption.KeyId,
		})
		if err != nil {
			return nil, err
		}

		compressedData = append(compressedData, decryptedData...)
	}

	// Decompress the data
	return compression.Decompress(compressedData, compressionMetadata)
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
