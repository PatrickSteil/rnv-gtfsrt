// Package config loads and validates application configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration.
type Config struct {
	// OAuth2 / API credentials
	OAuthURL     string
	ClientID     string
	ClientSecret string
	ResourceID   string

	// API endpoint
	ClientAPIURL string

	// Polling behaviour
	PollInterval time.Duration
	// First/page size used when fetching stations and journeys
	PageSize int

	// HTTP server
	ListenAddr string

	// Optional: limit polling to specific station hafasIDs (comma-separated env var).
	// Empty means "all stations".
	StationFilter []string
}

// Load reads configuration from environment variables.
//
// Required:
//
//	RNV_OAUTH_URL        – OAuth2 token endpoint
//	RNV_CLIENT_ID        – OAuth2 client id
//	RNV_CLIENT_SECRET    – OAuth2 client secret
//	RNV_RESOURCE_ID      – OAuth2 resource/audience
//	RNV_API_URL          – GraphQL endpoint, e.g. https://graphql-sandbox-dds.rnv-online.de/
//
// Optional:
//
//	RNV_POLL_INTERVAL    – polling interval (default 30s)
//	RNV_PAGE_SIZE        – elements per page (default 50)
//	RNV_LISTEN_ADDR      – HTTP listen address (default :8080)
//	RNV_STATION_FILTER   – comma-separated hafasIDs to restrict polling (default: all)
func Load() (*Config, error) {
	cfg := &Config{
		OAuthURL:     mustEnv("RNV_OAUTH_URL"),
		ClientID:     mustEnv("RNV_CLIENT_ID"),
		ClientSecret: mustEnv("RNV_CLIENT_SECRET"),
		ResourceID:   mustEnv("RNV_RESOURCE_ID"),
		ClientAPIURL: mustEnv("RNV_API_URL"),

		PollInterval: envDuration("RNV_POLL_INTERVAL", 60*time.Second),
		PageSize:     envInt("RNV_PAGE_SIZE", 500),
		ListenAddr:   envStr("RNV_LISTEN_ADDR", ":8080"),
	}

	if raw := os.Getenv("RNV_STATION_FILTER"); raw != "" {
		cfg.StationFilter = splitComma(raw)
	}

	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.OAuthURL == "" {
		return fmt.Errorf("RNV_OAUTH_URL is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("RNV_CLIENT_ID is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("RNV_CLIENT_SECRET is required")
	}
	if c.ResourceID == "" {
		return fmt.Errorf("RNV_RESOURCE_ID is required")
	}
	if c.ClientAPIURL == "" {
		return fmt.Errorf("RNV_API_URL is required")
	}
	return nil
}

// mustEnv returns the value of an env var, or empty string (validation handled later).
func mustEnv(key string) string {
	return os.Getenv(key)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func splitComma(s string) []string {
	var out []string
	for _, part := range splitRunes(s, ',') {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitRunes(s string, sep rune) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == sep {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
