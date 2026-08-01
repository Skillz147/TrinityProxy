package dashboard

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/config"
)

const (
	defaultDashboardPort    = 8081 // API port; Vite dev UI uses :8080
	defaultDashboardBind    = "0.0.0.0"
	defaultDashboardDBPath  = "./dashboard.db"
	defaultSessionTTL       = 24 * time.Hour
	defaultViteURL          = "http://127.0.0.1:8080"
	defaultDashboardURL     = "http://localhost:8080"
)

// Config holds dashboard server settings.
type Config struct {
	Port           int
	BindAddr       string
	DBPath         string
	NodesDBPath    string
	SessionTTL     time.Duration
	DashboardURL   string
	ViteDevURL     string
	DevProxy       bool
	ControllerURL  string
	AgentKey       string
	AdminUsername  string
}

func LoadConfig() Config {
	devProxy := envBool("DASHBOARD_DEV_PROXY", false)
	viteURL := envString("DASHBOARD_VITE_URL", defaultViteURL)

	return Config{
		Port:          envInt("DASHBOARD_PORT", defaultDashboardPort),
		BindAddr:      envString("DASHBOARD_BIND_ADDR", defaultDashboardBind),
		DBPath:        envString("DASHBOARD_DB_PATH", defaultDashboardDBPath),
		NodesDBPath:   envString("DB_PATH", config.Load().DBPath),
		SessionTTL:    envDuration("DASHBOARD_SESSION_TTL", defaultSessionTTL),
		DashboardURL:  envString("DASHBOARD_URL", defaultDashboardURL),
		ViteDevURL:    viteURL,
		DevProxy:      devProxy,
		ControllerURL: envString("CONTROLLER_URL", config.Load().ControllerURL),
		AgentKey:      envString("TRINITY_AGENT_KEY", config.Load().AgentKey),
		AdminUsername: envString("DASHBOARD_ADMIN_USERNAME", "admin"),
	}
}

func (c Config) ListenAddr() string {
	host := c.BindAddr
	if host == "" {
		host = defaultDashboardBind
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
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

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
