package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServiceName string `envconfig:"SERVICE_NAME" default:"shardshards3"`
	Environment string `envconfig:"ENVIRONMENT" default:"development"`
	Address     string `envconfig:"ADDRESS" default:":8080"`

	// KMS config
	KMSPasswordKeyPath string `envconfig:"KMS_PASSWORD_KEY_PATH" default:"kms.key"`

	// Database config
	SQLitePath          string `envconfig:"SQLITE_PATH" default:"shards3.db"`
	SQLiteBusyTimeoutMS int    `envconfig:"SQLITE_BUSY_TIMEOUT_MS" default:"5000"`
	SQLiteMaxOpenConns  int    `envconfig:"SQLITE_MAX_OPEN_CONNS" default:"1"`

	// Chunking
	ChunkSize int `envconfig:"CHUNK_SIZE" default:"134217728"` // 128 MB

	// Compression
	CompressionLevel int `envconfig:"COMPRESSION_LEVEL" default:"3"` // 1-22 for zstd

	// Encryption
	EncryptionMethod int `envconfig:"ENCRYPTION_METHOD" default:"AES-256-GCM"` // AES-256-GCM, ChaCha20-Poly1305

	// Failure Tolerance
	FailureTolerance int `envconfig:"FAILURE_TOLERANCE" default:"2"` // Number of backends that can fail without losing data. Must be less than the number of backends.
}

var Cfg Config

func LoadConfig() error {
	err := envconfig.Process("SHARDS3", &Cfg)
	if err != nil {
		return fmt.Errorf("failed to load config from env: %w", err)
	}
	return nil
}
