#!/usr/bin/env bash
#
# Start TrinityProxy local dev stack in one terminal:
#   - Dashboard API (:8081) + Vite UI (:8080)
#   - Controller API (:3100) with isolated .dev/ secrets
#
# Usage: make start-dev   (or: ./scripts/start-dashboard-dev.sh)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" == "Darwin" ]] && [[ $EUID -eq 0 ]]; then
	echo "[-] Error: do not run 'sudo make start-dev' on macOS."
	echo "    Local dev runs as your user: make start-dev"
	echo "    Production bootstrap runs on your Linux VPS: ssh user@vps 'cd TrinityProxy && sudo make start'"
	exit 1
fi

# shellcheck source=scripts/lib/dev-ports.sh
source "$ROOT/scripts/lib/dev-ports.sh"
# shellcheck source=scripts/lib/dev-env.sh
source "$ROOT/scripts/lib/dev-env.sh"
DEV_ENV_ROOT="$ROOT"
dev_env_apply
dev_env_print_banner

# Restore UI dist ownership before npm/go build (sudo make start leaves root-owned dirs).
chmod +x scripts/fix-dev-permissions.sh scripts/lib/dev-ui-permissions.sh 2>/dev/null || true
./scripts/fix-dev-permissions.sh --auto 2>/dev/null || true

PID_DIR="$DEV_DIR"
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
DASHBOARD_DB="$DASHBOARD_DB_PATH"

mkdir -p "$PID_DIR"

cleanup() {
	local code=$?
	if [[ "${TRINITY_START_CLEANUP_DONE:-}" == "1" ]]; then
		exit "$code"
	fi
	export TRINITY_START_CLEANUP_DONE=1
	echo ""
	echo "[*] Stopping dev servers..."
	stop_dev_pid_file "$CONTROLLER_PID_FILE" "controller API" || true
	stop_dev_pid_file "$API_PID_FILE" "dashboard API" || true
	stop_dev_pid_file "$VITE_PID_FILE" "dashboard UI" || true
	stop_port_force "$CONTROLLER_PORT" "controller API" || true
	stop_port_force "$VITE_PORT" "Vite UI" || true
	stop_port_force "$DASHBOARD_PORT" "dashboard API" || true
	exit "$code"
}

trap cleanup EXIT INT TERM

if dev_ports_in_use; then
	echo "[!] Ports :$VITE_PORT, :$DASHBOARD_PORT, or :$CONTROLLER_PORT are already in use."
	echo "[*] Stopping stale dev processes..."
	stop_dev_pid_file "$CONTROLLER_PID_FILE" "controller API" || true
	stop_dev_pid_file "$API_PID_FILE" "dashboard API" || true
	stop_dev_pid_file "$VITE_PID_FILE" "dashboard UI" || true
	stop_port_force "$CONTROLLER_PORT" "controller API" || true
	stop_port_force "$VITE_PORT" "Vite UI" || true
	stop_port_force "$DASHBOARD_PORT" "dashboard API" || true
	sleep 0.5
	if dev_ports_in_use; then
		echo "[-] Could not free dev ports. Run 'make stop', or manually kill processes on :$VITE_PORT, :$DASHBOARD_PORT, and :$CONTROLLER_PORT."
		echo "    Start the dev stack with: make start-dev   (hyphen, not 'make start dev')"
		exit 1
	fi
	echo "[+] Ports cleared."
fi

echo "[*] Building dev binaries for this machine..."
make build-dashboard
make build-main

# Rebuild controller if cross-compiled for another OS (e.g. Linux ELF on macOS).
if [[ -f "$CONTROLLER_BIN" ]]; then
	bin_type="$(file -b "$CONTROLLER_BIN" 2>/dev/null || true)"
	need_rebuild=0
	case "$(uname -s)" in
	Darwin)
		[[ "$bin_type" != *"Mach-O"* ]] && need_rebuild=1
		;;
	Linux)
		[[ "$bin_type" != *"ELF"* ]] && need_rebuild=1
		;;
	esac
	if [[ $need_rebuild -eq 1 ]]; then
		echo "[*] Rebuilding controller API for $(uname -s)/$(uname -m) (was: ${bin_type:-unknown})..."
		export PATH="/usr/local/go/bin:${PATH:-}"
		go build -o "$CONTROLLER_BIN" ./cmd/api
	fi
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
	DASHBOARD_DB_PATH="$DASHBOARD_DB" \
		DB_PATH="$DB_PATH" \
		CONTROLLER_URL="$CONTROLLER_URL" \
		TRINITY_AGENT_KEY="$TRINITY_AGENT_KEY" \
		"$DASHBOARD_BIN" --init-only
