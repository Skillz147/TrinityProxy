package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/config"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
)

const remoteCommandTimeout = 45 * time.Second

var allowedRemoteActions = map[string]struct{}{
	"uninstall": {},
	"restart":   {},
	"status":    {},
	"repair":    {},
}

// RemoteCommand is a command delivered by the controller heartbeat response.
type RemoteCommand struct {
	ID     string            `json:"id"`
	Action string            `json:"action"`
	Params map[string]string `json:"params,omitempty"`
}

// CommandOutcome is the result of executing a remote command locally.
type CommandOutcome struct {
	CommandID string
	Status    string
	Result    string
}

// ProcessPendingCommands executes commands from the heartbeat response.
func ProcessPendingCommands(cfg config.Config, agentKey string, meta *NodeMetadata, commands []RemoteCommand) {
	if len(commands) == 0 {
		return
	}
	log := logutil.New("agent")

	if meta == nil {
		var err error
		meta, err = GatherMetadata()
		if err != nil {
			log.Warn("failed to gather metadata for command reporting", "err", err)
		}
	}

	for _, cmd := range commands {
		log.Info("executing remote command", "id", cmd.ID, "action", cmd.Action)

		outcome := executeRemoteCommand(cmd)
		reportErr := postCommandResult(cfg, agentKey, meta, outcome)
		if reportErr != nil {
			log.Warn("failed to report command result", "id", cmd.ID, "status", outcome.Status, "err", reportErr)
		} else {
			log.Info("reported command result", "id", cmd.ID, "status", outcome.Status)
		}

		if strings.EqualFold(cmd.Action, "uninstall") && outcome.Status == "success" && meta != nil && reportErr == nil {
			if err := postNodePayload(cfg.DeregisterURL(), agentKey, *meta, cfg); err != nil {
				log.Warn("deregister after uninstall failed", "err", err)
			} else {
				log.Info("deregistered after uninstall", "ip", meta.IP, "port", meta.Port)
			}
		}
	}
}

func executeRemoteCommand(cmd RemoteCommand) CommandOutcome {
	outcome := CommandOutcome{CommandID: cmd.ID, Status: "success"}

	action := strings.ToLower(strings.TrimSpace(cmd.Action))
	if _, ok := allowedRemoteActions[action]; !ok {
		outcome.Status = "failure"
		outcome.Result = fmt.Sprintf("unknown action: %s", cmd.Action)
		return outcome
	}

	var output string
	var err error

	switch action {
	case "uninstall":
		output, err = runUninstall()
	case "restart":
		output, err = runRestart()
	case "status":
		output, err = runStatusCheck()
	case "repair":
		logLevel := "info"
		if cmd.Params != nil && cmd.Params["log_level"] != "" {
			logLevel = cmd.Params["log_level"]
		}
		output, err = runRepair(logLevel)
	}

	if err != nil {
		outcome.Status = "failure"
		if output != "" {
			outcome.Result = output + "\n" + err.Error()
		} else {
			outcome.Result = err.Error()
		}
		return outcome
	}
	outcome.Result = strings.TrimSpace(output)
	if outcome.Result == "" {
		outcome.Result = "ok"
	}
	return outcome
}

func runShell(script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteCommandTimeout)
	defer cancel()
	return runShellContext(ctx, script)
}

func runShellContext(ctx context.Context, script string) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	default:
		cmd = exec.CommandContext(ctx, "bash", "-c", script)
	}
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return strings.TrimSpace(string(out)), fmt.Errorf("command timed out after %s", remoteCommandTimeout)
	}
	return strings.TrimSpace(string(out)), err
}

func linuxManagedLocally() bool {
	if v := strings.TrimSpace(os.Getenv("TRINITY_CONTAINER")); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return inContainer() || embeddedSOCKSMode()
}

func runUninstall() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return runShell(windowsUninstallScript())
	case "darwin":
		return runShell(macosUninstallScript())
	case "linux":
		if linuxManagedLocally() {
			return runShell(linuxContainerUninstallScript())
		}
		return runShell(linuxUninstallScript())
	default:
		return "", fmt.Errorf("uninstall not supported on %s", runtime.GOOS)
	}
}

