package object

import (
	"shards3/services/shards3/internal/modules/storage/chunker"
	"shards3/services/shards3/internal/modules/storage/compression"
	"time"
)

type Bucket struct {
	Name      string
	CreatedAt time.Time
}
type ObjectLocation struct {
	Bucket Bucket
	Key    string
}

type BucketStats struct {
	Bucket       Bucket
	TotalObjects int64
	TotalSize    int64
}

type Object struct {
	Location ObjectLocation
	Size     int64

	Compression compression.Compression

	LastModified time.Time
	// Checksum. Always of type FULL_OBJECT, uses XXHASH64 algorithm.
	ETag uint64

	Chunks []chunker.Chunk
}

func (o Object) GetData() ([]byte, error) {
	data, err := chunker.CollectChunks(o.Chunks, o.Compression)
	if err != nil {
		return nil, err
	}
	return data, nil
}
