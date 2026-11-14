// Command bosho runs the QR table ordering server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"bosho/internal/config"
	"bosho/internal/database"
	"bosho/internal/server"
	"bosho/internal/storage"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" || cfg.S3Endpoint == "" {
		log.Fatal("DATABASE_URL and S3_ENDPOINT must be set (copy .env.example to .env and fill it in)")
	}

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		log.Fatal(err)
	}

	photos, err := storage.New(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3Region, cfg.S3UseSSL)
	if err != nil {
		log.Fatal("object storage: ", err)
	}

	app, err := server.New(cfg, db, photos)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Stop cleanly on Ctrl+C or "docker stop": finish requests in flight, then exit.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Printf("Bosho listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
