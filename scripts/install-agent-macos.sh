#!/bin/bash
#
# Install TrinityProxy Agent as a macOS launchd service (user LaunchAgent).
# Embedded SOCKS mode — no Dante (TRINITY_SKIP_INSTALLER=1).
#
# Required env (non-interactive):
#   CONTROLLER_URL   — controller base URL (e.g. http://127.0.0.1:3100)
# Optional env:
#   TRINITY_AGENT_KEY — heartbeat auth key (must match controller)
#   TRINITY_DEVICE_CLASS — desktop|vps (default: desktop)
#   TRINITY_SOCKS_PORT — embedded SOCKS listen port (default: 1080)
#   TRINITY_LOG_LEVEL — quiet | silent | info | debug (default: info)
#
# Usage:
#   CONTROLLER_URL=http://127.0.0.1:3100 TRINITY_AGENT_KEY=... ./scripts/install-agent-macos.sh

set -euo pipefail

TRINITY_LOG_LEVEL="${TRINITY_LOG_LEVEL:-info}"
TRINITY_LOG_LEVEL="$(echo "$TRINITY_LOG_LEVEL" | tr '[:upper:]' '[:lower:]')"

case "$TRINITY_LOG_LEVEL" in
	quiet|total-silent|totalsilent) TRINITY_LOG_LEVEL="quiet" ;;
	silent|info|debug) ;;
	*)
		echo "[!] Invalid TRINITY_LOG_LEVEL '$TRINITY_LOG_LEVEL' (use: quiet, silent, info, debug)" >&2
		exit 1
		;;
esac

log_at_least() {
	local min=$1
	case "$TRINITY_LOG_LEVEL" in
		quiet) [[ "$min" == "quiet" ]] ;;
		silent) [[ "$min" == "quiet" || "$min" == "silent" ]] ;;
		info) [[ "$min" != "debug" ]] ;;
		debug) true ;;
	esac
}

log_debug() {
	[[ "$TRINITY_LOG_LEVEL" == "debug" ]] && echo "[debug] $*" >&2 || true
}

log_info() {
	log_at_least info && echo "[*] $*" || true
}

log_ok() {
	log_at_least info && echo "[+] $*" || true
}

log_warn() {
	[[ "$TRINITY_LOG_LEVEL" != "quiet" ]] && echo "[!] $*" >&2 || true
}

fail() {
	echo "[!] $*" >&2
	exit 1
}

log_start() {
	case "$TRINITY_LOG_LEVEL" in
		quiet) ;;
		silent) echo "[*] TrinityProxy: installing..." ;;
		info|debug)
			echo "[*] Installing TrinityProxy Agent as launchd service..."
			;;
	esac
}

log_done() {
	case "$TRINITY_LOG_LEVEL" in
		quiet) ;;
		silent) echo "[+] TrinityProxy: setup complete" ;;
		info|debug)
			echo "[+] TrinityProxy Agent installed successfully!"
			echo ""
			echo "Service Management:"
			echo "  Status:  launchctl print gui/$(id -u)/$LABEL"
			echo "  Logs:    tail -f ${LOG_DIR}/${SERVICE_NAME}.log"
			echo "  Stop:    launchctl bootout gui/$(id -u)/$LABEL"
			echo "  Start:   launchctl bootstrap gui/$(id -u) $PLIST_PATH"
			echo ""
			echo "Plist: $PLIST_PATH"
			;;
	esac
}

LABEL="com.trinityproxy.agent"
SERVICE_NAME="trinityproxy-agent"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BINARY="$PROJECT_ROOT/build/trinityproxy"
LOG_DIR="$PROJECT_ROOT/.dev"
PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"

if [[ "$(uname -s)" != "Darwin" ]]; then
	fail "This script is for macOS only. On Linux, use: sudo scripts/install-agent-service.sh"
fi

if [[ ! -f "$BINARY" ]]; then
	fail "Binary not found at $BINARY — run 'make build' or 'make build-darwin-agent' first."
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
if [[ -z "$CONTROLLER_URL" ]]; then
	fail "CONTROLLER_URL is required (e.g. CONTROLLER_URL=http://127.0.0.1:3100 $0)"
fi

TRINITY_ENROLLMENT_KEY="${TRINITY_ENROLLMENT_KEY:-${TRINITY_AGENT_KEY:-}}"
TRINITY_DEVICE_CLASS="${TRINITY_DEVICE_CLASS:-desktop}"
TRINITY_SOCKS_PORT="${TRINITY_SOCKS_PORT:-1080}"

log_start
log_info "Controller: $CONTROLLER_URL"
log_info "Device class: $TRINITY_DEVICE_CLASS"
log_info "Embedded SOCKS port: $TRINITY_SOCKS_PORT"
log_debug "Binary=$BINARY plist=$PLIST_PATH log_level=$TRINITY_LOG_LEVEL"
if [[ -z "$TRINITY_ENROLLMENT_KEY" ]]; then
	log_warn "TRINITY_ENROLLMENT_KEY unset — heartbeats will be unauthenticated (dev mode)"
fi

mkdir -p "$LOG_DIR" "$HOME/Library/LaunchAgents"

# Stop and unload existing agent if present
if launchctl print "gui/$(id -u)/$LABEL" &>/dev/null; then
	log_info "Stopping existing $LABEL service..."
	launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
fi

# Build EnvironmentVariables dict for plist
ENV_XML=""
append_env() {
	local key=$1 val=$2
	# Escape XML special chars in value
	val="${val//&/&amp;}"
	val="${val//</&lt;}"
	val="${val//>/&gt;}"
	val="${val//\"/&quot;}"
	ENV_XML="${ENV_XML}
		<key>${key}</key>
		<string>${val}</string>"
}

append_env "TRINITY_ROLE" "agent"
append_env "TRINITY_NONINTERACTIVE" "1"
append_env "TRINITY_SKIP_INSTALLER" "1"
append_env "CONTROLLER_URL" "$CONTROLLER_URL"
append_env "TRINITY_DEVICE_CLASS" "$TRINITY_DEVICE_CLASS"
append_env "TRINITY_SOCKS_PORT" "$TRINITY_SOCKS_PORT"
append_env "TRINITY_ROOT" "$PROJECT_ROOT"
if [[ -n "$TRINITY_ENROLLMENT_KEY" ]]; then
	append_env "TRINITY_ENROLLMENT_KEY" "$TRINITY_ENROLLMENT_KEY"
fi

cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>${LABEL}</string>
	<key>ProgramArguments</key>
	<array>
		<string>${BINARY}</string>
	</array>
	<key>WorkingDirectory</key>
	<string>${PROJECT_ROOT}</string>
	<key>EnvironmentVariables</key>
	<dict>${ENV_XML}
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>${LOG_DIR}/${SERVICE_NAME}.log</string>
	<key>StandardErrorPath</key>
	<string>${LOG_DIR}/${SERVICE_NAME}.err</string>
</dict>
</plist>
EOF

chmod 644 "$PLIST_PATH"
log_debug "Wrote plist to $PLIST_PATH"

log_info "Loading launchd service..."
launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH"
launchctl enable "gui/$(id -u)/$LABEL" 2>/dev/null || true
launchctl kickstart -k "gui/$(id -u)/$LABEL"

log_done
