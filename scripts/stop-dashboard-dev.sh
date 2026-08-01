#!/usr/bin/env bash
#
# Stop TrinityProxy dev servers started by make start-dev.
# Usage: make stop   (or: ./scripts/stop-dashboard-dev.sh)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="$ROOT/.dev"
# shellcheck source=scripts/lib/dev-ports.sh
source "$ROOT/scripts/lib/dev-ports.sh"

stopped=0

if stop_dev_pid_file "$PID_DIR/controller-api.pid" "controller API"; then stopped=1; fi
if stop_dev_pid_file "$PID_DIR/dashboard-api.pid" "dashboard API"; then stopped=1; fi
if stop_dev_pid_file "$PID_DIR/dashboard-vite.pid" "dashboard UI"; then stopped=1; fi

if stop_port_force "$DEV_CONTROLLER_PORT" "controller API"; then stopped=1; fi
if stop_port_force "$DEV_VITE_PORT" "Vite UI"; then stopped=1; fi
if stop_port_force "$DEV_DASHBOARD_PORT" "dashboard API"; then stopped=1; fi

if [[ "$stopped" -eq 1 ]]; then
	echo "[+] Dev servers stopped."
else
	echo "[*] No dev servers were running."
fi
