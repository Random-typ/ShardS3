package interfaces

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

/*
* This is a test implementation of the Service interface for testing purposes. It uses the local file system to store and retrieve objects.
 */
// set fail to true to simulate a failure in the FileService methods.
type FileService struct{ fail bool }

func getStorageDirectory() string {
	return "./testdata"
}

// generateLocation returns a name that is unique even when multiple shards
// are written concurrently (e.g. across backends uploading in parallel) -
// a timestamp alone is not fine-grained/unique enough for that.
func generateLocation() string {
	return "shard_" + uuid.New().String()
}

func (s *FileService) GetMaxObjectSize() int {
	return 30 * 1024 * 1024 // 30MiB
}

func (s *FileService) GetObject(location string) ([]byte, error) {
	if s.fail {
		return nil, fmt.Errorf("simulated failure in GetObject")
	}
	filePath := filepath.Join(getStorageDirectory(), location)
	log.Printf("trace file_backend get location=%s path=%s", location, filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("trace file_backend get_failed location=%s path=%s err=%v", location, filePath, err)
		return nil, err
	}
	return data, nil
}

func (s *FileService) PutObject(data []byte) (string, error) {
	if s.fail {
		return "", fmt.Errorf("simulated failure in PutObject")
	}
	storageDir := getStorageDirectory()
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		log.Printf("trace file_backend mkdir_failed dir=%s err=%v", storageDir, err)
		return "", err
	}
	location := generateLocation() // You need to implement this function to generate a unique location for the shard.
	filePath := filepath.Join(storageDir, location)
	log.Printf("trace file_backend put location=%s path=%s bytes=%d", location, filePath, len(data))
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		log.Printf("trace file_backend put_failed location=%s path=%s err=%v", location, filePath, err)
		return "", err
	}
	return location, nil
}

func (s *FileService) DeleteObject(location string) error {
	if s.fail {
		return fmt.Errorf("simulated failure in DeleteObject")
	}
	filePath := filepath.Join(getStorageDirectory(), location)
	log.Printf("trace file_backend delete location=%s path=%s", location, filePath)
	err := os.Remove(filePath)
	if err != nil {
		log.Printf("trace file_backend delete_failed location=%s path=%s err=%v", location, filePath, err)
	}
	return err
}
