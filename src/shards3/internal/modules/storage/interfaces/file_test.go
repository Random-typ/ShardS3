package interfaces

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileServicePutObject_CreatesStorageDirectory(t *testing.T) {
	svc := &FileService{fail: false}
	storageDir := svc.storageDirectory()
	if err := os.RemoveAll(storageDir); err != nil {
		t.Fatalf("RemoveAll(%q) error: %v", storageDir, err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(storageDir)
	})

	location, err := svc.PutObject([]byte("payload"))
	if err != nil {
		t.Fatalf("PutObject() error: %v", err)
	}
	if location == "" {
		t.Fatal("PutObject() returned empty location")
	}

	if _, err := os.Stat(storageDir); err != nil {
		t.Fatalf("expected storage dir to exist at %q: %v", storageDir, err)
	}
	if _, err := os.Stat(filepath.Join(storageDir, location)); err != nil {
		t.Fatalf("expected shard file to exist for location %q: %v", location, err)
	}
}
