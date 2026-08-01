#!/bin/bash
#
# Install TrinityProxy Dashboard as a systemd service (requires root).
# Serves API + embedded UI on :8081 (no Node.js required on VPS).

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

cd "$ROOT"

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
	local id_bin
	id_bin="$(production_resolve_cmd id)" || exit 1
	if "$id_bin" "$DASHBOARD_USER" &>/dev/null; then
		echo "[*] System user $DASHBOARD_USER already exists"
	fi
	production_ensure_trinityproxy_user
}

setup_state_dir() {
	echo "[*] Preparing state directory: $STATE_DIR"
	production_install -d -o "$DASHBOARD_USER" -g "$DASHBOARD_GROUP" -m 750 "$STATE_DIR"
}

setup_dev_project_permissions() {
	local project_root
	project_root="$(pwd)"
	echo "[*] Dev: granting $DASHBOARD_USER read/execute on $project_root/build"
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
if production_is_dev_install; then
	setup_dev_project_permissions
else
	production_install_binaries "$ROOT" trinityproxy-dashboard
fi

echo "[*] Installing systemd service..."
production_install_systemd_unit "$ROOT/scripts/trinityproxy-dashboard.service" \
	/etc/systemd/system/trinityproxy-dashboard.service "$ROOT"

echo "[*] Enabling TrinityProxy Dashboard service..."
production_systemctl daemon-reload
production_systemctl enable trinityproxy-dashboard
if [[ "${SKIP_START:-}" != "1" ]]; then
	if production_systemctl is-active trinityproxy-dashboard >/dev/null 2>&1; then
		production_systemctl restart trinityproxy-dashboard
	else
		production_systemctl start trinityproxy-dashboard
	fi
else
	echo "[*] Skipping service start (SKIP_START=1)"
fi

echo "[+] TrinityProxy Dashboard installed as systemd service!"
echo ""
echo "Runtime user: $DASHBOARD_USER"
echo "Dashboard API + UI: $(production_http_url "${DASHBOARD_PORT:-8081}")"
echo "Put Caddy/nginx in front for HTTPS on your domain (see README)."
echo ""
echo "  sudo systemctl status trinityproxy-dashboard"
echo "  sudo journalctl -u trinityproxy-dashboard -f"
