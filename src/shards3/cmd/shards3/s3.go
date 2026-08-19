package main

import (
	"log"
	"net/http"
	"shards3/internal/modules/s3/auth"
	"shards3/internal/modules/s3/bucket"
	"shards3/internal/modules/s3/web"
	"shards3/internal/platform/config"
	"time"
)

func runS3Service() {
	authService := auth.NewService(auth.Config{
		AccessKeyID:     config.Cfg.S3AccessKeyID,
		SecretAccessKey: config.Cfg.S3SecretAccessKey,
		Region:          config.Cfg.S3Region,
		AllowedSkew:     time.Duration(config.Cfg.S3AllowedSkewSec) * time.Second,
	})

	server := web.NewServer(bucket.NewService(), authService)
	httpServer := &http.Server{
		Addr:              config.Cfg.S3Address,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("shards3 s3 api listening on %s", httpServer.Addr)
	log.Fatal(httpServer.ListenAndServe())
}
