package metadata

import (
	"database/sql"
	"fmt"
	"shards3/services/shards3/internal/modules/storage/object"
	"time"
)

// ListBackendStats aggregates usage metrics by backend type from shard metadata.
func ListBackendStats() ([]object.BackendStats, error) {
	database, err := getDB()
	if err != nil {
		return nil, err
	}

	rows, err := database.Query(`
		SELECT
			s.backend_type,
			COUNT(*) AS total_shards,
			COALESCE(SUM((s.last - s.first) + 1), 0) AS total_bytes,
			COUNT(DISTINCT s.chunk_id) AS total_chunks,
			COUNT(DISTINCT CASE WHEN c.object_key IS NOT NULL THEN c.bucket || '/' || c.object_key END) AS total_objects,
			COUNT(DISTINCT CASE WHEN c.object_key IS NOT NULL THEN c.bucket END) AS total_buckets,
			MAX(s.lastVerified) AS last_verified
		FROM shards s
		LEFT JOIN chunks c ON c.id = s.chunk_id
		GROUP BY s.backend_type
		ORDER BY s.backend_type ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query backend stats: %w", err)
	}
	defer rows.Close()

	stats := make([]object.BackendStats, 0)
	for rows.Next() {
		var stat object.BackendStats
		var lastVerified sql.NullString
		if err := rows.Scan(
			&stat.Backend,
			&stat.TotalShards,
			&stat.TotalBytes,
			&stat.TotalChunks,
			&stat.TotalObjects,
			&stat.TotalBuckets,
			&lastVerified,
		); err != nil {
			return nil, fmt.Errorf("scan backend stats: %w", err)
		}
		if lastVerified.Valid && lastVerified.String != "" {
			parsed, err := parseSQLiteTime(lastVerified.String)
			if err != nil {
				return nil, fmt.Errorf("parse backend stats lastVerified %q: %w", lastVerified.String, err)
			}
			stat.LastVerified = parsed
		}
		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backend stats: %w", err)
	}

	return stats, nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp format")
}
