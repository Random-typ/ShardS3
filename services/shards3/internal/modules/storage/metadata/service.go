package metadata

import (
	"errors"
	"sync"

	"shards3/services/shards3/internal/platform/db"
)

/* Manages Metadata for objects
*
* Stores the map of objects → chunks → locations
*
*
*
 */

var (
	defaultDBMu sync.RWMutex
	defaultDB   *db.DB
)

// Configure wires the database instance used by this package. Must be called
// once during startup, before any other function in this package is used.
func Configure(database *db.DB) {
	defaultDBMu.Lock()
	defer defaultDBMu.Unlock()
	defaultDB = database
}

func getDB() (*db.DB, error) {
	defaultDBMu.RLock()
	defer defaultDBMu.RUnlock()
	if defaultDB == nil {
		return nil, errors.New("metadata: database not configured, call Configure first")
	}
	return defaultDB, nil
}
