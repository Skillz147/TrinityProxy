#!/usr/bin/env bash
#
# Start TrinityProxy local dev stack in one terminal:
#   - Dashboard API (:8081) + Vite UI (:8080)
#   - Controller API (:3100) with .env.controller
#
# Usage: make start   (or: ./scripts/start-dashboard-dev.sh)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PID_DIR="$ROOT/.dev"
API_PID_FILE="$PID_DIR/dashboard-api.pid"
VITE_PID_FILE="$PID_DIR/dashboard-vite.pid"
CONTROLLER_PID_FILE="$PID_DIR/controller-api.pid"
API_LOG="$PID_DIR/dashboard-api.log"
VITE_LOG="$PID_DIR/dashboard-vite.log"
CONTROLLER_LOG="$PID_DIR/controller-api.log"
DASHBOARD_BIN="$ROOT/build/trinityproxy-dashboard"
CONTROLLER_BIN="$ROOT/build/trinityproxy-api"
DASHBOARD_PORT="${DASHBOARD_PORT:-8081}"
VITE_PORT="${VITE_PORT:-8080}"
CONTROLLER_PORT="${CONTROLLER_PORT:-3100}"
DASHBOARD_DB="${DASHBOARD_DB_PATH:-./dashboard.db}"

mkdir -p "$PID_DIR"

stop_by_pid_file() {
	local file=$1
	if [[ -f "$file" ]]; then
		local pid
		pid="$(cat "$file")"
		if kill -0 "$pid" 2>/dev/null; then
			kill "$pid" 2>/dev/null || true
			pkill -P "$pid" 2>/dev/null || true
		fi
		rm -f "$file"
	fi
}

stop_by_port() {
	local port=$1
	local pids
	pids="$(lsof -ti:"$port" 2>/dev/null || true)"
	if [[ -n "$pids" ]]; then
		kill $pids 2>/dev/null || true
	fi
}

cleanup() {
	local code=$?
	if [[ "${TRINITY_START_CLEANUP_DONE:-}" == "1" ]]; then
		exit "$code"
	fi
	export TRINITY_START_CLEANUP_DONE=1
	echo ""
	echo "[*] Stopping dev servers..."
	stop_by_pid_file "$API_PID_FILE"
	stop_by_pid_file "$VITE_PID_FILE"
	stop_by_pid_file "$CONTROLLER_PID_FILE"
	stop_by_port "$VITE_PORT"
	stop_by_port "$DASHBOARD_PORT"
	stop_by_port "$CONTROLLER_PORT"
	exit "$code"
}

trap cleanup EXIT INT TERM

if lsof -ti:"$VITE_PORT" >/dev/null 2>&1 || lsof -ti:"$DASHBOARD_PORT" >/dev/null 2>&1 || lsof -ti:"$CONTROLLER_PORT" >/dev/null 2>&1; then
	echo "[!] Ports :$VITE_PORT, :$DASHBOARD_PORT, or :$CONTROLLER_PORT are already in use."
	echo "    Run 'make stop' first, then try again."
	exit 1
fi

echo "[*] Building dashboard API and controller API..."
make build-dashboard
if [[ ! -f "$CONTROLLER_BIN" ]]; then
	make build
fi

if [[ ! -d "$ROOT/web/dashboard/node_modules" ]]; then
	echo "[*] Installing dashboard UI dependencies (first time)..."
	(cd "$ROOT/web/dashboard" && npm install)
fi

needs_admin_init=1
if [[ -f "$DASHBOARD_DB" ]] && command -v sqlite3 >/dev/null 2>&1; then
	if sqlite3 "$DASHBOARD_DB" "SELECT 1 FROM dashboard_users LIMIT 1;" 2>/dev/null | grep -q 1; then
		needs_admin_init=0
	fi
fi

if [[ "$needs_admin_init" -eq 1 ]]; then
	echo "[*] Creating your admin login (shown once)..."
	"$DASHBOARD_BIN" --init-only
else
	echo "[*] Admin account ready."
fi

if command -v sqlite3 >/dev/null 2>&1 && [[ -f "$DASHBOARD_DB" ]]; then
	key="$(sqlite3 "$DASHBOARD_DB" "SELECT agent_key FROM dashboard_deployment WHERE id = 1;" 2>/dev/null || true)"
	key="${key//$'\r'/}"
	key="${key//$'\n'/}"
	if [[ -n "$key" ]]; then
		./scripts/sync-agent-key.sh >/dev/null 2>&1 || true
	fi
fi

export PATH="/usr/local/go/bin:${PATH:-}"

echo "[*] Starting controller API on :$CONTROLLER_PORT..."
(
	set -a
	if [[ -f "$ROOT/.env.controller" ]]; then
		# shellcheck source=/dev/null
		. "$ROOT/.env.controller"
	fi
	set +a
	API_PORT="$CONTROLLER_PORT" "$CONTROLLER_BIN"
) >>"$CONTROLLER_LOG" 2>&1 &
echo $! >"$CONTROLLER_PID_FILE"

echo "[*] Starting dashboard API on :$DASHBOARD_PORT..."
DASHBOARD_PORT="$DASHBOARD_PORT" "$DASHBOARD_BIN" >>"$API_LOG" 2>&1 &
echo $! >"$API_PID_FILE"

echo "[*] Starting dashboard UI on :$VITE_PORT..."
(
	cd "$ROOT/web/dashboard"
	VITE_DEV_PORT="$VITE_PORT" npm run dev >>"$VITE_LOG" 2>&1 &
	echo $! >"$VITE_PID_FILE"
)

wait_for_port() {
	local port=$1
	local label=$2
	local log=$3
	local i
	for i in $(seq 1 60); do
		if lsof -ti:"$port" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.5
	done
	echo "[-] $label failed to start on :$port"
	echo "    Check log: $log"
	exit 1
}

wait_for_port "$CONTROLLER_PORT" "Controller API" "$CONTROLLER_LOG"
wait_for_port "$DASHBOARD_PORT" "Dashboard API" "$API_LOG"
wait_for_port "$VITE_PORT" "Dashboard UI" "$VITE_LOG"

echo ""
echo "============================================"
echo "  TrinityProxy is ready"
echo "============================================"
echo ""
echo "  Dashboard:   http://localhost:$VITE_PORT"
echo "  Controller:  http://localhost:$CONTROLLER_PORT"
echo ""
echo "  First time?"
echo "    1. Log in with the credentials above (if shown)"
echo "    2. Change your password when prompted"
echo "    3. Settings — enter your domain and click Save"
echo "    4. Deploy Agent — copy the install command to your VPS"
echo ""
echo "  macOS agent dev: make run-agent-dev  (embedded SOCKS :1080)"
echo ""
echo "  Press Ctrl+C to stop."
echo "  Or run 'make stop' from another terminal."
echo ""

wait "$(cat "$CONTROLLER_PID_FILE")" 2>/dev/null || true
wait "$(cat "$API_PID_FILE")" 2>/dev/null || true
wait "$(cat "$VITE_PID_FILE")" 2>/dev/null || true
