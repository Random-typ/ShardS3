package main

import (
	"log"
	"net/http"
	"time"

	"shards3/services/shards3/internal/modules/dashboard"
	"shards3/services/shards3/internal/modules/dashboard/web"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Config load failed: %v", err)
	}

	database, err := db.New()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	metadata.Configure(database)

	service := dashboard.NewService(database)
	server, err := web.NewServer(service)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("dashboard listening on %s", httpServer.Addr)
	log.Fatal(httpServer.ListenAndServe())
}