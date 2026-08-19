package main

import (
	"log"

	"shards3/internal/modules/storage/encryption"
	"shards3/internal/modules/storage/interfaces"
	"shards3/internal/modules/storage/metadata"
	"shards3/internal/platform/backendconfig"
	"shards3/internal/platform/config"
	"shards3/internal/platform/db"
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

	backendDefs, err := backendconfig.LoadBackends(config.Cfg.BackendsConfigPath)
	if err != nil {
		log.Fatalf("Failed to load backends config: %v", err)
	}
	backendIDs, err := backendconfig.BuildBackends(backendDefs)
	if err != nil {
		log.Fatalf("Failed to configure backends: %v", err)
	}
	if len(backendIDs) <= config.Cfg.FailureTolerance {
		log.Fatalf("Not enough enabled backends: have %d, need more than FailureTolerance (%d)", len(backendIDs), config.Cfg.FailureTolerance)
	}
	interfaces.SetAvailableBackends(backendIDs)

	if config.Cfg.EnableDashboard {
		go runS3Service()
		runDashboardService(database)
		return
	}
	runS3Service()
}
