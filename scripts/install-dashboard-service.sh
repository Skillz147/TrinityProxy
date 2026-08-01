#!/bin/bash
#
# Install TrinityProxy Dashboard as a systemd service (requires root).
# Builds the React UI and serves it from the dashboard binary on :8081.

set -euo pipefail

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
	if ! id "$DASHBOARD_USER" &>/dev/null; then
		echo "[*] Creating system user: $DASHBOARD_USER"
		useradd --system --no-create-home --shell /usr/sbin/nologin \
			--home-dir "$STATE_DIR" "$DASHBOARD_USER"
	else
		echo "[*] System user $DASHBOARD_USER already exists"
	fi
}

setup_state_dir() {
	echo "[*] Preparing state directory: $STATE_DIR"
	install -d -o "$DASHBOARD_USER" -g "$DASHBOARD_GROUP" -m 750 "$STATE_DIR"
}

setup_project_permissions() {
	local project_root
	project_root="$(pwd)"
	echo "[*] Setting project permissions for $DASHBOARD_USER"
	chmod o+rX "$project_root" "$project_root/build" "$project_root/web/dashboard/dist" 2>/dev/null || true
	chmod o+r "$project_root/build/trinityproxy-dashboard" 2>/dev/null || true
	chmod -R o+rX "$project_root/web/dashboard/dist" 2>/dev/null || true
}

echo "[*] Building TrinityProxy dashboard..."
export PATH="/usr/local/go/bin:$PATH"
make build-dashboard

if ! command -v npm >/dev/null 2>&1; then
	echo "[-] Node.js/npm required to build dashboard UI. Install Node 18+ and re-run."
	exit 1
fi

echo "[*] Building dashboard UI (web/dashboard/dist)..."
if [[ ! -d web/dashboard/node_modules ]]; then
	(cd web/dashboard && npm install)
fi
(cd web/dashboard && npm run build)

create_dashboard_user
setup_state_dir
setup_project_permissions

CURRENT_DIR="$(pwd)"
echo "[*] Installing systemd service..."
sed \
	-e "s|WorkingDirectory=/root/TrinityProxy|WorkingDirectory=$CURRENT_DIR|g" \
	-e "s|ExecStart=/root/TrinityProxy/build/trinityproxy-dashboard|ExecStart=$CURRENT_DIR/build/trinityproxy-dashboard|g" \
	-e "s|DASHBOARD_STATIC_DIR=/root/TrinityProxy/web/dashboard/dist|DASHBOARD_STATIC_DIR=$CURRENT_DIR/web/dashboard/dist|g" \
	-e "s|ReadOnlyPaths=/root/TrinityProxy/build /root/TrinityProxy/web/dashboard/dist|ReadOnlyPaths=$CURRENT_DIR/build $CURRENT_DIR/web/dashboard/dist|g" \
	scripts/trinityproxy-dashboard.service > /etc/systemd/system/trinityproxy-dashboard.service

chmod 644 /etc/systemd/system/trinityproxy-dashboard.service

echo "[*] Enabling TrinityProxy Dashboard service..."
systemctl daemon-reload
systemctl enable trinityproxy-dashboard
systemctl start trinityproxy-dashboard

echo "[+] TrinityProxy Dashboard installed as systemd service!"
echo ""
echo "Runtime user: $DASHBOARD_USER"
echo "Dashboard API + UI: http://<your-vps-ip>:8081"
echo "Put Caddy/nginx in front for HTTPS on your domain (see README)."
echo ""
echo "  sudo systemctl status trinityproxy-dashboard"
echo "  sudo journalctl -u trinityproxy-dashboard -f"
