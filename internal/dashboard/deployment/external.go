package deployment

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	DefaultCaddySitePath     = "/etc/caddy/trinityproxy.caddy"
	DefaultControllerEnvPath = "/etc/trinityproxy/controller.env"
)

var (
	caddySiteHeaderRE    = regexp.MustCompile(`(?m)^\*\.([a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+),\s*([a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+)\s*\{`)
	caddyDashboardHostRE = regexp.MustCompile(`(?m)^\s*@dashboard host ([a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+)`)
)

// ExternalSources locates deployment metadata written by VPS setup scripts.
type ExternalSources struct {
	CaddySitePath     string
	ControllerEnvPath string
	CaddyActiveCheck  func() bool
}

func DefaultExternalSources() ExternalSources {
	return ExternalSources{
		CaddySitePath:     DefaultCaddySitePath,
		ControllerEnvPath: DefaultControllerEnvPath,
	}
}

// ExternalSnapshot is deployment metadata discovered outside dashboard.db.
type ExternalSnapshot struct {
	PublicDomain  string
	ControllerURL string
	SSLMode       string
}

func (s ExternalSources) caddySitePath() string {
	if strings.TrimSpace(s.CaddySitePath) != "" {
		return s.CaddySitePath
	}
	return DefaultCaddySitePath
}

func (s ExternalSources) controllerEnvPath() string {
	if strings.TrimSpace(s.ControllerEnvPath) != "" {
		return s.ControllerEnvPath
	}
	return DefaultControllerEnvPath
}

func (s ExternalSources) caddyActive() bool {
	if s.CaddyActiveCheck != nil {
		return s.CaddyActiveCheck()
	}
	return systemctlCaddyActive()
}

func systemctlCaddyActive() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	cmd := exec.Command("systemctl", "is-active", "--quiet", "caddy")
	return cmd.Run() == nil
}

// ReadCaddySiteDomain parses the apex domain from a TrinityProxy Caddy site file.
func ReadCaddySiteDomain(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := string(data)
	if m := caddySiteHeaderRE.FindStringSubmatch(text); len(m) == 3 {
		wildcard := NormalizeDomain(m[1])
		apex := NormalizeDomain(m[2])
		if wildcard != "" && wildcard == apex {
			return apex, true
		}
	}
	if m := caddyDashboardHostRE.FindStringSubmatch(text); len(m) == 2 {
		return NormalizeDomain(m[1]), true
	}
	return "", false
}

func caddySiteUsesTLS(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "tls {") && strings.Contains(string(data), "dns cloudflare")
}

// ReadControllerEnvDeployment reads PUBLIC_DOMAIN and CONTROLLER_URL from controller.env.
func ReadControllerEnvDeployment(path string) (publicDomain, controllerURL string) {
	values, err := parseEnvFile(path)
	if err != nil {
		return "", ""
	}

	publicDomain = NormalizeDomain(values["PUBLIC_DOMAIN"])
	controllerURL = strings.TrimSpace(values["CONTROLLER_URL"])

	if publicDomain == "" && controllerURL != "" {
		if host := hostFromURL(controllerURL); strings.HasPrefix(host, "api.") {
			publicDomain = NormalizeDomain(strings.TrimPrefix(host, "api."))
		}
	}

	return publicDomain, controllerURL
}

func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func hostFromURL(raw string) string {
	return hostFromControllerURL(raw)
}

// DiscoverExternal merges Caddy site config and controller.env into one snapshot.
func DiscoverExternal(src ExternalSources) ExternalSnapshot {
	var snap ExternalSnapshot

	caddyPath := src.caddySitePath()
	if domain, ok := ReadCaddySiteDomain(caddyPath); ok {
		snap.PublicDomain = domain
	}

	envDomain, envControllerURL := ReadControllerEnvDeployment(src.controllerEnvPath())
	if snap.PublicDomain == "" {
		snap.PublicDomain = envDomain
	}
	if envControllerURL != "" {
		snap.ControllerURL = envControllerURL
	}

	if snap.SSLMode == "" {
		if src.caddyActive() && snap.PublicDomain != "" && fileExists(caddyPath) && caddySiteUsesTLS(caddyPath) {
			snap.SSLMode = SSLModeCaddy
		} else {
			snap.SSLMode = SSLModeNone
		}
	}

	if snap.PublicDomain == "" {
		return snap
	}

	if snap.SSLMode == SSLModeCaddy {
		snap.ControllerURL = DeriveControllerURL(snap.PublicDomain, SSLModeCaddy)
	} else if snap.ControllerURL == "" {
		snap.ControllerURL = DeriveControllerURL(snap.PublicDomain, SSLModeNone)
	}

	return snap
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
