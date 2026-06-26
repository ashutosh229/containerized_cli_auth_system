package config

import (
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	DatabaseDSN       string
	SessionTimeout    time.Duration
	MaxFailedAttempts int
	LockoutDuration   time.Duration
	TOTPIssuer        string
	BCryptCost        int
}

func Load() Config {
	return Config{
		DatabaseDSN:       getEnv("DB_DSN", "data/auth.db"),
		SessionTimeout:    durationEnv("SESSION_TIMEOUT", 30*time.Minute),
		MaxFailedAttempts: intEnv("MAX_FAILED_ATTEMPTS", 5),
		LockoutDuration:   durationEnv("LOCKOUT_DURATION", 15*time.Minute),
		TOTPIssuer:        getEnv("TOTP_ISSUER", "Containerized CLI Auth"),
		BCryptCost:        intEnv("BCRYPT_COST", bcrypt.DefaultCost),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
