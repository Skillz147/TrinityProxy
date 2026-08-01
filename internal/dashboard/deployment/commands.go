package deployment

import (
	"fmt"
	"strings"
)

const (
	githubRawInstallScript = "https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-windows.ps1"

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
			Description:   "One paste in elevated PowerShell: downloads the installer and pre-built trinityproxy-windows-amd64.exe from GitHub Releases (no Go on your PC). Falls back to a fresh source zip only if the release is not published yet.",
			ControllerURL: productionURL,
			Command:       windowsCommand(productionURL, agentKey),
			RunAs:         "Administrator (elevated PowerShell)",
			Prerequisites: "Elevated PowerShell only — no Git or Go required once GitHub Release latest is published. Optional: TRINITY_LOCAL_BINARY or TRINITY_DOWNLOAD_URL to use your own binary.",
		},
		{
			ID:            "docker",
			Label:         "Docker (Mac dev)",
			Description:   "Run a Linux agent container on your Mac to simulate a VPS. Heartbeats reach the controller on your host.",
			ControllerURL: dockerControllerURL,
			Command:       dockerDevCommand(),
			Prerequisites: "Docker Desktop installed. Run make start-dev (or make sync-agent-key) with controller running locally.",
		},
		{
			ID:            "mac-dev",
			Label:         "Local dev (Mac)",
			Description:   "Foreground dev agent with embedded SOCKS on :1080 — no install, no Dante. Ideal while developing on macOS.",
			ControllerURL: localURL,
			Command:       macDevCommand(),
			Prerequisites: "Run make start-dev in another terminal, then make sync-agent-key if needed.",
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

func psSingleQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func windowsBootstrapOneLiner(controllerURL, agentKey string) string {
	scriptURL := psSingleQuoted(githubRawInstallScript)
	ctrl := psSingleQuoted(controllerURL)
	inner := fmt.Sprintf("$env:CONTROLLER_URL=%s; $env:TRINITY_NONINTERACTIVE='1'", ctrl)
	if agentKey != "" {
		inner += fmt.Sprintf("; $env:TRINITY_AGENT_KEY=%s", psSingleQuoted(agentKey))
	}
	inner += fmt.Sprintf("; $s=Join-Path $env:TEMP 'tp-install.ps1'; iwr -UseBasicParsing -Uri %s -OutFile $s; & $s", scriptURL)
	return "& { " + inner + " }"
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
	line := windowsBootstrapOneLiner(controllerURL, agentKey)
	if agentKey == "" {
		return fmt.Sprintf(`# Save Settings first to generate an agent key, then refresh this page.
# Paste into elevated PowerShell (Run as administrator):
%s`, line)
	}
	return fmt.Sprintf(`# Paste into elevated PowerShell (Run as administrator):
%s`, line)
}

func dockerDevCommand() string {
	return `make start-dev    # controller on :3100 (another terminal)
make sync-agent-key
make docker-agent

# Logs:  docker logs -f trinityproxy-agent-dev
# Stop:  make docker-agent-down`
}

func macDevCommand() string {
	return `make start-dev    # controller on :3100 (another terminal)
make run-agent-dev`
}
