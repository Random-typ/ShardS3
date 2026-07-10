package object

import (
	"shards3/services/shards3/internal/modules/storage/chunker"
	"shards3/services/shards3/internal/modules/storage/compression"
	"time"
)

type Bucket string
type Key string

type ObjectLocation struct {
	Bucket Bucket
	Key    Key
}

type Object struct {
	Location ObjectLocation
	Size     int64

	Compression compression.Compression

	Created time.Time

	Chunks []chunker.Chunk
}

func (o Object) GetData() ([]byte, error) {
	data, err := chunker.CollectChunks(o.Chunks, o.Compression)
	if err != nil {
		return nil, err
	}
	return data, nil
}
