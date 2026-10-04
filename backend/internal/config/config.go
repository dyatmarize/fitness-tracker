package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	ListenAddr     string        // HTTP API listen address
	AuthListenAddr string        // OAuth callback server listen address
	DatabaseURL    string        // PostgreSQL connection string
	CorosMCPURL    string        // COROS MCP endpoint
	AppToken       string        // optional bearer token protecting /api/*
	SyncInterval   time.Duration // sync worker interval
}

func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:     getenv("LISTEN_ADDR", ":8080"),
		AuthListenAddr: getenv("AUTH_LISTEN_ADDR", ":8787"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		CorosMCPURL:    getenv("COROS_MCP_URL", "https://mcp.coros.com/mcp"),
		AppToken:       os.Getenv("APP_TOKEN"),
	}
	var err error
	if cfg.SyncInterval, err = time.ParseDuration(getenv("SYNC_INTERVAL", "1h")); err != nil {
		return nil, fmt.Errorf("parse SYNC_INTERVAL: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
