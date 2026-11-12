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
	"fmt"
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

	// SMTPTLSMode controls transport encryption to the SMTP server:
	//   "auto"     - upgrade with STARTTLS only if the server offers it (default)
	//   "none"     - never encrypt (local test servers like Mailpit)
	//   "starttls" - require STARTTLS; fail if the server does not offer it (port 587)
	//   "tls"      - encrypted from the first byte, "implicit TLS" (port 465)
	SMTPTLSMode string

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

	// MaxUploadMB is the largest spreadsheet (in MB) the upload endpoints accept.
	// Parsing holds the whole sheet in memory, so it is capped at MaxUploadMBLimit.
	MaxUploadMB int

	// ImportChunkSize is how many recipient rows each background import
	// transaction writes at once.
	ImportChunkSize int

	// JobGenerationChunkSize is how many email jobs each background
	// generation transaction creates at once after Send.
	JobGenerationChunkSize int

	// UploadDir is the folder where uploaded spreadsheets are saved
	// (batch-<id>.xlsx). The import worker reads them from there, also after
	// a restart, so it must survive restarts (not /tmp on some systems).
	UploadDir string

	// DomainCheckEnabled turns on the DNS/MX check of recipient domains when
	// recipients are parsed. Rows whose domain cannot receive mail are rejected.
	DomainCheckEnabled bool

	// CORSAllowedOrigins lists the browser origins (the dashboard's URL) that
	// may call the API, e.g. "https://dashboard.example.com". "*" allows any.
	CORSAllowedOrigins []string

	// AutoMigrate makes the server apply pending database migrations at startup.
	// Set it to false when migrations run as a separate deploy step; the server
	// then refuses to start if the database is behind.
	AutoMigrate bool

	// LogLevel is the least serious level written: "debug", "info", "warn" or "error".
	LogLevel string

	// LogFormat is "text" (key=value, easy to read) or "json" (for log tools).
	LogFormat string

	// LogFile, when set, receives a copy of every log line (e.g. "logs/server.log").
	// Empty means the terminal (stderr) only.
	LogFile string

	// SettingsEncryptionKey is a base64-encoded 32-byte key (openssl rand -base64 32)
	// used to encrypt secret settings, such as the SMTP password, in the database.
	// Without it, secrets cannot be edited from the dashboard.
	SettingsEncryptionKey string
}

// Upload size bounds, in MB. Parsing needs roughly 13x the file size in memory,
// so the ceiling keeps one upload from exhausting the server.
const (
	DefaultMaxUploadMB = 10
	MaxUploadMBLimit   = 50
)

// Chunk size defaults and upper bounds for background import and job generation.
const (
	DefaultImportChunkSize        = 1000
	MaxImportChunkSize            = 50000
	DefaultJobGenerationChunkSize = 5000
	MaxJobGenerationChunkSize     = 100000
)

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

	tlsMode := strings.ToLower(getEnv("SMTP_TLS_MODE", "auto"))
	if !ValidTLSMode(tlsMode) {
		return Config{}, fmt.Errorf("SMTP_TLS_MODE %q is invalid: use auto, none, starttls or tls", tlsMode)
	}

	maxUploadMB := getEnvInt("MAX_UPLOAD_MB", DefaultMaxUploadMB)
	if maxUploadMB > MaxUploadMBLimit {
		return Config{}, fmt.Errorf("MAX_UPLOAD_MB %d is too large: the limit is %d", maxUploadMB, MaxUploadMBLimit)
	}

	importChunkSize := getEnvInt("IMPORT_CHUNK_SIZE", DefaultImportChunkSize)
	if importChunkSize > MaxImportChunkSize {
		return Config{}, fmt.Errorf("IMPORT_CHUNK_SIZE %d is too large: the limit is %d", importChunkSize, MaxImportChunkSize)
	}

	jobGenerationChunkSize := getEnvInt("JOB_GENERATION_CHUNK_SIZE", DefaultJobGenerationChunkSize)
	if jobGenerationChunkSize > MaxJobGenerationChunkSize {
		return Config{}, fmt.Errorf("JOB_GENERATION_CHUNK_SIZE %d is too large: the limit is %d", jobGenerationChunkSize, MaxJobGenerationChunkSize)
	}

	logLevel := strings.ToLower(getEnv("LOG_LEVEL", "info"))
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL %q is invalid: use debug, info, warn or error", logLevel)
	}
	logFormat := strings.ToLower(getEnv("LOG_FORMAT", "text"))
	if logFormat != "text" && logFormat != "json" {
		return Config{}, fmt.Errorf("LOG_FORMAT %q is invalid: use text or json", logFormat)
	}

	return Config{
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://localhost:5432/sendmail?sslmode=disable"),
		SMTPHost:     smtpHost,
		SMTPPort:     smtpPort,
		SMTPUsername: smtpUser,
		SMTPPassword: smtpPass,
		SMTPTLSMode:  tlsMode,
		MailFrom:     getEnv("MAIL_FROM", "hello@example.com"),
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),

		// Deliberately generous so large uploads do not fail with a dropped connection.
		HTTPReadTimeout:  getEnvDuration("HTTP_READ_TIMEOUT", 5*time.Minute),
		HTTPWriteTimeout: getEnvDuration("HTTP_WRITE_TIMEOUT", 15*time.Minute),

		WorkerCount:            getEnvInt("WORKER_COUNT", 3),
		SMTPPoolSize:           getEnvInt("SMTP_POOL_SIZE", 3),
		JobPollInterval:        getEnvDuration("JOB_POLL_INTERVAL", 500*time.Millisecond),
		MaxJobRetries:          getEnvInt("MAX_JOB_RETRIES", 3),
		MaxUploadMB:            maxUploadMB,
		ImportChunkSize:        importChunkSize,
		JobGenerationChunkSize: jobGenerationChunkSize,
		UploadDir:              getEnv("UPLOAD_DIR", "data/uploads"),

		DomainCheckEnabled: getEnvBool("DOMAIN_CHECK_ENABLED", true),

		AutoMigrate: getEnvBool("AUTO_MIGRATE", true),

		LogLevel:  logLevel,
		LogFormat: logFormat,
		LogFile:   strings.TrimSpace(os.Getenv("LOG_FILE")),

		SettingsEncryptionKey: getEnv("SETTINGS_ENCRYPTION_KEY", ""),

		CORSAllowedOrigins: getEnvList("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
	}, nil
}

// ValidTLSMode reports whether mode is a supported SMTP_TLS_MODE value.
func ValidTLSMode(mode string) bool {
	switch mode {
	case "auto", "none", "starttls", "tls":
		return true
	}
	return false
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
