package deployment

import (
	"fmt"
	"strings"
)

const (
	localControllerURL  = "http://127.0.0.1:3100"
	dockerControllerURL = "http://host.docker.internal:3100"
)

// DeployPlatform is one install target shown on the Deploy Agent page.
type DeployPlatform struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Description   string `json:"description"`
	Command       string `json:"command"`
	ControllerURL string `json:"controller_url"`
	RunAs         string `json:"run_as,omitempty"`
	Prerequisites string `json:"prerequisites,omitempty"`
}

// DeployCommands is the API payload for platform-specific install commands.
type DeployCommands struct {
	HasAgentKey             bool             `json:"has_agent_key"`
	SSLMode                 string           `json:"ssl_mode"`
	PublicDomain            string           `json:"public_domain"`
	ProductionControllerURL string           `json:"production_controller_url"`
	LocalControllerURL      string           `json:"local_controller_url"`
	Platforms               []DeployPlatform `json:"platforms"`
}

// BuildDeployCommands generates install commands for each supported platform.
func BuildDeployCommands(settings *Settings, envFallback, agentKey string) DeployCommands {
	productionURL := resolveProductionControllerURL(settings, envFallback)
	localURL := localControllerURL

	hasKey := agentKey != ""

	platforms := []DeployPlatform{
		{
			ID:            "linux-vps",
			Label:         "Linux VPS",
			Description:   "Production agent on a fresh Linux server. Installs systemd service, Dante SOCKS proxy, and registers with your controller.",
			ControllerURL: productionURL,
			Command:       linuxVPSCommand(productionURL, agentKey),
			RunAs:         "root",
			Prerequisites: "Ubuntu/Debian VPS with curl installed.",
		},
		{
			ID:            "macos",
			Label:         "macOS",
			Description:   "Install as a launchd service on your Mac with embedded SOCKS5 proxy (Go-based, no Dante).",
			ControllerURL: localURL,
			Command:       macOSCommand(localURL, agentKey),
			RunAs:         "your Mac user",
			Prerequisites: "Run from the TrinityProxy repo after make build. Uses make install-agent-macos or the shell script directly.",
		},
		{
			ID:            "windows",
			Label:         "Windows",
			Description:   "Install as a Windows service with embedded SOCKS5 proxy (Go-based, no Dante).",
			ControllerURL: productionURL,
			Command:       windowsCommand(productionURL, agentKey),
			RunAs:         "Administrator (elevated PowerShell)",
			Prerequisites: "Build trinityproxy.exe with make build-windows-agent, or set TRINITY_DOWNLOAD_URL.",
		},
		{
			ID:            "docker",
			Label:         "Docker (Mac dev)",
			Description:   "Run a Linux agent container on your Mac to simulate a VPS. Heartbeats reach the controller on your host.",
			ControllerURL: dockerControllerURL,
			Command:       dockerDevCommand(),
			Prerequisites: "Docker Desktop installed. Controller running locally. Run make sync-agent-key first.",
		},
		{
			ID:            "mac-dev",
			Label:         "Local dev (Mac)",
			Description:   "Foreground dev agent with embedded SOCKS on :1080 — no install, no Dante. Ideal while developing on macOS.",
			ControllerURL: localURL,
			Command:       macDevCommand(),
			Prerequisites: "Controller running locally. Run make sync-agent-key so .env.controller has TRINITY_AGENT_KEY.",
		},
	}

	return DeployCommands{
		HasAgentKey:             hasKey,
		SSLMode:                 settings.SSLMode,
		PublicDomain:            settings.PublicDomain,
		ProductionControllerURL: productionURL,
		LocalControllerURL:      localURL,
		Platforms:               platforms,
	}
}

func resolveProductionControllerURL(settings *Settings, envFallback string) string {
	if url := NormalizeControllerURL(settings.ControllerPublicURL, settings.SSLMode); url != "" {
		return url
	}
	if domain := NormalizeDomain(settings.PublicDomain); domain != "" {
		return DeriveControllerURL(domain, settings.SSLMode)
	}
	return strings.TrimRight(strings.TrimSpace(envFallback), "/")
}

func linuxVPSCommand(controllerURL, agentKey string) string {
	if agentKey == "" {
		return fmt.Sprintf(
			`# Save Settings first to generate an agent key, then refresh this page.
curl -fsSL https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-service.sh | CONTROLLER_URL=%q TRINITY_NONINTERACTIVE=1 bash`,
			controllerURL,
		)
	}
	return fmt.Sprintf(
		`curl -fsSL https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-service.sh | CONTROLLER_URL=%q TRINITY_AGENT_KEY=%q TRINITY_NONINTERACTIVE=1 bash`,
		controllerURL,
		agentKey,
	)
}

func macOSCommand(controllerURL, agentKey string) string {
	if agentKey == "" {
		return fmt.Sprintf(`# Save Settings first, then run:
make sync-agent-key
make install-agent-macos

# Or run the script directly (after make build):
CONTROLLER_URL=%q ./scripts/install-agent-macos.sh`, controllerURL)
	}
	return fmt.Sprintf(`make sync-agent-key
make install-agent-macos

# Or run the script directly (after make build):
CONTROLLER_URL=%q TRINITY_AGENT_KEY=%q ./scripts/install-agent-macos.sh`, controllerURL, agentKey)
}

func windowsCommand(controllerURL, agentKey string) string {
	if agentKey == "" {
		return fmt.Sprintf(`# Save Settings first to generate an agent key, then refresh this page.
# Run in elevated PowerShell from the TrinityProxy repo:
$env:TRINITY_NONINTERACTIVE = "1"
$env:CONTROLLER_URL = %q
.\\scripts\\install-agent-windows.ps1`, controllerURL)
	}
	return fmt.Sprintf(`# Run in elevated PowerShell from the TrinityProxy repo:
$env:TRINITY_NONINTERACTIVE = "1"
$env:CONTROLLER_URL = %q
$env:TRINITY_AGENT_KEY = %q
.\\scripts\\install-agent-windows.ps1`, controllerURL, agentKey)
}

func dockerDevCommand() string {
	return `make sync-agent-key
make docker-agent

# Logs:  docker logs -f trinityproxy-agent-dev
# Stop:  make docker-agent-down`
}

func macDevCommand() string {
	return `make sync-agent-key
make run-agent-dev`
}
