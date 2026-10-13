// Package config is responsible for ONE thing: reading settings from the
// environment and handing them to the rest of the program as a typed struct.
//
// Concept: keep configuration out of the code (the "12-factor app" idea).
// Secrets like SMTP passwords come from environment variables, so the same
// binary can run on your laptop (Mailtrap sandbox) and in production (a real
// SMTP provider) without code changes.
package config

import (
	"errors"
	"io/fs"
	"os"

	"github.com/joho/godotenv"
)

// Config holds every setting the app needs.
type Config struct {
	// SMTP server we relay mail through (Mailtrap sandbox in development).
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string

	// MailFrom is the sender address. It lives in config, NOT in the API
	// request: clients must not be able to send mail "as" anyone they like.
	MailFrom string

	// HTTPAddr is where our API listens, e.g. ":8080".
	HTTPAddr string
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

	return Config{
		SMTPHost:     os.Getenv("MAILTRAP_HOST"),
		SMTPPort:     os.Getenv("MAILTRAP_PORT"),
		SMTPUsername: os.Getenv("MAILTRAP_USERNAME"),
		SMTPPassword: os.Getenv("MAILTRAP_PASSWORD"),
		MailFrom:     os.Getenv("MAIL_FROM"),
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
	}, nil
}

// getEnv returns the variable's value, or fallback if it is unset/empty.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
