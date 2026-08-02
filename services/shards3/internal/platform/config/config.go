package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServiceName      string `envconfig:"SERVICE_NAME" default:"shards3"`
	Environment      string `envconfig:"ENVIRONMENT" default:"development"`
	S3Address        string `envconfig:"ADDRESS" default:":8080"`
	DashboardAddress string `envconfig:"DASHBOARD_ADDRESS" default:":8088"`
	FQDN             string `envconfig:"FQDN" default:"s3.localhost"`
	StorageClass     string `envconfig:"STORAGE_CLASS" default:"STANDARD_SHARDS3"`
	EnableDashboard  bool   `envconfig:"ENABLE_DASHBOARD" default:"true"`

	// S3 Auth
	S3AccountID       string `envconfig:"S3_ACCOUNT_ID" default:"0"`
	S3Region          string `envconfig:"S3_REGION" default:"us-east-1"`
	S3AccessKeyID     string `envconfig:"S3_ACCESS_KEY_ID" default:"test-access-key"`
	S3SecretAccessKey string `envconfig:"S3_SECRET_ACCESS_KEY" default:"test-secret-key"`
	S3AllowedSkewSec  int    `envconfig:"S3_ALLOWED_SKEW_SEC" default:"300"`

	// KMS config
	KMSPasswordKeyPath string `envconfig:"KMS_PASSWORD_KEY_PATH" default:"kms.key"`

	// Database config
	SQLitePath          string `envconfig:"SQLITE_PATH" default:"shards3.db"`
	SQLiteBusyTimeoutMS int    `envconfig:"SQLITE_BUSY_TIMEOUT_MS" default:"5000"`
	SQLiteMaxOpenConns  int    `envconfig:"SQLITE_MAX_OPEN_CONNS" default:"1"`

	// Chunking
	ChunkSize int `envconfig:"CHUNK_SIZE" default:"134217728"` // 128 MB

	// Number of chunks that may be compressed/encrypted/sharded concurrently
	// while streaming an object upload. Bounds peak memory usage to roughly
	// ChunkConcurrency * ChunkSize.
	ChunkConcurrency int `envconfig:"CHUNK_CONCURRENCY" default:"4"`

	// Compression
	CompressionLevel int `envconfig:"COMPRESSION_LEVEL" default:"3"` // 1-22 for zstd

	// Encryption
	EncryptionMethod string `envconfig:"ENCRYPTION_METHOD" default:"AES-256-GCM"` // AES-256-GCM, ChaCha20-Poly1305

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
