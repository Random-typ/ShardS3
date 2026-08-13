package objectManager

import (
	"bytes"
	"io"

	"shards3/services/shards3/internal/modules/storage/chunker"
	"shards3/services/shards3/internal/modules/storage/compression"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/platform/config"
)

/* Object Manager
*
* Coordinates the lifecycle of objects.
*
 */

// PutObject stores data as a new object. It is a convenience wrapper around
// PutObjectStream for callers that already have the whole object in memory.
func PutObject(location object.ObjectLocation, data []byte) (object.Object, error) {
	return PutObjectStream(location, bytes.NewReader(data))
}

// PutObjectStream stores the data read from r as a new object, streaming it
// through the chunking pipeline instead of buffering the whole object in
// memory - see chunker.ChunkStream for details.
func PutObjectStream(location object.ObjectLocation, r io.Reader) (object.Object, error) {
	encType, err := encryption.EncryptionTypeFromString(config.Cfg.EncryptionMethod)
	compression := compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel}
	if err != nil {
		return object.Object{}, err
	}
	chunks, size, hash, err := chunker.ChunkStream(r, encType, compression, interfaces.GetAvailableBackends(), config.Cfg.ChunkConcurrency)
	if err != nil {
		return object.Object{}, err
	}

	var obj = object.Object{
		Location:     location,
		Size:         size,
		Compression:  compression,
		LastModified: metadata.GetCurrentTime(),
		ETag:         hash,
		Chunks:       chunks,
	}

	err = metadata.PutObject(obj)
	if err != nil {
		return object.Object{}, err
	}

	return obj, nil
}

func GetObject(location object.ObjectLocation) (object.Object, error) {
	obj, err := metadata.GetObject(location)
	if err != nil {
		return object.Object{}, err
	}
	return obj, nil
}

// UpdateObject replaces an existing object's data. It is a convenience
// wrapper around UpdateObjectStream for callers that already have the whole
// object in memory.
func UpdateObject(location object.ObjectLocation, data []byte) (object.Object, error) {
	return UpdateObjectStream(location, bytes.NewReader(data))
}

// UpdateObjectStream replaces an existing object's data with the data read
// from r, streaming it through the chunking pipeline instead of buffering
// the whole object in memory - see chunker.ChunkStream for details.
func UpdateObjectStream(location object.ObjectLocation, r io.Reader) (object.Object, error) {
	encType, err := encryption.EncryptionTypeFromString(config.Cfg.EncryptionMethod)
	compression := compression.Compression{Type: compression.Zstd, Level: config.Cfg.CompressionLevel}
	if err != nil {
		return object.Object{}, err
	}
	chunks, size, hash, err := chunker.ChunkStream(r, encType, compression, interfaces.GetAvailableBackends(), config.Cfg.ChunkConcurrency)
	if err != nil {
		return object.Object{}, err
	}

	var obj = object.Object{
		Location:    location,
		Size:        size,
		Compression: compression,
		ETag:        hash,
		Chunks:      chunks,
	}

	err = metadata.UpdateObject(obj)
	if err != nil {
		return object.Object{}, err
	}

	return obj, nil
}

func DeleteObject(location object.ObjectLocation) error {
	// fetch object metadata
	obj, err := metadata.GetObject(location)
	if err != nil {
		return err
	}

	// delete actual data
	err = chunker.DeleteChunks(obj.Chunks)
	if err != nil {
		return err
	}

	// delete metadata
	return metadata.DeleteObject(location)
}
