package config

import (
	"os"
	"strconv"
)

// Config holds all configuration for market-scanner.
type Config struct {
	// GitHubToken is a GitHub personal access token for API access.
	GitHubToken string

	// FactoryDataDir is the directory where factory data (SQLite DB, specs) lives.
	FactoryDataDir string

	// NoveltyThreshold is the minimum novelty score (0.0-1.0) below which
	// a build should be skipped. Default 0.6.
	NoveltyThreshold float64

	// ListenAddr is the address for the HTTP API server.
	ListenAddr string

	// DBPath is the path to the SQLite database file.
	DBPath string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	c := &Config{
		GitHubToken:      os.Getenv("GITHUB_TOKEN"),
		FactoryDataDir:   envOrDefault("FACTORY_DATA_DIR", "/tmp/factory-data"),
		NoveltyThreshold: envFloatOrDefault("NOVELTY_THRESHOLD", 0.6),
		ListenAddr:       envOrDefault("LISTEN_ADDR", ":8090"),
	}
	c.DBPath = envOrDefault("DB_PATH", c.FactoryDataDir+"/market-scanner.db")
	return c
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envFloatOrDefault(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
