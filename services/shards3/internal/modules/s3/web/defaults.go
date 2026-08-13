package web

import "shards3/services/shards3/internal/platform/config"

func GetDefaultUser() User {
	return User{
		ID:          config.Cfg.S3AccountID,
		DisplayName: config.Cfg.ServiceName,
	}
}

func GetDefaultStorageClass() string {
	return config.Cfg.StorageClass
}

type ChecksumMetadata struct {
	Algorithm string
	Type      string
}

func GetDefaultChecksumMetadata() ChecksumMetadata {
	return ChecksumMetadata{
		Algorithm: "XXHASH64",
		Type:      "FULL_OBJECT",
	}
}
