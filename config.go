package main

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	To       string
}

func loadConfig() (Config, error) {
	err := godotenv.Load()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Host:     os.Getenv("MAILTRAP_HOST"),
		Port:     os.Getenv("MAILTRAP_PORT"),
		Username: os.Getenv("MAILTRAP_USERNAME"),
		Password: os.Getenv("MAILTRAP_PASSWORD"),
		From:     os.Getenv("MAIL_FROM"),
		To:       os.Getenv("MAIL_TO"),
	}, nil
}