func runRestart() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return runShell(`Restart-Service -Name TrinityProxyAgent -ErrorAction Stop; Start-Sleep -Seconds 2; (Get-Service TrinityProxyAgent).Status`)
	case "darwin":
		return runShell(`launchctl kickstart -k "gui/$(id -u)/com.trinityproxy.agent" 2>&1 || launchctl bootout "gui/$(id -u)/com.trinityproxy.agent" 2>/dev/null; launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.trinityproxy.agent.plist" 2>&1`)
	case "linux":
		if linuxManagedLocally() {
			return runShell(linuxContainerRestartScript())
		}
		return runShell(`systemctl restart trinityproxy-agent && systemctl is-active trinityproxy-agent`)
	default:
		return "", fmt.Errorf("restart not supported on %s", runtime.GOOS)
	}
}

func runStatusCheck() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return runShell(windowsStatusScript())
	case "darwin":
		return runShell(darwinStatusScript())
	case "linux":
		if inContainer() || embeddedSOCKSMode() {
			return linuxDynamicStatus()
		}
		return runShell(linuxServiceStatusScript())
	default:
		return "", fmt.Errorf("status not supported on %s", runtime.GOOS)
	}
}

func linuxServiceStatusScript() string {
	return `systemctl status trinityproxy-agent --no-pager 2>&1 || true; PORT=$(cat /etc/trinityproxy-port 2>/dev/null || echo ""); if [ -n "$PORT" ]; then ss -tlnp | grep ":$PORT " || echo "Port $PORT is not listening"; else echo "No /etc/trinityproxy-port"; fi`
}

func darwinStatusScript() string {
	return `launchctl print "gui/$(id -u)/com.trinityproxy.agent" 2>/dev/null || echo "Agent not loaded"
PORT=$(defaults read "$HOME/Library/Preferences/com.trinityproxy.agent" SOCKSPort 2>/dev/null || echo "${TRINITY_SOCKS_PORT:-1080}")
if lsof -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | grep -q LISTEN; then
  echo "Port $PORT: LISTENING"
else
  echo "Port $PORT: NOT LISTENING"
fi`
}

func runRepair(logLevel string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		controllerURL := strings.TrimRight(os.Getenv("CONTROLLER_URL"), "/")
		enrollKey := os.Getenv("TRINITY_ENROLLMENT_KEY")
		if enrollKey == "" {
			enrollKey = os.Getenv("TRINITY_AGENT_KEY")
		}
		script := fmt.Sprintf(
			`$env:CONTROLLER_URL='%s'; $env:TRINITY_NONINTERACTIVE='1'; $env:TRINITY_LOG_LEVEL='%s'; if('%s' -ne ''){$env:TRINITY_ENROLLMENT_KEY='%s'}; $s=Join-Path $env:TEMP 'tp-install.ps1'; iwr -UseBasicParsing -Uri 'https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-windows.ps1' -OutFile $s; & $s`,
			controllerURL, logLevel, enrollKey, enrollKey,
		)
		return runShell(script)
	case "linux":
		if linuxManagedLocally() {
			return runShell(linuxContainerRepairScript(logLevel))
		}
		controllerURL := strings.TrimRight(os.Getenv("CONTROLLER_URL"), "/")
		enrollKey := os.Getenv("TRINITY_ENROLLMENT_KEY")
		if enrollKey == "" {
			enrollKey = os.Getenv("TRINITY_AGENT_KEY")
		}
		keyPart := ""
		if enrollKey != "" {
			keyPart = fmt.Sprintf(" TRINITY_ENROLLMENT_KEY=%q", enrollKey)
		}
		script := fmt.Sprintf(
			`curl -fsSL https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-service.sh | CONTROLLER_URL=%q TRINITY_LOG_LEVEL=%s TRINITY_NONINTERACTIVE=1%s bash`,
			controllerURL, logLevel, keyPart,
		)
		return runShell(script)
	case "darwin":
		return runShell(`make -C "${TRINITY_ROOT:-.}" install-agent-macos 2>&1 || echo "Repair requires TRINITY_ROOT repo path on macOS"`)
	default:
		return "", fmt.Errorf("repair not supported on %s", runtime.GOOS)
	}
}

