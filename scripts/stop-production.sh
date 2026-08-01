#!/usr/bin/env bash
#
# Stop TrinityProxy production systemd services.
#
# Usage: make stop-production   (or: ./scripts/stop-production.sh)

set -euo pipefail

if ! command -v systemctl >/dev/null 2>&1; then
	echo "[!] systemd not found — nothing to stop."
	exit 0
fi

stopped=0

stop_unit() {
	local unit=$1
	if systemctl list-unit-files "$unit.service" >/dev/null 2>&1; then
		if systemctl is-active "$unit" >/dev/null 2>&1; then
			echo "[*] Stopping $unit..."
			sudo systemctl stop "$unit"
			stopped=1
		fi
	fi
}

stop_unit trinityproxy-dashboard
stop_unit trinityproxy-controller

if [[ "$stopped" -eq 1 ]]; then
	echo "[+] Production services stopped."
else
	echo "[*] No production services were running."
fi
