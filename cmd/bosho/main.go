// Command bosho runs the QR table ordering server.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bosho/internal/cache"
	"bosho/internal/config"
	"bosho/internal/database"
	"bosho/internal/server"
	"bosho/internal/storage"
)

func main() {
	// Structured log lines (time, level, message, key=value pairs). The standard log package
	// writes through the same handler.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	cfg := config.Load()
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		createAdmin(cfg)
		return
	}
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

	// Redis is optional: without it (or if it is unreachable) everything still works, refreshes
	// just always ask PostgreSQL. The pause must outlast a refresh token (server.pollSlot).
	ver, err := cache.New(cfg.RedisURL, time.Minute)
	if err != nil {
		log.Println("redis unavailable, running without the refresh shortcut:", err)
		ver = nil
	} else if ver != nil {
		log.Println("redis connected: unchanged screens skip the database")
		defer ver.Close()
	}

	app, err := server.New(cfg, db, photos, ver)
	if err != nil {
		log.Fatal(err)
	}

	// Generous read time for photo uploads on slow mobile data; handlers themselves are limited
	// to 20 seconds (server.handlerTimeout).
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	// Stop cleanly on Ctrl+C or "docker stop": finish requests in flight, then exit.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go app.Maintain(ctx)
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

// createAdmin adds a platform admin, or sets a new password for one: bosho create-admin EMAIL.
// The password is read from ADMIN_PASSWORD, or typed in (it shows on the screen).
func createAdmin(cfg config.Config) {
	if len(os.Args) != 3 || cfg.DatabaseURL == "" {
		log.Fatal("usage: DATABASE_URL=... bosho create-admin you@example.com")
	}
	pass := os.Getenv("ADMIN_PASSWORD")
	if pass == "" {
		fmt.Fprint(os.Stderr, "Password for "+os.Args[2]+" (12 or more characters): ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		pass = strings.TrimRight(line, "\r\n")
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		log.Fatal(err)
	}
	if err := server.CreateAdmin(db, os.Args[2], pass); err != nil {
		log.Fatal("create-admin: ", err)
	}
	fmt.Println("Admin ready:", os.Args[2], "- log in at /admin/login")
}
