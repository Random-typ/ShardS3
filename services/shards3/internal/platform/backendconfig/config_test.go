package backendconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBackendsYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backends.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error: %v", err)
	}
	return path
}

func TestLoadBackends_Valid(t *testing.T) {
	path := writeBackendsYAML(t, `
backends:
  - id: file
    kind: file
    enabled: true
    settings:
      storage_dir: ./testdata
  - id: telegram
    kind: telegram
    enabled: false
`)

	defs, err := LoadBackends(path)
	if err != nil {
		t.Fatalf("LoadBackends() error: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("len(defs) = %d, want 2", len(defs))
	}
	if defs[0].ID != "file" || defs[0].Kind != "file" || !defs[0].Enabled {
		t.Fatalf("defs[0] = %+v, unexpected", defs[0])
	}
	if defs[0].Settings["storage_dir"] != "./testdata" {
		t.Fatalf("defs[0].Settings[storage_dir] = %v, want ./testdata", defs[0].Settings["storage_dir"])
	}
	if defs[1].Enabled {
		t.Fatalf("defs[1].Enabled = true, want false")
	}
}

func TestLoadBackends_MissingID(t *testing.T) {
	path := writeBackendsYAML(t, `
backends:
  - kind: file
    enabled: true
`)

	if _, err := LoadBackends(path); err == nil {
		t.Fatal("LoadBackends() error = nil, want error for missing id")
	}
}

func TestLoadBackends_MissingKind(t *testing.T) {
	path := writeBackendsYAML(t, `
backends:
  - id: file
    enabled: true
`)

	if _, err := LoadBackends(path); err == nil {
		t.Fatal("LoadBackends() error = nil, want error for missing kind")
	}
}

func TestLoadBackends_DuplicateID(t *testing.T) {
	path := writeBackendsYAML(t, `
backends:
  - id: file
    kind: file
    enabled: true
  - id: file
    kind: file
    enabled: false
`)

	if _, err := LoadBackends(path); err == nil {
		t.Fatal("LoadBackends() error = nil, want error for duplicate id")
	}
}

func TestLoadBackends_MissingFile(t *testing.T) {
	if _, err := LoadBackends(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatal("LoadBackends() error = nil, want error for missing file")
	}
}
