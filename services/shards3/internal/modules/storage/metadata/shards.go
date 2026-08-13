package metadata

import (
	"database/sql"
	"fmt"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/shard"
	"shards3/services/shards3/internal/platform/db"
	"time"

	"github.com/google/uuid"
)

func getShards(database *db.DB, chunkID string) ([]shard.Shard, error) {
	rows, err := database.Query(`
		SELECT first, last, backend_type, location, checksum
		FROM shards WHERE chunk_id = ? ORDER BY first ASC`,
		chunkID,
	)
	if err != nil {
		return nil, fmt.Errorf("query shards: %w", err)
	}
	defer rows.Close()

	var shards []shard.Shard
	for rows.Next() {
		var s shard.Shard
		var backendType string
		if err := rows.Scan(&s.First, &s.Last, &backendType, &s.Location, &s.Checksum); err != nil {
			return nil, fmt.Errorf("scan shard: %w", err)
		}
		s.Backend = interfaces.BackendType(backendType)
		shards = append(shards, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shards: %w", err)
	}

	return shards, nil
}

func insertShards(tx *sql.Tx, chunkID string, shards []shard.Shard) error {
	now := time.Now().UTC()
	for _, s := range shards {
		if _, err := tx.Exec(`
			INSERT INTO shards (chunk_id, first, last, backend_type, location, lastVerified, checksum)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			chunkID, s.First, s.Last, string(s.Backend), s.Location, now, s.Checksum,
		); err != nil {
			return fmt.Errorf("insert shard [%d,%d] for chunk %s: %w", s.First, s.Last, chunkID, err)
		}
	}
	return nil
}

// GetShards returns the shards recorded for the given chunk. Intended for
// read-only consumers such as stats/dashboard APIs.
func GetShards(chunkID uuid.UUID) ([]shard.Shard, error) {
	database, err := getDB()
	if err != nil {
		return nil, err
	}
	return getShards(database, chunkID.String())
}
