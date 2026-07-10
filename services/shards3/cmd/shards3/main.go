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
	cfg := config.Config{ServiceName: "shardshards3", SQLitePath: "shards3.db", SQLiteBusyTimeoutMS: 5000}
	database, err := db.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	if err := encryption.ConfigureKMS(cfg, database); err != nil {
		log.Fatalf("Failed to configure KMS: %v", err)
	}
	fmt.Println("Database and KMS initialized successfully.")
}
