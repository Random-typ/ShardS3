package interfaces

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func init() {
	RegisterKind("file", newFileService)
}

// newFileService reads the "storage_dir" setting (defaults to "./testdata").
// The file backend has no secrets.
func newFileService(id BackendType, settings map[string]any, secrets SecretResolver) (Service, error) {
	dir, _ := settings["storage_dir"].(string)
	if dir == "" {
		dir = "./testdata"
	}
	return &FileService{dir: dir}, nil
}

// FileService stores objects on the local file system. Primarily useful for
// local development and deterministic tests. set fail to true to simulate a
// failure in the FileService methods.
type FileService struct {
	fail bool
	dir  string
}

func (s *FileService) storageDirectory() string {
	if s.dir == "" {
		return "./testdata"
	}
	return s.dir
}

// RegisterFileTestBackends registers n local-file backend instances under
// IDs "file-0".. "file-(n-1)" (optionally simulating a failure at the given
// 0-based indices) and returns their IDs, ready to pass to
// SetAvailableBackends. Intended for tests exercising multi-backend
// topologies without real external services.
func RegisterFileTestBackends(n int, failIdx ...int) []BackendType {
	fail := make(map[int]bool, len(failIdx))
	for _, i := range failIdx {
		fail[i] = true
	}
	ids := make([]BackendType, n)
	for i := 0; i < n; i++ {
		id := BackendType(fmt.Sprintf("file-%d", i))
		RegisterInstance(id, &FileService{fail: fail[i], dir: "./testdata"})
		ids[i] = id
	}
	return ids
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
	filePath := filepath.Join(s.storageDirectory(), location)
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
	storageDir := s.storageDirectory()
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
	filePath := filepath.Join(s.storageDirectory(), location)
	log.Printf("trace file_backend delete location=%s path=%s", location, filePath)
	err := os.Remove(filePath)
	if err != nil {
		log.Printf("trace file_backend delete_failed location=%s path=%s err=%v", location, filePath, err)
	}
	return err
}
