package main

import (
	"log"
	"net/http"
	"shards3/services/shards3/internal/modules/dashboard"
	"shards3/services/shards3/internal/modules/dashboard/web"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
	"time"
)

func runDashboardService(database *db.DB) {
	service := dashboard.NewService(database)
	server, err := web.NewServer(service)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{
		Addr:              config.Cfg.DashboardAddress,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("dashboard listening on %s", httpServer.Addr)
	log.Fatal(httpServer.ListenAndServe())
}
