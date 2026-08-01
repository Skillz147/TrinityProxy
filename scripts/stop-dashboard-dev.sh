#!/usr/bin/env bash
#
# Stop TrinityProxy dev servers started by make start-dev.
# Usage: make stop   (or: ./scripts/stop-dashboard-dev.sh)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="$ROOT/.dev"
API_PID_FILE="$PID_DIR/dashboard-api.pid"
VITE_PID_FILE="$PID_DIR/dashboard-vite.pid"
CONTROLLER_PID_FILE="$PID_DIR/controller-api.pid"
DASHBOARD_PORT="${DASHBOARD_PORT:-8081}"
VITE_PORT="${VITE_PORT:-8080}"
CONTROLLER_PORT="${CONTROLLER_PORT:-3100}"

stopped=0

stop_pid_file() {
	local file=$1
	local label=$2
	if [[ ! -f "$file" ]]; then
		return 0
	fi

	local pid
	pid="$(cat "$file")"
	if kill -0 "$pid" 2>/dev/null; then
		echo "[*] Stopping $label (PID $pid)..."
		kill "$pid" 2>/dev/null || true
		pkill -P "$pid" 2>/dev/null || true
		stopped=1
	fi
	rm -f "$file"
}

stop_port() {
	local port=$1
	local label=$2
	local pids
	pids="$(lsof -ti:"$port" 2>/dev/null || true)"
	if [[ -z "$pids" ]]; then
		return 0
	fi
	echo "[*] Stopping $label on :$port..."
	kill $pids 2>/dev/null || true
	stopped=1
}

stop_pid_file "$CONTROLLER_PID_FILE" "controller API"
stop_pid_file "$API_PID_FILE" "dashboard API"
stop_pid_file "$VITE_PID_FILE" "dashboard UI"
stop_port "$CONTROLLER_PORT" "controller API"
stop_port "$VITE_PORT" "Vite UI"
stop_port "$DASHBOARD_PORT" "dashboard API"

if [[ "$stopped" -eq 1 ]]; then
	echo "[+] Dev servers stopped."
else
	echo "[*] No dev servers were running."
fi
