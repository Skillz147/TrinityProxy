#!/bin/bash
#
# Install TrinityProxy controller + dashboard as systemd services (VPS production).
# Requires root. Run from the TrinityProxy repo root.
#
# Usage: sudo ./scripts/install-production.sh
#    or: make install-production

set -euo pipefail

echo "============================================"
echo "  TrinityProxy production install"
echo "============================================"
echo ""

if [[ $EUID -ne 0 ]]; then
	echo "[-] Error: run with sudo"
	exit 1
fi

if [[ ! -f "cmd/api/enhanced_main.go" ]] || [[ ! -f "cmd/dashboard/main.go" ]]; then
	echo "[-] Error: Run this from the TrinityProxy root directory"
	exit 1
fi

export PATH="/usr/local/go/bin:$PATH"

echo "[1/4] Building binaries..."
make build

echo ""
echo "[2/4] Syncing agent key (if dashboard DB exists)..."
if [[ -f ./dashboard.db ]] && command -v sqlite3 >/dev/null 2>&1; then
	chmod +x scripts/sync-agent-key.sh
	./scripts/sync-agent-key.sh || true
	if [[ -f .env.controller ]]; then
		install -d -m 750 /etc/trinityproxy
		grep -E '^export TRINITY_AGENT_KEY=' .env.controller | sed 's/^export //' > /etc/trinityproxy/controller.env
		chmod 640 /etc/trinityproxy/controller.env
		echo "[+] Wrote /etc/trinityproxy/controller.env"
	fi
else
	echo "[!] No dashboard.db yet — set TRINITY_AGENT_KEY in /etc/trinityproxy/controller.env after first dashboard login"
	install -d -m 750 /etc/trinityproxy
	touch /etc/trinityproxy/controller.env
	chmod 640 /etc/trinityproxy/controller.env
fi

echo ""
echo "[3/4] Installing controller systemd service..."
chmod +x scripts/install-service.sh
bash scripts/install-service.sh

echo ""
echo "[4/4] Installing dashboard systemd service..."
chmod +x scripts/install-dashboard-service.sh
bash scripts/install-dashboard-service.sh

echo ""
echo "============================================"
echo "  Production install complete"
echo "============================================"
echo ""
echo "Services (auto-start on reboot):"
echo "  trinityproxy-controller  → :3100"
echo "  trinityproxy-dashboard   → :8081 (API + built UI)"
echo ""
echo "Next steps:"
echo "  1. Open http://<vps-ip>:8081 and log in"
echo "  2. Settings → set domain → Save"
echo "  3. Settings → Cloudflare SSL (or run scripts/setup-ssl-caddy-cloudflare.sh)"
echo "  4. Deploy Agent → copy install command to agent VPS"
echo ""
echo "Service commands:"
echo "  sudo systemctl status trinityproxy-controller trinityproxy-dashboard"
echo "  sudo journalctl -u trinityproxy-controller -f"
echo "  sudo journalctl -u trinityproxy-dashboard -f"
