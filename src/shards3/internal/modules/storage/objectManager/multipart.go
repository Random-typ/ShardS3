package objectManager

import (
	"io"

	"shards3/internal/modules/storage/chunker"
	"shards3/internal/modules/storage/compression"
	"shards3/internal/modules/storage/encryption"
	"shards3/internal/modules/storage/interfaces"
	"shards3/internal/modules/storage/metadata"
	"shards3/internal/modules/storage/object"
	"shards3/internal/platform/config"
)

func CreateMultipartUpload(location object.ObjectLocation) (object.MultipartUpload, error) {
	compression := compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel}
	id, err := metadata.CreateMultipartUpload(location, compression)
	if err != nil {
		return object.MultipartUpload{}, err
	}
	return object.MultipartUpload{
		UploadID:    id,
		Location:    location,
		Compression: compression,
		Initiated:   metadata.GetCurrentTime(),
	}, nil
}

func UploadPartStream(location object.ObjectLocation, partNumber int, uploadId string, r io.Reader) (object.MultipartPart, error) {
	encType, err := encryption.EncryptionTypeFromString(config.Cfg.EncryptionMethod)
	compression := compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel}

	if err != nil {
		return object.MultipartPart{}, err
	}
	multipartUpload, err := metadata.GetMultipartUpload(uploadId)
	if err != nil {
		return object.MultipartPart{}, err
	}

	chunks, size, hash, err := chunker.ChunkStream(r, encType, compression, interfaces.GetAvailableBackends(), config.Cfg.ChunkConcurrency)
	if err != nil {
		return object.MultipartPart{}, err
	}

	multipart := object.MultipartPart{
		UploadID:   multipartUpload.UploadID,
		PartNumber: partNumber,
		ETag:       hash,
		Size:       size,
		CreatedAt:  metadata.GetCurrentTime(),
		Chunks:     chunks,
	}

	err = metadata.PutPart(multipart)
	if err != nil {
		return object.MultipartPart{}, err
	}
	return multipart, nil
}

func CompleteMultipartUpload(uploadId string) (object.Object, error) {
	_, err := metadata.GetMultipartUpload(uploadId)
	if err != nil {
		return object.Object{}, err
	}

	obj, err := metadata.CompleteMultipartUpload(uploadId)
	if err != nil {
		return object.Object{}, err
	}
	return obj, nil
}

func AbortMultipartUpload(uploadId string) error {
	return metadata.AbortMultipartUpload(uploadId)
}
