package backendconfig

import (
	"fmt"
	"os"
	"strings"

	"shards3/internal/modules/storage/interfaces"
)

// envSecretResolver reads secrets for backend id from environment variables
// named SHARDS3_BACKEND_<ID>_<KEY> (id and key are upper-cased, non-alphanumeric
// characters replaced with underscores).
func envSecretResolver(id string) interfaces.SecretResolver {
	return func(key string) (string, bool) {
		return os.LookupEnv(secretEnvName(id, key))
	}
}

func secretEnvName(id, key string) string {
	sanitize := func(s string) string {
		s = strings.ToUpper(s)
		return strings.Map(func(r rune) rune {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return '_'
		}, s)
	}
	return "SHARDS3_BACKEND_" + sanitize(id) + "_" + sanitize(key)
}

// BuildBackends constructs and registers a Service for every enabled
// definition, returning the IDs of the backends that are now available for
// use via interfaces.SetAvailableBackends. Disabled backends are skipped
// entirely (their secrets are never looked up).
func BuildBackends(defs []BackendDef) ([]interfaces.BackendType, error) {
	ids := make([]interfaces.BackendType, 0, len(defs))
	for _, def := range defs {
		if !def.Enabled {
			continue
		}

		factory, ok := interfaces.NewFactory(interfaces.Kind(def.Kind))
		if !ok {
			return nil, fmt.Errorf("backend %q: unknown kind %q", def.ID, def.Kind)
		}

		id := interfaces.BackendType(def.ID)
		svc, err := factory(id, def.Settings, envSecretResolver(def.ID))
		if err != nil {
			return nil, fmt.Errorf("backend %q: %w", def.ID, err)
		}

		interfaces.RegisterInstance(id, svc)
		ids = append(ids, id)
	}
	return ids, nil
}
