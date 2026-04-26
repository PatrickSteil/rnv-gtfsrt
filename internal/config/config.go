// Package config loads and validates application configuration from
// environment variables. All required variables are validated at startup
// so the server fails fast rather than encountering missing credentials
// at runtime.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all runtime configuration for the rnv-gtfsrt server.
type Config struct {
	// OAuthURL is the OAuth2 token endpoint used to obtain a bearer token.
	OAuthURL string
	// ClientID is the OAuth2 client identifier.
	ClientID string
	// ClientSecret is the OAuth2 client secret. Keep this value out of
	// source control; pass it via the RNV_CLIENT_SECRET environment variable.
	ClientSecret string
	// ResourceID is the OAuth2 audience / resource identifier required by
	// the RNV token endpoint.
	ResourceID string

	// ClientAPIURL is the GraphQL endpoint, e.g.
	// https://graphql-sandbox-dds.rnv-online.de/
	ClientAPIURL string

	// PollInterval controls how often the poller fetches fresh occupancy
	// data from the RNV API. Defaults to 30s.
	PollInterval time.Duration

	// ListenAddr is the TCP address the HTTP server binds to, e.g. ":8080".
	ListenAddr string
}

// Load reads configuration from environment variables and validates that all
// required values are present. It returns an error if any required variable
// is missing or if the resulting configuration is otherwise invalid.
//
// Required environment variables:
//
//	RNV_OAUTH_URL        – OAuth2 token endpoint
//	RNV_CLIENT_ID        – OAuth2 client id
//	RNV_CLIENT_SECRET    – OAuth2 client secret
//	RNV_RESOURCE_ID      – OAuth2 resource/audience
//	RNV_API_URL          – GraphQL endpoint
//
// Optional environment variables:
//
//	RNV_POLL_INTERVAL    – polling interval as a Go duration string, e.g. "60s" (default: "30s")
//	RNV_LISTEN_ADDR      – HTTP listen address (default: ":8080")
func Load() (*Config, error) {
	cfg := &Config{
		OAuthURL:     requiredEnv("RNV_OAUTH_URL"),
		ClientID:     requiredEnv("RNV_CLIENT_ID"),
		ClientSecret: requiredEnv("RNV_CLIENT_SECRET"),
		ResourceID:   requiredEnv("RNV_RESOURCE_ID"),
		ClientAPIURL: requiredEnv("RNV_API_URL"),

		PollInterval: envDuration("RNV_POLL_INTERVAL", 30*time.Second),
		ListenAddr:   envStr("RNV_LISTEN_ADDR", ":8080"),
	}

	return cfg, cfg.validate()
}

// validate returns an error if any required configuration field is empty.
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

// requiredEnv returns the value of the named environment variable.
// The caller is responsible for checking that the returned value is non-empty.
func requiredEnv(key string) string {
	return os.Getenv(key)
}

// envStr returns the value of the named environment variable, falling back to
// def if the variable is unset or empty.
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envDuration parses the named environment variable as a Go duration string
// (e.g. "30s", "2m"). If the variable is unset, empty, or unparseable, def
// is returned silently.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
