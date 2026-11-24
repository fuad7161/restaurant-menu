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

	TrustProxy bool // read the guest address from X-Forwarded-For; set only when a reverse proxy sits in front

	// Email (verify email, forgot password): Brevo when BREVO_API_KEY is set, else SMTP (for
	// example Gmail) when SMTP_HOST is set, else Resend when RESEND_API_KEY is set, else the log.
	BrevoAPIKey                                string
	SMTPHost, SMTPPort, SMTPUser, SMTPPassword string
	ResendAPIKey                               string
	EmailFrom                                  string // for example "Bosho <you@gmail.com>"
	EmailVerify                                bool   // owners must confirm their email before guests can order (EMAIL_VERIFY=1); off by default, and then nothing is checked or sent

	// PasswordKey encrypts the staff passwords the owner can look up (32 bytes, base64). Empty:
	// the app makes one and keeps it in the database.
	PasswordKey string

	RedisURL string // for example rediss://default:<token>@<name>.upstash.io:6379 (optional: lets refreshing screens skip the database when nothing changed)
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
		TrustProxy:  os.Getenv("TRUST_PROXY") == "1",

		BrevoAPIKey:  os.Getenv("BREVO_API_KEY"),
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     os.Getenv("SMTP_PORT"),
		SMTPUser:     os.Getenv("SMTP_USER"),
		SMTPPassword: strings.ReplaceAll(os.Getenv("SMTP_PASSWORD"), " ", ""), // Google shows app passwords in groups of 4
		ResendAPIKey: os.Getenv("RESEND_API_KEY"),
		EmailFrom:    os.Getenv("EMAIL_FROM"),
		EmailVerify:  os.Getenv("EMAIL_VERIFY") == "1",
		PasswordKey:  os.Getenv("PASSWORD_KEY"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
