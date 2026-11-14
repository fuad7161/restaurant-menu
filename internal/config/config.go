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

	S3Endpoint  string // S3-compatible host[:port], for example s3.us-east-005.backblazeb2.com
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string // region from the endpoint, for example "us-east-005" on Backblaze B2
	S3UseSSL    bool

	RedisURL string // for example rediss://default:<token>@<name>.upstash.io:6379 (not used by the app yet)
}

func Load() Config {
	return Config{
		Addr:        env("ADDR", ":8081"),
		BaseURL:     strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Demo:        os.Getenv("DEMO") == "1",
		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		S3Bucket:    env("S3_BUCKET", "bosho-photos"),
		S3Region:    os.Getenv("S3_REGION"),
		S3UseSSL:    os.Getenv("S3_USE_SSL") != "0",
		RedisURL:    os.Getenv("REDIS_URL"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
