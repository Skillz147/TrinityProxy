#!/bin/bash
#
# Install TrinityProxy Dashboard as a systemd service (requires root).
# Serves API + embedded UI on :8081 (no Node.js required on VPS).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

DASHBOARD_USER="trinityproxy"
DASHBOARD_GROUP="trinityproxy"
STATE_DIR="/var/lib/trinityproxy"

echo "[*] Installing TrinityProxy Dashboard as systemd service..."

if [[ $EUID -ne 0 ]]; then
	echo "[-] Error: run with sudo (installer needs root for /etc/ and systemd)"
	exit 1
fi

if [[ ! -f "cmd/dashboard/main.go" ]]; then
	echo "[-] Error: Run this from the TrinityProxy root directory"
	exit 1
fi

create_dashboard_user() {
	if id "$DASHBOARD_USER" &>/dev/null; then
		echo "[*] System user $DASHBOARD_USER already exists"
	fi
	production_ensure_trinityproxy_user
}

setup_state_dir() {
	echo "[*] Preparing state directory: $STATE_DIR"
	install -d -o "$DASHBOARD_USER" -g "$DASHBOARD_GROUP" -m 750 "$STATE_DIR"
}

setup_project_permissions() {
	local project_root
	project_root="$(pwd)"
	echo "[*] Setting project permissions for $DASHBOARD_USER"
	chmod o+rX "$project_root" "$project_root/build" 2>/dev/null || true
	chmod o+r "$project_root/build/trinityproxy-dashboard" 2>/dev/null || true
}

maybe_build_dashboard_ui() {
	if [[ -f build/trinityproxy-dashboard ]] && [[ -f cmd/dashboard/dist/index.html ]]; then
		echo "[+] Dashboard binary and embedded UI present — skipping UI build"
		return 0
	fi
	if [[ -f web/dashboard/dist/index.html ]]; then
		echo "[+] web/dashboard/dist present — syncing into go:embed tree"
		chmod +x scripts/build-dashboard-ui.sh
		./scripts/build-dashboard-ui.sh
		return 0
	fi
	if command -v npm >/dev/null 2>&1; then
		echo "[*] Building dashboard UI (npm)..."
		chmod +x scripts/build-dashboard-ui.sh
		./scripts/build-dashboard-ui.sh
		return 0
	fi
	if [[ -f cmd/dashboard/dist/index.html ]]; then
		echo "[+] Using existing cmd/dashboard/dist for go:embed"
		return 0
	fi
	echo "[*] No npm and no pre-built UI dist — binary must already embed UI from dev/CI build"
}

echo "[*] Building TrinityProxy dashboard binary..."
export PATH="/usr/local/go/bin:$PATH"
maybe_build_dashboard_ui
make build-dashboard

if [[ ! -f build/trinityproxy-dashboard ]]; then
	echo "[-] build/trinityproxy-dashboard missing — run 'make build' from repo root"
	exit 1
fi

create_dashboard_user
setup_state_dir
setup_project_permissions

CURRENT_DIR="$(pwd)"

echo "[*] Installing systemd service..."
sed \
	-e "s|WorkingDirectory=/root/TrinityProxy|WorkingDirectory=$CURRENT_DIR|g" \
	-e "s|ExecStart=/root/TrinityProxy/build/trinityproxy-dashboard|ExecStart=$CURRENT_DIR/build/trinityproxy-dashboard|g" \
	-e "s|ReadOnlyPaths=/root/TrinityProxy/build|ReadOnlyPaths=$CURRENT_DIR/build|g" \
	scripts/trinityproxy-dashboard.service > /etc/systemd/system/trinityproxy-dashboard.service

chmod 644 /etc/systemd/system/trinityproxy-dashboard.service

echo "[*] Enabling TrinityProxy Dashboard service..."
systemctl daemon-reload
systemctl enable trinityproxy-dashboard
if systemctl is-active trinityproxy-dashboard >/dev/null 2>&1; then
	systemctl restart trinityproxy-dashboard
else
	systemctl start trinityproxy-dashboard
fi

echo "[+] TrinityProxy Dashboard installed as systemd service!"
echo ""
echo "Runtime user: $DASHBOARD_USER"
echo "Dashboard API + UI: http://<your-vps-ip>:8081"
echo "Put Caddy/nginx in front for HTTPS on your domain (see README)."
echo ""
echo "  sudo systemctl status trinityproxy-dashboard"
echo "  sudo journalctl -u trinityproxy-dashboard -f"
