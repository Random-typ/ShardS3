package db

import (
	"context"
	"database/sql"
	"fmt"
	"shards3/services/shards3/internal/platform/config"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

type DB struct {
	*sql.DB
}

func New() (*DB, error) {
	dbPath := config.Cfg.SQLitePath
	if dbPath == "" {
		dbPath = "shards3.db"
	}

	// append PRAGMAs for performance and reliability
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)", dbPath, config.Cfg.SQLiteBusyTimeoutMS)
	if config.Cfg.SQLiteBusyTimeoutMS == 0 {
		dsn = fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// SQLite handles concurrent reads but only one writer safely without SQLITE_BUSY locking contention.
	db.SetMaxOpenConns(config.Cfg.SQLiteMaxOpenConns)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{db}, nil
}

func runMigrations(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS kms_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	key_ciphertext BLOB NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME NULL
);

CREATE TABLE IF NOT EXISTS buckets (
	name TEXT PRIMARY KEY,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS objects (
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	size INTEGER NOT NULL,
	compression_type INTEGER NOT NULL,
	compression_level INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (bucket, object_key),
	FOREIGN KEY (bucket) REFERENCES buckets(name) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS chunks (
	id TEXT PRIMARY KEY,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	ordinal INTEGER NOT NULL,
	EncodingShardSize INTEGER NOT NULL,
	EncodingDataShards INTEGER NOT NULL,
	encryption_type INTEGER NOT NULL,
	key_id INTEGER NULL,
	size INTEGER NOT NULL,
	FOREIGN KEY (bucket, object_key) REFERENCES objects(bucket, object_key) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS shards (
	chunk_id TEXT NOT NULL,
	first INTEGER NOT NULL,
	last INTEGER NOT NULL,
	backend_type INTEGER NOT NULL,
	location TEXT NOT NULL,
	lastVerified DATETIME DEFAULT CURRENT_TIMESTAMP,
	checksum INTEGER NOT NULL,
	PRIMARY KEY (chunk_id, first, last),
	FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
);

-- Basic Migration Tracking
INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);
`
	_, err := db.Exec(schema)
	return err
}
