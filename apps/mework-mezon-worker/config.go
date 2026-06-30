package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all configuration for the mework-mezon-worker binary.
type Config struct {
	MezonAppID      string
	MezonAPIKey     string
	MezonBaseURL    string
	MeworkServerURL string
	MeworkToken     string
	PollInterval    time.Duration
	CursorPath      string
}

// Load reads configuration from environment variables and returns a Config.
// Required vars: MEZON_APP_ID, MEZON_API_KEY, MEWORK_TOKEN.
// Optional vars: MEZON_BASE_URL, MEWORK_SERVER_URL (default http://localhost:8080),
// POLL_INTERVAL (default 5s), CURSOR_FILE (default ./.mework-mezon-cursor).
func Load() (*Config, error) {
	cfg := &Config{
		MezonAppID:      os.Getenv("MEZON_APP_ID"),
		MezonAPIKey:     os.Getenv("MEZON_API_KEY"),
		MezonBaseURL:    os.Getenv("MEZON_BASE_URL"),
		MeworkServerURL: os.Getenv("MEWORK_SERVER_URL"),
		MeworkToken:     os.Getenv("MEWORK_TOKEN"),
		PollInterval:    5 * time.Second,
		CursorPath:      "./.mework-mezon-cursor",
	}

	if cfg.MeworkServerURL == "" {
		cfg.MeworkServerURL = "http://localhost:8080"
	}

	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = d
	}

	if v := os.Getenv("CURSOR_FILE"); v != "" {
		cfg.CursorPath = v
	}

	var missing []string
	if cfg.MezonAppID == "" {
		missing = append(missing, "MEZON_APP_ID")
	}
	if cfg.MezonAPIKey == "" {
		missing = append(missing, "MEZON_API_KEY")
	}
	if cfg.MeworkToken == "" {
		missing = append(missing, "MEWORK_TOKEN")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
