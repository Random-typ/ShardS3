package main

import (
	"log"
	"net/http"
	"time"

	"shards3/services/shards3/internal/modules/s3/auth"
	"shards3/services/shards3/internal/modules/s3/bucket"
	"shards3/services/shards3/internal/modules/s3/web"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

func main() {
	err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load failed: %v", err)
	}

	database, err := db.New()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	if err := encryption.ConfigureKMS(database); err != nil {
		log.Fatalf("Failed to configure KMS: %v", err)
	}
	metadata.Configure(database)

	interfaces.SetAvailableBackends([]interfaces.BackendType{interfaces.File, interfaces.File2, interfaces.File3, interfaces.File4, interfaces.File5})

	authService := auth.NewService(auth.Config{
		AccessKeyID:     config.Cfg.S3AccessKeyID,
		SecretAccessKey: config.Cfg.S3SecretAccessKey,
		Region:          config.Cfg.S3Region,
		AllowedSkew:     time.Duration(config.Cfg.S3AllowedSkewSec) * time.Second,
	})

	server := web.NewServer(bucket.NewService(), authService)
	httpServer := &http.Server{
		Addr:              config.Cfg.Address,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("shards3 s3 api listening on %s", httpServer.Addr)
	log.Fatal(httpServer.ListenAndServe())
}
