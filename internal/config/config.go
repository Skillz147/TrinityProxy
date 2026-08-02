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
// Agent key flow (per-node tokens):
//
//  1. Dashboard generates and persists enrollment_key in dashboard.db
//     (table dashboard_deployment, column agent_key).
//  2. Controller API receives TRINITY_ENROLLMENT_KEY (or legacy TRINITY_AGENT_KEY)
//     to validate first-time agent enrollment on POST /api/heartbeat.
//  3. On first heartbeat, controller issues a unique node_token stored hashed in
//     proxy_nodes.token_hash and returns it in the heartbeat response.
//  4. Agents persist node_token locally and send it via Bearer / X-Agent-Token
//     on subsequent heartbeats, command polls, and command results.
//  5. Bridge dashboard → controller: `make sync-agent-key` writes enrollment key
//     to .env.controller as TRINITY_ENROLLMENT_KEY.
type Config struct {
	ControllerURL     string
	APIPort           int
	DBPath            string
	HeartbeatInterval time.Duration
	ProbeInterval     time.Duration
	APIKey            string
	AgentKey          string // legacy TRINITY_AGENT_KEY (deprecated)
	EnrollmentKey     string // TRINITY_ENROLLMENT_KEY
	NodeToken         string // TRINITY_NODE_TOKEN (persisted per-node token)
	APIBindAddr       string
}

// Load reads configuration from environment variables with sensible defaults.
//
// Env vars: CONTROLLER_URL, API_PORT, API_BIND_ADDR, DB_PATH, HEARTBEAT_INTERVAL, PROBE_INTERVAL,
// TRINITY_API_KEY, TRINITY_AGENT_KEY, TRINITY_ENROLLMENT_KEY, TRINITY_NODE_TOKEN,
// TRINITY_ENV (production|prod), TRINITY_NONINTERACTIVE
func Load() Config {
	return Config{
		ControllerURL:     envString("CONTROLLER_URL", defaultControllerURL),
		APIPort:           envInt("API_PORT", defaultAPIPort),
		DBPath:            envString("DB_PATH", defaultDBPath),
		HeartbeatInterval: envDuration("HEARTBEAT_INTERVAL", defaultHeartbeatInterval),
		ProbeInterval:     envDuration("PROBE_INTERVAL", defaultProbeInterval),
		APIKey:            envString("TRINITY_API_KEY", ""),
		AgentKey:          envString("TRINITY_AGENT_KEY", ""),
		EnrollmentKey:     envString("TRINITY_ENROLLMENT_KEY", ""),
		NodeToken:         envString("TRINITY_NODE_TOKEN", ""),
		APIBindAddr:       envString("API_BIND_ADDR", defaultAPIBindAddr),
	}
}

// HeartbeatURL returns the full heartbeat endpoint derived from ControllerURL.
func (c Config) HeartbeatURL() string {
	base := strings.TrimRight(c.ControllerURL, "/")
	return base + "/api/heartbeat"
}

// DeregisterURL returns the agent deregister endpoint derived from ControllerURL.
func (c Config) DeregisterURL() string {
	base := strings.TrimRight(c.ControllerURL, "/")
	return base + "/api/deregister"
}

// AgentCommandsURL returns the agent command poll endpoint.
func (c Config) AgentCommandsURL() string {
	base := strings.TrimRight(c.ControllerURL, "/")
	return base + "/api/agent/commands"
}

// CommandResultURL returns the agent command result ACK endpoint.
func (c Config) CommandResultURL() string {
	base := strings.TrimRight(c.ControllerURL, "/")
	return base + "/api/agent/command-result"
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

// ProbeLocalFallback reports whether SOCKS probes may retry via 127.0.0.1 when the
// agent's reported WAN IP is unreachable (NAT hairpin, same-host dev agents).
// Controlled by TRINITY_PROBE_LOCAL_FALLBACK; defaults to enabled unless
// TRINITY_ENV is production/prod. TRINITY_NONINTERACTIVE does not affect this.
func (c Config) ProbeLocalFallback() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRINITY_PROBE_LOCAL_FALLBACK"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRINITY_ENV"))) {
	case "production", "prod":
		return false
	}
	return true
}

// LogSecurityWarnings emits startup warnings when auth keys are unset.
func (c Config) LogSecurityWarnings(log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	enrollment := c.EnrollmentKey
	if enrollment == "" {
		enrollment = c.AgentKey
	}
	if enrollment == "" {
		if c.IsProduction() {
			log.Warn("TRINITY_ENROLLMENT_KEY unset — heartbeats are unauthenticated; generate in dashboard Settings, then run: make sync-agent-key")
		} else {
			log.Warn("TRINITY_ENROLLMENT_KEY unset — heartbeat auth disabled (dev mode)")
		}
	} else if c.AgentKey != "" && c.EnrollmentKey == "" {
		log.Warn("TRINITY_AGENT_KEY is deprecated — set TRINITY_ENROLLMENT_KEY on controller and use per-node tokens")
	}
	if c.APIKey == "" && c.IsProduction() {
		log.Warn("TRINITY_API_KEY unset — client API endpoints are unauthenticated")
	}
	if c.IsProduction() && strings.HasPrefix(strings.ToLower(c.ControllerURL), "http://") {
		log.Warn("CONTROLLER_URL uses HTTP — use HTTPS in production so agents cannot be MITM'd into executing injected remote commands")
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
