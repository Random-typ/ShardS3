package objectManager

import (
	"shards3/services/shards3/internal/modules/storage/chunker"
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
* Responsibilities:
*  receives uploads
*  creates object IDs
*  starts chunking
*  tracks object state
*  triggers storage operations
*
 */

func PutObject(location object.ObjectLocation, data []byte) (object.Object, error) {
	encType, err := encryption.EncryptionTypeFromString(config.Cfg.EncryptionMethod)
	if err != nil {
		return object.Object{}, err
	}
	chunks, err := chunker.ChunkData(data, encType, interfaces.GetAvailableBackends())
	if err != nil {
		return object.Object{}, err
	}

	var obj = object.Object{
		Location: location,
		Size:     int64(len(data)),
		Chunks:   chunks,
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

func UpdateObject(location object.ObjectLocation, data []byte) (object.Object, error) {
	encType, err := encryption.EncryptionTypeFromString(config.Cfg.EncryptionMethod)
	if err != nil {
		return object.Object{}, err
	}
	chunks, err := chunker.ChunkData(data, encType, interfaces.GetAvailableBackends())
	if err != nil {
		return object.Object{}, err
	}

	var obj = object.Object{
		Location: location,
		Size:     int64(len(data)),
		Chunks:   chunks,
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
