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

type BackendStats struct {
	Backend      string
	TotalShards  int64
	TotalBytes   int64
	TotalChunks  int64
	TotalObjects int64
	TotalBuckets int64
	LastVerified time.Time
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

// MultipartUpload is an in-progress upload that has not yet been assembled
// into an Object.
type MultipartUpload struct {
	UploadID    string
	Location    ObjectLocation
	Compression compression.Compression
	Initiated   time.Time
}

// MultipartPart is a single part of a MultipartUpload. ETag is the part's own
// XXHASH64 checksum, mirroring Object.ETag.
type MultipartPart struct {
	UploadID   string
	PartNumber int
	ETag       uint64
	Size       int64
	CreatedAt  time.Time

	Chunks []chunker.Chunk
}

// returns the first and last chunk indices that contain the requested range,
// as well as the offset within the first chunk and the length of the range to read from the last chunk.
func getChunkOffset(chunks []chunker.Chunk, begin int64, end int64) (int, int, int64, int64) {
	firstChunk := 0
	for _, chunk := range chunks {
		if begin >= chunk.Size {
			firstChunk++
			begin -= chunk.Size
			continue
		}
		break
	}
	lastChunk := len(chunks)
	for _, chunk := range chunks {
		if end > chunk.Size {
			lastChunk--
			end -= chunk.Size
			continue
		}
		break
	}
	return firstChunk, lastChunk, begin, end
}

func (o *Object) GetData(begin int64, end int64) ([]byte, error) {
	firstChunk, lastChunk, begin, end := getChunkOffset(o.Chunks, begin, end)

	data, err := chunker.CollectChunks(o.Chunks[firstChunk:lastChunk], o.Compression)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		end = int64(len(data))
	}
	return data[begin:end], nil
}

func (o *MultipartUpload) GetData(part MultipartPart, begin int64, end int64) ([]byte, error) {
	firstChunk, lastChunk, begin, end := getChunkOffset(part.Chunks, begin, end)

	data, err := chunker.CollectChunks(part.Chunks[firstChunk:lastChunk], o.Compression)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		end = int64(len(data))
	}
	return data[begin:end], nil
}
