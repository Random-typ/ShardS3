package interfaces

import (
	"fmt"
	"os"
	"time"
)

/*
* This is a test implementation of the Service interface for testing purposes. It uses the local file system to store and retrieve objects.
 */
// set fail to true to simulate a failure in the FileService methods.
type FileService struct{ fail bool }

func getStorageDirectory() string {
	return "./testdata"
}

func generateLocation() string {
	return "shard_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *FileService) GetMaxObjectSize() int {
	return 30 * 1024 * 1024 // 30MiB
}

func (s *FileService) GetObject(location string) ([]byte, error) {
	if s.fail {
		return nil, fmt.Errorf("simulated failure in GetObject")
	}
	filePath := getStorageDirectory() + "/" + location
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *FileService) PutObject(data []byte) (string, error) {
	if s.fail {
		return "", fmt.Errorf("simulated failure in PutObject")
	}
	location := generateLocation() // You need to implement this function to generate a unique location for the shard.
	filePath := getStorageDirectory() + "/" + location
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		return "", err
	}
	return location, nil
}

func (s *FileService) DeleteObject(location string) error {
	if s.fail {
		return fmt.Errorf("simulated failure in DeleteObject")
	}
	filePath := getStorageDirectory() + "/" + location
	err := os.Remove(filePath)
	return err
}
