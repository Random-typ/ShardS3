package main

import (
	"fmt"
	"log"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

func main() {
	fmt.Println("hello world from shards3 shards3")
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
	fmt.Println("Database and KMS initialized successfully.")
}