else
	echo "[*] Admin account ready."
fi

if command -v sqlite3 >/dev/null 2>&1 && [[ -f "$DASHBOARD_DB" ]]; then
	key="$(sqlite3 "$DASHBOARD_DB" "SELECT agent_key FROM dashboard_deployment WHERE id = 1;" 2>/dev/null || true)"
	key="${key//$'\r'/}"
	key="${key//$'\n'/}"
	if [[ -n "$key" ]]; then
		TRINITY_DEV=1 DASHBOARD_DB_PATH="$DASHBOARD_DB" CONTROLLER_ENV_FILE="$DEV_CONTROLLER_ENV" \
			./scripts/sync-agent-key.sh >/dev/null 2>&1 || true
	fi
fi

export PATH="/usr/local/go/bin:${PATH:-}"

echo "[*] Starting controller API on :$CONTROLLER_PORT..."
(
	set -a
	# shellcheck source=/dev/null
	. "$DEV_CONTROLLER_ENV"
	set +a
	TRINITY_DEV=1 \
		TRINITY_ENV=development \
		API_PORT="$CONTROLLER_PORT" \
		DB_PATH="$DB_PATH" \
		CONTROLLER_URL="$CONTROLLER_URL" \
		"$CONTROLLER_BIN"
) >>"$CONTROLLER_LOG" 2>&1 &
echo $! >"$CONTROLLER_PID_FILE"

echo "[*] Starting dashboard API on :$DASHBOARD_PORT..."
TRINITY_DEV=1 \
	TRINITY_ENV=development \
	DASHBOARD_PORT="$DASHBOARD_PORT" \
	DASHBOARD_DB_PATH="$DASHBOARD_DB" \
	DB_PATH="$DB_PATH" \
	CONTROLLER_URL="$CONTROLLER_URL" \
	TRINITY_AGENT_KEY="$TRINITY_AGENT_KEY" \
	TRINITY_CONTROLLER_ENV_PATH="/dev/null" \
	TRINITY_CADDY_SITE_PATH="/dev/null" \
	"$DASHBOARD_BIN" >>"$API_LOG" 2>&1 &
echo $! >"$API_PID_FILE"

echo "[*] Starting dashboard UI on :$VITE_PORT..."
(
	cd "$ROOT/web/dashboard"
	VITE_DEV_PORT="$VITE_PORT" npm run dev >>"$VITE_LOG" 2>&1 &
	echo $! >"$VITE_PID_FILE"
) &
VITE_WRAPPER_PID=$!

for _i in $(seq 1 30); do
	[[ -f "$VITE_PID_FILE" ]] && break
	sleep 0.1
done

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
	if [[ -f "$log" ]]; then
		tail -20 "$log" 2>/dev/null || true
	fi
	exit 1
}

wait_for_port "$CONTROLLER_PORT" "Controller API" "$CONTROLLER_LOG"
wait_for_port "$DASHBOARD_PORT" "Dashboard API" "$API_LOG"
wait_for_port "$VITE_PORT" "Dashboard UI" "$VITE_LOG"

echo ""
echo "============================================"
echo "  TrinityProxy LOCAL DEV is ready"
echo "  (not connected to production VPS)"
echo "============================================"
echo ""
echo "  Dashboard:   http://localhost:$VITE_PORT"
echo "  Controller:  http://localhost:$CONTROLLER_PORT"
echo ""
echo "  First time?"
echo "    1. Log in with the credentials above (if shown)"
echo "    2. Change your password when prompted"
echo "    3. Settings — use localhost for dev (not your VPS domain)"
echo ""
echo "  macOS agent dev: make run-agent-dev  (embedded SOCKS :1080)"
echo ""
echo "  Press Ctrl+C to stop."
echo "  Or run 'make stop' from another terminal."
echo ""

dev_server_alive() {
	local pid=$1
	kill -0 "$pid" 2>/dev/null
}

# macOS bash `wait` on PIDs from nested subshells is unreliable — poll instead.
while dev_server_alive "$(cat "$CONTROLLER_PID_FILE")" \
	&& dev_server_alive "$(cat "$API_PID_FILE")" \
	&& dev_server_alive "$(cat "$VITE_PID_FILE")"; do
	sleep 2
done

echo "[!] A dev server exited unexpectedly — check logs in $PID_DIR/"
exit 1
