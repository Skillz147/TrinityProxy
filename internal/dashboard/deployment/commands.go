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

// RemoteCommand is a paste-and-run operation for a deployed agent (install, remove, etc.).
type RemoteCommand struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

// DeployPlatform is one install target shown on the Deploy Agent page.
type DeployPlatform struct {
	ID            string          `json:"id"`
	Label         string          `json:"label"`
	Description   string          `json:"description"`
	Command       string          `json:"command"`
	ControllerURL string          `json:"controller_url"`
	RunAs         string          `json:"run_as,omitempty"`
	Prerequisites string          `json:"prerequisites,omitempty"`
	Operations    []RemoteCommand `json:"operations,omitempty"`
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
func BuildDeployCommands(settings *Settings, envFallback, agentKey, logLevel string) DeployCommands {
	logLevel = NormalizeLogLevel(logLevel)
	productionURL := resolveProductionControllerURL(settings, envFallback)
	localURL := localControllerURL

	hasKey := agentKey != ""

	platforms := []DeployPlatform{
		withOperations(DeployPlatform{
			ID:            "linux-vps",
			Label:         "Linux VPS",
			Description:   "Production agent on a fresh Linux server. Installs systemd service, Dante SOCKS proxy, and registers with your controller.",
			ControllerURL: productionURL,
			Command:       linuxVPSCommand(productionURL, agentKey, logLevel),
			RunAs:         "root",
			Prerequisites: "Ubuntu/Debian VPS with curl installed.",
		}, linuxVPSOperations(productionURL, agentKey, logLevel)),
		withOperations(DeployPlatform{
			ID:            "macos",
			Label:         "macOS",
			Description:   "Install as a launchd service on your Mac with embedded SOCKS5 proxy (Go-based, no Dante).",
			ControllerURL: localURL,
			Command:       macOSCommand(localURL, agentKey, logLevel),
			RunAs:         "your Mac user",
			Prerequisites: "Run from the TrinityProxy repo after make build. Uses make install-agent-macos or the shell script directly.",
		}, macOSOperations(localURL, agentKey, logLevel)),
		withOperations(DeployPlatform{
			ID:            "windows",
			Label:         "Windows",
			Description:   "One paste in elevated PowerShell: downloads the installer and pre-built trinityproxy-windows-amd64.exe from GitHub Releases (no Go on your PC). Falls back to a fresh source zip only if the release is not published yet.",
			ControllerURL: productionURL,
			Command:       windowsCommand(productionURL, agentKey, logLevel),
			RunAs:         "Administrator (elevated PowerShell)",
			Prerequisites: "Elevated PowerShell only — no Git or Go required once GitHub Release latest is published. Optional: TRINITY_LOCAL_BINARY or TRINITY_DOWNLOAD_URL to use your own binary.",
		}, windowsOperations(productionURL, agentKey, logLevel)),
		withOperations(DeployPlatform{
			ID:            "docker",
			Label:         "Docker (Mac dev)",
			Description:   "Run a Linux agent container on your Mac to simulate a VPS. Heartbeats reach the controller on your host.",
			ControllerURL: dockerControllerURL,
			Command:       dockerDevCommand(),
			Prerequisites: "Docker Desktop installed. Run make start-dev (or make sync-agent-key) with controller running locally.",
		}, dockerOperations()),
		withOperations(DeployPlatform{
			ID:            "mac-dev",
			Label:         "Local dev (Mac)",
			Description:   "Foreground dev agent with embedded SOCKS on :1080 — no install, no Dante. Ideal while developing on macOS.",
			ControllerURL: localURL,
			Command:       macDevCommand(),
			Prerequisites: "Run make start-dev in another terminal, then make sync-agent-key if needed.",
		}, macDevOperations()),
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

func linuxVPSCommand(controllerURL, agentKey, logLevel string) string {
	if agentKey == "" {
		return fmt.Sprintf(
			`# Save Settings first to generate an agent key, then refresh this page.
curl -fsSL https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-service.sh | CONTROLLER_URL=%q TRINITY_LOG_LEVEL=%s TRINITY_NONINTERACTIVE=1 bash`,
			controllerURL,
			logLevel,
		)
	}
	return fmt.Sprintf(
		`curl -fsSL https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-service.sh | CONTROLLER_URL=%q TRINITY_AGENT_KEY=%q TRINITY_LOG_LEVEL=%s TRINITY_NONINTERACTIVE=1 bash`,
		controllerURL,
		agentKey,
		logLevel,
	)
}

func psSingleQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func windowsBootstrapOneLiner(controllerURL, agentKey, logLevel string) string {
	scriptURL := psSingleQuoted(githubRawInstallScript)
	ctrl := psSingleQuoted(controllerURL)
	inner := fmt.Sprintf("$env:CONTROLLER_URL=%s; $env:TRINITY_NONINTERACTIVE='1'; $env:TRINITY_LOG_LEVEL=%s", ctrl, psSingleQuoted(logLevel))
	if agentKey != "" {
		inner += fmt.Sprintf("; $env:TRINITY_AGENT_KEY=%s", psSingleQuoted(agentKey))
	}
	inner += fmt.Sprintf("; $s=Join-Path $env:TEMP 'tp-install.ps1'; iwr -UseBasicParsing -Uri %s -OutFile $s; & $s", scriptURL)
	return "& { " + inner + " }"
}

func macOSCommand(controllerURL, agentKey, logLevel string) string {
	if agentKey == "" {
		return fmt.Sprintf(`# Save Settings first, then run:
make sync-agent-key
make install-agent-macos

# Or run the script directly (after make build):
CONTROLLER_URL=%q TRINITY_LOG_LEVEL=%s ./scripts/install-agent-macos.sh`, controllerURL, logLevel)
	}
	return fmt.Sprintf(`make sync-agent-key
make install-agent-macos

# Or run the script directly (after make build):
CONTROLLER_URL=%q TRINITY_AGENT_KEY=%q TRINITY_LOG_LEVEL=%s ./scripts/install-agent-macos.sh`, controllerURL, agentKey, logLevel)
}

func windowsCommand(controllerURL, agentKey, logLevel string) string {
	line := windowsBootstrapOneLiner(controllerURL, agentKey, logLevel)
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

func withOperations(p DeployPlatform, ops []RemoteCommand) DeployPlatform {
	p.Operations = ops
	return p
}

func linuxVPSOperations(controllerURL, agentKey, logLevel string) []RemoteCommand {
	install := linuxVPSCommand(controllerURL, agentKey, logLevel)
	return []RemoteCommand{
		{
			ID:          "install",
			Label:       "Install",
			Description: "Fresh install on a Linux VPS via curl bootstrap script.",
			Command:     install,
		},
		{
			ID:          "remove",
			Label:       "Remove",
			Description: "Stop the systemd service, remove unit files, and delete agent state. Remove the node from the dashboard Agents page separately.",
			Command: `# Run as root on the VPS
systemctl stop trinityproxy-agent 2>/dev/null || true
systemctl disable trinityproxy-agent 2>/dev/null || true
rm -f /etc/systemd/system/trinityproxy-agent.service
systemctl daemon-reload
rm -rf /var/lib/trinityproxy-agent
rm -f /etc/trinityproxy-username /etc/trinityproxy-password /etc/trinityproxy-port
echo "TrinityProxy agent removed. Delete the node in the dashboard if it still appears."`,
		},
		{
			ID:          "repair",
			Label:       "Reinstall / repair",
			Description: "Re-run the install script to refresh the binary and restart the service.",
			Command:     install,
		},
		{
			ID:          "status",
			Label:       "Check status",
			Description: "Show systemd service state and whether the SOCKS port is listening.",
			Command: `# Run on the VPS
systemctl status trinityproxy-agent --no-pager || true
PORT=$(cat /etc/trinityproxy-port 2>/dev/null || echo "")
if [ -n "$PORT" ]; then
  ss -tlnp | grep ":$PORT " || echo "Port $PORT is not listening"
else
  echo "No /etc/trinityproxy-port — agent may not be installed"
fi`,
		},
	}
}

func macOSOperations(controllerURL, agentKey, logLevel string) []RemoteCommand {
	install := macOSCommand(controllerURL, agentKey, logLevel)
	return []RemoteCommand{
		{
			ID:          "install",
			Label:       "Install",
			Description: "Install or update the launchd LaunchAgent from the repo.",
			Command:     install,
		},
		{
			ID:          "remove",
			Label:       "Remove",
			Description: "Unload and delete the LaunchAgent plist.",
			Command: `launchctl bootout "gui/$(id -u)/com.trinityproxy.agent" 2>/dev/null || true
rm -f "$HOME/Library/LaunchAgents/com.trinityproxy.agent.plist"
echo "TrinityProxy agent removed from this Mac."`,
		},
		{
			ID:          "repair",
			Label:       "Reinstall / repair",
			Description: "Re-run the install script to refresh the binary and reload launchd.",
			Command:     install,
		},
		{
			ID:          "status",
			Label:       "Check status",
			Description: "Show launchd service state and SOCKS port.",
			Command: `launchctl print "gui/$(id -u)/com.trinityproxy.agent" 2>/dev/null || echo "Agent not loaded"
lsof -iTCP:1080 -sTCP:LISTEN 2>/dev/null || echo "Port 1080 not listening (check TRINITY_SOCKS_PORT if customized)"`,
		},
	}
}

func windowsRemoveOneLiner() string {
	return "& { $sn='TrinityProxyAgent'; $dir=Join-Path $env:ProgramFiles 'TrinityProxy'; if(Get-Service -Name $sn -EA SilentlyContinue){if((Get-Service $sn).Status -eq 'Running'){Stop-Service $sn -Force -EA SilentlyContinue}; sc.exe delete $sn | Out-Null; Start-Sleep -Seconds 2}; $t=Get-ScheduledTask -TaskName $sn -EA SilentlyContinue; if($t){Stop-ScheduledTask -TaskName $sn -EA SilentlyContinue; Unregister-ScheduledTask -TaskName $sn -Confirm:$false}; Get-NetFirewallRule -DisplayName 'TrinityProxy SOCKS5*' -EA SilentlyContinue | Remove-NetFirewallRule; if(Test-Path $dir){Remove-Item -LiteralPath $dir -Recurse -Force}; Write-Host 'TrinityProxy agent removed. Delete the node in the dashboard Agents page if needed.' }"
}

func windowsStatusOneLiner() string {
	return "& { $sn='TrinityProxyAgent'; $dir=Join-Path $env:ProgramFiles 'TrinityProxy'; Write-Host '=== TrinityProxy Agent Status ==='; $svc=Get-Service -Name $sn -EA SilentlyContinue; if($svc){Write-Host ('Service: ' + $svc.Status)}else{$t=Get-ScheduledTask -TaskName $sn -EA SilentlyContinue; if($t){Write-Host ('Scheduled task: ' + $t.State)}else{Write-Host 'Service: NOT INSTALLED'}}; $portFile=Join-Path $dir 'trinityproxy-port'; if(Test-Path $portFile){$port=(Get-Content $portFile -Raw).Trim(); Write-Host ('SOCKS port (config): ' + $port); $conn=Get-NetTCPConnection -LocalPort ([int]$port) -State Listen -EA SilentlyContinue; if($conn){Write-Host ('Port ' + $port + ': LISTENING')}else{Write-Host ('Port ' + $port + ': NOT LISTENING')}}else{Write-Host 'Install dir / port file not found'}; if(Test-Path $dir){Write-Host ('Install dir: ' + $dir + ' (exists)')}else{Write-Host ('Install dir: ' + $dir + ' (missing)')} }"
}

func windowsOperations(controllerURL, agentKey, logLevel string) []RemoteCommand {
	installLine := windowsBootstrapOneLiner(controllerURL, agentKey, logLevel)
	install := fmt.Sprintf("# Paste into elevated PowerShell (Run as administrator):\n%s", installLine)
	repair := install
	if agentKey == "" {
		install = fmt.Sprintf(`# Save Settings first to generate an agent key, then refresh this page.
# Paste into elevated PowerShell (Run as administrator):
%s`, installLine)
		repair = install
	}
	return []RemoteCommand{
		{
			ID:          "install",
			Label:       "Install",
			Description: "Download installer and register the TrinityProxyAgent Windows service.",
			Command:     install,
		},
		{
			ID:          "remove",
			Label:       "Remove / uninstall",
			Description: "Stop TrinityProxyAgent, delete the service, remove Program Files, and delete firewall rules for the SOCKS port.",
			Command: fmt.Sprintf(`# Paste into elevated PowerShell (Run as administrator):
%s`, windowsRemoveOneLiner()),
		},
		{
			ID:          "repair",
			Label:       "Reinstall / repair",
			Description: "Re-download the binary, refresh config, and restart the Windows service.",
			Command:     repair,
		},
		{
			ID:          "status",
			Label:       "Check status",
			Description: "Query whether TrinityProxyAgent is running and the configured SOCKS port is listening.",
			Command: fmt.Sprintf(`# Paste into elevated PowerShell (Run as administrator):
%s`, windowsStatusOneLiner()),
		},
	}
}

func dockerOperations() []RemoteCommand {
	install := dockerDevCommand()
	return []RemoteCommand{
		{
			ID:          "install",
			Label:       "Install / start",
			Description: "Start the dev agent container.",
			Command:     install,
		},
		{
			ID:          "remove",
			Label:       "Remove",
			Description: "Stop and remove the dev agent container.",
			Command: `make docker-agent-down
docker rm -f trinityproxy-agent-dev 2>/dev/null || true`,
		},
		{
			ID:          "repair",
			Label:       "Reinstall / repair",
			Description: "Recreate the dev agent container.",
			Command: `make docker-agent-down
make sync-agent-key
make docker-agent`,
		},
		{
			ID:          "status",
			Label:       "Check status",
			Description: "Show container state and recent logs.",
			Command: `docker ps -a --filter name=trinityproxy-agent-dev
docker logs --tail 30 trinityproxy-agent-dev 2>/dev/null || echo "Container not running"`,
		},
	}
}

func macDevOperations() []RemoteCommand {
	install := macDevCommand()
	return []RemoteCommand{
		{
			ID:          "install",
			Label:       "Start",
			Description: "Run the foreground dev agent.",
			Command:     install,
		},
		{
			ID:          "remove",
			Label:       "Stop",
			Description: "Stop the foreground dev agent (Ctrl+C in its terminal).",
			Command: `# In the terminal running make run-agent-dev, press Ctrl+C
# Or kill by port:
lsof -tiTCP:1080 -sTCP:LISTEN | xargs kill 2>/dev/null || echo "No agent on :1080"`,
		},
		{
			ID:          "repair",
			Label:       "Restart",
			Description: "Stop any dev agent on :1080 and start fresh.",
			Command: `lsof -tiTCP:1080 -sTCP:LISTEN | xargs kill 2>/dev/null || true
make sync-agent-key
make run-agent-dev`,
		},
		{
			ID:          "status",
			Label:       "Check status",
			Description: "Check if the dev agent is listening on :1080.",
			Command: `lsof -iTCP:1080 -sTCP:LISTEN 2>/dev/null || echo "Dev agent not listening on :1080"`,
		},
	}
}
