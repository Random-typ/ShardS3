// Package backendconfig loads the storage backend inventory from a YAML file
// and builds configured backend instances from it. Non-secret settings live
// in YAML; secrets are always resolved from environment variables using the
// SHARDS3_BACKEND_<ID>_<KEY> naming convention, never stored in YAML.
package backendconfig

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// BackendDef is one backend instance definition as declared in backends.yaml.
type BackendDef struct {
	ID       string         `yaml:"id"`
	Kind     string         `yaml:"kind"`
	Enabled  bool           `yaml:"enabled"`
	Settings map[string]any `yaml:"settings"`
}

type backendsFile struct {
	Backends []BackendDef `yaml:"backends"`
}

// LoadBackends reads and validates the backend inventory at path. It checks
// for empty/duplicate IDs and missing kinds, but does not check whether a
// kind has a registered driver - that is left to BuildBackends so this
// function has no dependency on which backend drivers are compiled in.
func LoadBackends(path string) ([]BackendDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backends config %q: %w", path, err)
	}

	var file backendsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse backends config %q: %w", path, err)
	}

	seen := make(map[string]bool, len(file.Backends))
	for i, def := range file.Backends {
		if def.ID == "" {
			return nil, fmt.Errorf("backends config %q: backend at index %d is missing \"id\"", path, i)
		}
		if def.Kind == "" {
			return nil, fmt.Errorf("backends config %q: backend %q is missing \"kind\"", path, def.ID)
		}
		if seen[def.ID] {
			return nil, fmt.Errorf("backends config %q: duplicate backend id %q", path, def.ID)
		}
		seen[def.ID] = true
	}

	return file.Backends, nil
}
