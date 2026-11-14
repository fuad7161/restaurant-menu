// Package config reads the application settings from environment variables.
package config

import (
	"os"
	"strings"
)

type Config struct {
	Addr        string // listen address, for example ":8081"
	BaseURL     string // public address printed into QR codes; empty means "use the request host"
	DatabaseURL string // PostgreSQL connection string
	Demo        bool   // seed a sample restaurant when the database is empty

	S3Endpoint  string // MinIO or S3 host:port
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
}

func Load() Config {
	return Config{
		Addr:        env("ADDR", ":8081"),
		BaseURL:     strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		DatabaseURL: env("DATABASE_URL", "postgres://bosho:bosho@localhost:5433/bosho?sslmode=disable"),
		Demo:        os.Getenv("DEMO") == "1",
		S3Endpoint:  env("S3_ENDPOINT", "localhost:9010"),
		S3AccessKey: env("S3_ACCESS_KEY", "bosho"),
		S3SecretKey: env("S3_SECRET_KEY", "bosho-secret"),
		S3Bucket:    env("S3_BUCKET", "bosho-photos"),
		S3UseSSL:    os.Getenv("S3_USE_SSL") == "1",
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
