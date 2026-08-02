package main

import (
	"log"

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

	if config.Cfg.EnableDashboard {
		go runS3Service()
		runDashboardService(database)
		return
	}
	runS3Service()
}