func linuxUninstallScript() string {
	return `systemctl stop trinityproxy-agent 2>/dev/null || true
systemctl disable trinityproxy-agent 2>/dev/null || true
rm -f /etc/systemd/system/trinityproxy-agent.service
systemctl daemon-reload
rm -rf /var/lib/trinityproxy-agent
rm -f /etc/trinityproxy-username /etc/trinityproxy-password /etc/trinityproxy-port
echo "TrinityProxy agent removed"`
}

func linuxContainerUninstallScript() string {
	return `dante="$(command -v danted 2>/dev/null || command -v sockd 2>/dev/null || true)"
if [[ -n "$dante" ]]; then
  pkill -x "$(basename "$dante")" 2>/dev/null || true
fi
rm -f /etc/trinityproxy-port /etc/trinityproxy-username /etc/trinityproxy-password /etc/danted.conf
echo "TrinityProxy agent removed (container)"`
}

func linuxContainerRestartScript() string {
	return `dante="$(command -v danted 2>/dev/null || command -v sockd 2>/dev/null || true)"
if [[ -n "$dante" ]]; then
  pkill -x "$(basename "$dante")" 2>/dev/null || true
  if [[ -f /etc/danted.conf ]]; then
    "$dante" -f /etc/danted.conf &
    echo "Dante restarted"
  else
    echo "No danted.conf — SOCKS not configured"
  fi
else
  echo "Dante not installed — heartbeat agent still running"
fi`
}

func linuxContainerRepairScript(logLevel string) string {
	root := strings.TrimSpace(os.Getenv("TRINITY_ROOT"))
	if root == "" {
		root = "/app"
	}
	return fmt.Sprintf(`set -e
cd %q
TRINITY_LOG_LEVEL=%s TRINITY_NONINTERACTIVE=1 ./build/installer || true
dante="$(command -v danted 2>/dev/null || command -v sockd 2>/dev/null || true)"
if [[ -n "$dante" ]]; then
  pkill -x "$(basename "$dante")" 2>/dev/null || true
  if [[ -f /etc/danted.conf ]]; then
    "$dante" -f /etc/danted.conf &
  fi
fi
echo "Repair complete (container)"`, root, logLevel)
}

func macosUninstallScript() string {
	return `launchctl bootout "gui/$(id -u)/com.trinityproxy.agent" 2>/dev/null || true
rm -f "$HOME/Library/LaunchAgents/com.trinityproxy.agent.plist"
echo "TrinityProxy agent removed from this Mac"`
}

func windowsUninstallScript() string {
	return `$sn='TrinityProxyAgent'; $dir=Join-Path $env:ProgramFiles 'TrinityProxy'
if(Get-Service -Name $sn -EA SilentlyContinue){if((Get-Service $sn).Status -eq 'Running'){Stop-Service $sn -Force -EA SilentlyContinue}; sc.exe delete $sn | Out-Null; Start-Sleep -Seconds 2}
$t=Get-ScheduledTask -TaskName $sn -EA SilentlyContinue; if($t){Stop-ScheduledTask -TaskName $sn -EA SilentlyContinue; Unregister-ScheduledTask -TaskName $sn -Confirm:$false}
Get-NetFirewallRule -DisplayName 'TrinityProxy SOCKS5*' -EA SilentlyContinue | Remove-NetFirewallRule
if(Test-Path $dir){Remove-Item -LiteralPath $dir -Recurse -Force}
Write-Host 'TrinityProxy agent removed'`
}

func windowsStatusScript() string {
	return `$sn='TrinityProxyAgent'; $dir=Join-Path $env:ProgramFiles 'TrinityProxy'
Write-Host '=== TrinityProxy Agent Status ==='
$svc=Get-Service -Name $sn -EA SilentlyContinue
if($svc){Write-Host ('Service: ' + $svc.Status)}else{Write-Host 'Service: NOT INSTALLED'}
$portFile=Join-Path $dir 'trinityproxy-port'
if(Test-Path $portFile){$port=(Get-Content $portFile -Raw).Trim(); Write-Host ('SOCKS port: ' + $port); $conn=Get-NetTCPConnection -LocalPort ([int]$port) -State Listen -EA SilentlyContinue; if($conn){Write-Host ('Port ' + $port + ': LISTENING')}else{Write-Host ('Port ' + $port + ': NOT LISTENING')}}else{Write-Host 'Port file not found'}`
}
