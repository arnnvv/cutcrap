package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	OpenRouterKey  string
	MaxConcurrent  int
	RequestTimeout time.Duration
	ChunkSize      int
	ChunkOverlap   int // New field for transcript processing
}

func Load() *Config {
	log.Println("Loading configuration from environment")

	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or couldn't be loaded: %v", err)
	} else {
		log.Println("Successfully loaded .env file")
	}

	port := getEnv("PORT", "8080")
	log.Printf("PORT: %s", port)

	apiKey := getEnv("OPENROUTER_API_KEY", "")
	if apiKey == "" {
		log.Printf("WARNING: OPENROUTER_API_KEY not set")
	} else {
		log.Printf("OPENROUTER_API_KEY: [REDACTED]")
	}

	maxConcurrent := getEnvAsInt("MAX_CONCURRENT", 10)
	log.Printf("MAX_CONCURRENT: %d", maxConcurrent)

	requestTimeout := getEnvAsDuration("REQUEST_TIMEOUT", 30*time.Second)
	log.Printf("REQUEST_TIMEOUT: %v", requestTimeout)

	chunkSize := getEnvAsInt("CHUNK_SIZE", 900)
	log.Printf("CHUNK_SIZE: %d", chunkSize)

	// New configuration for transcript processing
	chunkOverlap := getEnvAsInt("CHUNK_OVERLAP", 100)
	log.Printf("CHUNK_OVERLAP: %d", chunkOverlap)

	return &Config{
		Port:           port,
		OpenRouterKey:  apiKey,
		MaxConcurrent:  maxConcurrent,
		RequestTimeout: requestTimeout,
		ChunkSize:      chunkSize,
		ChunkOverlap:   chunkOverlap,
	}
}

// getEnv retrieves environment variable or returns default value if not set
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Printf("Environment variable %s not set, using default: %s", key, defaultValue)
		return defaultValue
	}
	return value
}

// getEnvAsInt retrieves environment variable as integer or returns default value
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Failed to parse %s as integer: %v, using default: %d", key, err, defaultValue)
		return defaultValue
	}
	return value
}

// getEnvAsDuration retrieves environment variable as duration or returns default value
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}

	value, err := time.ParseDuration(valueStr)
	if err != nil {
		log.Printf("Failed to parse %s as duration: %v, using default: %v", key, err, defaultValue)
		return defaultValue
	}
	return value
}
