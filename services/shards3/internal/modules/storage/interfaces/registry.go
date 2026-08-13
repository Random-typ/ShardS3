package interfaces

import "fmt"

// Kind identifies a backend driver/implementation (e.g. "telegram", "file").
// Distinct from BackendType, which identifies a configured instance of a kind.
type Kind string

// SecretResolver looks up a named secret for one configured backend instance.
// Implementations typically read an environment variable such as
// SHARDS3_BACKEND_<ID>_<KEY>; the value is never sourced from YAML.
type SecretResolver func(key string) (string, bool)

// Factory constructs a configured Service for one backend instance definition.
type Factory func(id BackendType, settings map[string]any, secrets SecretResolver) (Service, error)

var factories = map[Kind]Factory{}
var instances = map[BackendType]Service{}

// RegisterKind makes a backend driver available to the config loader. Must be
// called from the backend implementation's init().
func RegisterKind(kind Kind, factory Factory) {
	factories[kind] = factory
}

// NewFactory looks up the registered Factory for kind.
func NewFactory(kind Kind) (Factory, bool) {
	f, ok := factories[kind]
	return f, ok
}

// RegisteredKinds returns all currently registered driver kinds.
func RegisteredKinds() []Kind {
	kinds := make([]Kind, 0, len(factories))
	for k := range factories {
		kinds = append(kinds, k)
	}
	return kinds
}

// RegisterInstance makes a configured backend instance available under id for
// runtime use (GetShard/PutShard/DeleteShard/GetMaxShardSize).
func RegisterInstance(id BackendType, svc Service) {
	instances[id] = svc
}

func getService(id BackendType) (Service, error) {
	svc, ok := instances[id]
	if !ok {
		return nil, fmt.Errorf("unconfigured backend %q", id)
	}
	return svc, nil
}
