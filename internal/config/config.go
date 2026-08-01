package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIPort           = 3100
	defaultDBPath            = "./trinityproxy.db"
	defaultHeartbeatInterval = 60 * time.Second
	defaultProbeInterval     = 60 * time.Second
	defaultControllerURL     = ""
	defaultAPIBindAddr       = "0.0.0.0"
)

// Config holds runtime settings loaded from environment variables.
//
// Agent key flow (each master deployment has its own key):
//
//  1. Dashboard generates and persists agent_key in dashboard.db
//     (table dashboard_deployment, column agent_key).
//  2. Controller API must receive the same value as TRINITY_AGENT_KEY so
//     POST /api/heartbeat can validate agent heartbeats (cmd/api/enhanced_main.go).
//  3. Agents export TRINITY_AGENT_KEY and send it via X-API-Key or
//     Authorization: Bearer on every heartbeat.
//  4. Bridge dashboard → controller: `make sync-agent-key` reads dashboard.db
//     and writes .env.controller; source it or add TRINITY_AGENT_KEY to the
//     controller systemd unit before starting trinityproxy-api.
type Config struct {
	ControllerURL     string
	APIPort           int
	DBPath            string
	HeartbeatInterval time.Duration
	ProbeInterval     time.Duration
	APIKey            string
	AgentKey          string
	APIBindAddr       string
}

// Load reads configuration from environment variables with sensible defaults.
//
// Env vars: CONTROLLER_URL, API_PORT, API_BIND_ADDR, DB_PATH, HEARTBEAT_INTERVAL, PROBE_INTERVAL,
// TRINITY_API_KEY, TRINITY_AGENT_KEY, TRINITY_ENV (production|prod), TRINITY_NONINTERACTIVE
func Load() Config {
	return Config{
		ControllerURL:     envString("CONTROLLER_URL", defaultControllerURL),
		APIPort:           envInt("API_PORT", defaultAPIPort),
		DBPath:            envString("DB_PATH", defaultDBPath),
		HeartbeatInterval: envDuration("HEARTBEAT_INTERVAL", defaultHeartbeatInterval),
		ProbeInterval:     envDuration("PROBE_INTERVAL", defaultProbeInterval),
		APIKey:            envString("TRINITY_API_KEY", ""),
		AgentKey:          envString("TRINITY_AGENT_KEY", ""),
		APIBindAddr:       envString("API_BIND_ADDR", defaultAPIBindAddr),
	}
}

// HeartbeatURL returns the full heartbeat endpoint derived from ControllerURL.
func (c Config) HeartbeatURL() string {
	base := strings.TrimRight(c.ControllerURL, "/")
	return base + "/api/heartbeat"
}

// ListenAddr returns the API server bind host:port (default 0.0.0.0:3100).
func (c Config) ListenAddr() string {
	host := strings.TrimSpace(c.APIBindAddr)
	if host == "" {
		host = defaultAPIBindAddr
	}
	return fmt.Sprintf("%s:%d", host, c.APIPort)
}

// IsProduction reports whether the process runs in a non-dev deployment context.
// True when TRINITY_ENV is "production" or "prod", or TRINITY_NONINTERACTIVE=1
// (set by systemd controller/agent units).
func (c Config) IsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRINITY_ENV"))) {
	case "production", "prod":
		return true
	}
	return os.Getenv("TRINITY_NONINTERACTIVE") == "1"
}

// LogSecurityWarnings emits startup warnings when auth keys are unset.
func (c Config) LogSecurityWarnings(log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	if c.AgentKey == "" {
		if c.IsProduction() {
			log.Warn("TRINITY_AGENT_KEY unset — heartbeats are unauthenticated; generate in dashboard Settings, then run: make sync-agent-key")
		} else {
			log.Warn("TRINITY_AGENT_KEY unset — heartbeat auth disabled (dev mode)")
		}
	}
	if c.APIKey == "" && c.IsProduction() {
		log.Warn("TRINITY_API_KEY unset — client API endpoints are unauthenticated")
	}
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
