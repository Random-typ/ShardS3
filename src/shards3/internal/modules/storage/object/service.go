package object

import (
	"fmt"
	"io"
	"shards3/internal/modules/storage/chunker"
	"shards3/internal/modules/storage/compression"
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

func normalizeRange(total int64, begin int64, end int64) (int64, int64, error) {
	if begin < 0 {
		return 0, 0, fmt.Errorf("invalid range: begin must be >= 0")
	}
	if begin > total {
		return 0, 0, fmt.Errorf("invalid range: begin exceeds object size")
	}

	if end == 0 {
		end = total
	}

	if end < begin || end > total {
		return 0, 0, fmt.Errorf("invalid range: end out of bounds")
	}

	return begin, end, nil
}

func streamRangeFromChunks(chunks []chunker.Chunk, compressionMetadata compression.Compression, totalSize int64, begin int64, end int64) (io.ReadCloser, error) {
	begin, end, err := normalizeRange(totalSize, begin, end)
	if err != nil {
		return nil, err
	}

	base := chunker.CollectChunksStream(chunks, compressionMetadata)
	length := end - begin

	pr, pw := io.Pipe()
	go func() {
		defer base.Close()
		defer pw.Close()

		if begin > 0 {
			if _, err := io.CopyN(io.Discard, base, begin); err != nil {
				if err != io.EOF {
					_ = pw.CloseWithError(err)
				}
				return
			}
		}

		if length == 0 {
			return
		}

		if _, err := io.CopyN(pw, base, length); err != nil {
			if err != io.EOF {
				_ = pw.CloseWithError(err)
			}
			return
		}
	}()

	return pr, nil
}

func (o *Object) GetData(begin int64, end int64) ([]byte, error) {
	r, err := o.GetDataStream(begin, end)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

func (o *MultipartUpload) GetData(part MultipartPart, begin int64, end int64) ([]byte, error) {
	r, err := o.GetDataStream(part, begin, end)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

func (o *Object) GetDataStream(begin int64, end int64) (io.ReadCloser, error) {
	return streamRangeFromChunks(o.Chunks, o.Compression, o.Size, begin, end)
}

func (o *MultipartUpload) GetDataStream(part MultipartPart, begin int64, end int64) (io.ReadCloser, error) {
	return streamRangeFromChunks(part.Chunks, o.Compression, part.Size, begin, end)
}
