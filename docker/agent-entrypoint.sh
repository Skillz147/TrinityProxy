#!/bin/bash
# Foreground Linux agent for local Docker dev (no systemd).
# Do not run on macOS — use: make docker-agent-dev (Linux container entrypoint).
set -euo pipefail

cd /app

echo "[*] TrinityProxy Docker agent — Linux dev container"
echo "[*] Controller: ${CONTROLLER_URL:-http://host.docker.internal:3100}"

export TRINITY_SOCKS_PORT="${TRINITY_SOCKS_PORT:-1080}"

start_dante() {
	if [[ ! -f /etc/danted.conf ]]; then
		return 0
	fi
	local dante=""
	dante="$(command -v danted 2>/dev/null || command -v sockd 2>/dev/null || true)"
	if [[ -z "$dante" ]]; then
		echo "[!] Dante not installed — heartbeat only"
		return 0
	fi
	# Avoid duplicate sockd if container restarts
	pkill -x "$(basename "$dante")" 2>/dev/null || true
	echo "[*] Starting Dante SOCKS proxy ($dante)..."
	"$dante" -f /etc/danted.conf &
}

# Debian dante-server may ship a stub /etc/danted.conf — remove if invalid so
# the installer writes a real config (internal + socksmethod).
if [[ -f /etc/danted.conf ]] && ! grep -qE '^internal:' /etc/danted.conf 2>/dev/null; then
	echo "[*] Removing invalid stub danted.conf..."
	rm -f /etc/danted.conf
fi

# One-time SOCKS setup (same as install-agent-service.sh → build/installer)
if [[ ! -f /etc/trinityproxy-port ]]; then
	echo "[*] Running one-time SOCKS installer..."
	./build/installer || true
fi

start_dante

export TRINITY_ROLE=agent
export TRINITY_NONINTERACTIVE=1
export TRINITY_ROOT=/app
export TRINITY_CONTAINER=1

echo "[*] Starting agent (heartbeats every ${HEARTBEAT_INTERVAL:-60s})..."
exec ./build/trinityproxy
