// Package config is responsible for ONE thing: reading settings from the
// environment and handing them to the rest of the program as a typed struct.
//
// Concept: keep configuration out of the code (the "12-factor app" idea).
// Secrets like SMTP passwords come from environment variables, so the same
// binary can run on your laptop (Mailpit/Mailtrap in development) and in
// production (a real SMTP provider) without code changes.
package config

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every setting the app needs.
type Config struct {
	// Database connection string for PostgreSQL, e.g.:
	// "postgres://postgres:postgres@localhost:5432/sendmail?sslmode=disable"
	DatabaseURL string

	// SMTP server we relay mail through (Mailpit or Mailtrap in development).
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string

	// MailFrom is the sender address. It lives in config, NOT in the API
	// request: clients must not be able to send mail "as" anyone they like.
	MailFrom string

	// HTTPAddr is where our API listens, e.g. ":8080".
	HTTPAddr string

	// HTTPReadTimeout bounds how long the server waits to read an entire
	// request, including the uploaded file. HTTPWriteTimeout bounds how long
	// a handler may take before its response must be written; it must exceed
	// the slowest request (e.g. a large batch upload with domain checks).
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration

	// WorkerCount is the number of background Go worker goroutines
	// polling PostgreSQL and sending emails concurrently.
	WorkerCount int

	// SMTPPoolSize is the maximum number of active SMTP connections
	// kept in the connection pool.
	SMTPPoolSize int

	// JobPollInterval is how long background workers sleep before checking
	// PostgreSQL again when the job queue is empty.
	JobPollInterval time.Duration

	// MaxJobRetries is how many times a failed email job will be retried
	// before being permanently marked as failed.
	MaxJobRetries int

	// DomainCheckEnabled turns on the DNS/MX check of recipient domains when
	// recipients are parsed. Rows whose domain cannot receive mail are rejected.
	DomainCheckEnabled bool

	// CORSAllowedOrigins lists the browser origins (the dashboard's URL) that
	// may call the API, e.g. "https://dashboard.example.com". "*" allows any.
	CORSAllowedOrigins []string
}

// Load reads configuration from a .env file (if present) and the process
// environment.
func Load() (Config, error) {
	// godotenv copies the lines of .env into the process environment.
	// In production there is usually no .env file - real environment
	// variables are set by the platform - so a missing file is fine.
	// Any OTHER error (e.g. a malformed file) is a real problem.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, err
	}

	// For backward-compatibility, check SMTP_HOST first, then MAILTRAP_HOST.
	smtpHost := getFirstEnv([]string{"SMTP_HOST", "MAILTRAP_HOST"}, "localhost")
	smtpPort := getFirstEnv([]string{"SMTP_PORT", "MAILTRAP_PORT"}, "1025")
	smtpUser := getFirstEnv([]string{"SMTP_USERNAME", "MAILTRAP_USERNAME"}, "")
	smtpPass := getFirstEnv([]string{"SMTP_PASSWORD", "MAILTRAP_PASSWORD"}, "")

	return Config{
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://localhost:5432/sendmail?sslmode=disable"),
		SMTPHost:     smtpHost,
		SMTPPort:     smtpPort,
		SMTPUsername: smtpUser,
		SMTPPassword: smtpPass,
		MailFrom:     getEnv("MAIL_FROM", "hello@example.com"),
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),

		// Deliberately generous so large uploads do not fail with a dropped connection.
		HTTPReadTimeout:  getEnvDuration("HTTP_READ_TIMEOUT", 5*time.Minute),
		HTTPWriteTimeout: getEnvDuration("HTTP_WRITE_TIMEOUT", 15*time.Minute),

		WorkerCount:     getEnvInt("WORKER_COUNT", 3),
		SMTPPoolSize:    getEnvInt("SMTP_POOL_SIZE", 3),
		JobPollInterval: getEnvDuration("JOB_POLL_INTERVAL", 500*time.Millisecond),
		MaxJobRetries:   getEnvInt("MAX_JOB_RETRIES", 3),

		DomainCheckEnabled: getEnvBool("DOMAIN_CHECK_ENABLED", true),

		CORSAllowedOrigins: getEnvList("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
	}, nil
}

// getEnvBool parses a boolean environment variable ("true", "0", ...), or returns fallback.
func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// getEnvList parses a comma-separated environment variable, or returns fallback.
func getEnvList(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// getEnv returns the variable's value, or fallback if it is unset/empty.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getFirstEnv returns the value of the first non-empty environment variable
// from keys, or fallback if none are set.
func getFirstEnv(keys []string, fallback string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return fallback
}

// getEnvInt parses an integer environment variable, or returns fallback.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// getEnvDuration parses a time.Duration (e.g. "500ms", "1s"), or returns fallback.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
