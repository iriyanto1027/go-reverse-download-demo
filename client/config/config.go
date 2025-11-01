package config

import (
	"os"
	"strconv"
)

// Config holds the client configuration
type Config struct {
	ClientID    string
	ServerWSURL string
	ClientToken string
	FilePath    string
	LogLevel    string
}

// Load loads the configuration from environment variables
func Load() *Config {
	return &Config{
		ClientID:    getEnv("CLIENT_ID", "default-client"),
		ServerWSURL: getEnv("SERVER_WS_URL", "ws://localhost:8080/ws/connect"),
		ClientToken: getEnv("CLIENT_TOKEN", ""),
		FilePath:    getEnv("FILE_PATH", "/data/report.bin"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvInt gets an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}
