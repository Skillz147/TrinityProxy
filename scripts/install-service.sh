#!/bin/bash
#
# Install TrinityProxy Controller as a systemd service (requires root).
# Runtime runs as dedicated user "trinityproxy"; this script is one-time sudo setup.

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
set -e

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

cd "$ROOT"

CONTROLLER_USER="trinityproxy"
CONTROLLER_GROUP="trinityproxy"
STATE_DIR="/var/lib/trinityproxy"

echo "[*] Installing TrinityProxy Controller as systemd service..."

if [[ $EUID -ne 0 ]]; then
    echo "[-] Error: run with sudo (installer needs root for /etc/ and systemd)"
    exit 1
fi

# Ensure we're in the right directory
if [ ! -f "cmd/api/enhanced_main.go" ]; then
    echo "[-] Error: Run this from the TrinityProxy root directory"
    exit 1
fi

create_controller_user() {
	local id_bin
	id_bin="$(production_resolve_cmd id)" || exit 1
	if "$id_bin" "$CONTROLLER_USER" &>/dev/null; then
		echo "[*] System user $CONTROLLER_USER already exists"
	fi
	production_ensure_trinityproxy_user
}

setup_state_dir() {
    echo "[*] Preparing controller state directory: $STATE_DIR"
    production_install -d -o "$CONTROLLER_USER" -g "$CONTROLLER_GROUP" -m 750 "$STATE_DIR"
}

setup_dev_project_permissions() {
	local project_root
	project_root="$(pwd)"
	echo "[*] Dev: granting $CONTROLLER_USER read/execute on $project_root/build"
	chmod o+rX "$project_root" "$project_root/build" 2>/dev/null || true
	chmod o+r "$project_root/build/trinityproxy-api" 2>/dev/null || true
}

# Build binaries via Makefile (canonical paths: build/trinityproxy-api, etc.)
if [[ "${SKIP_BUILD:-}" != "1" ]]; then
	echo "[*] Building TrinityProxy..."
	export PATH="/usr/local/go/bin:$PATH"
	make build
else
	echo "[*] Skipping build (SKIP_BUILD=1)"
	export PATH="/usr/local/go/bin:$PATH"
fi

create_controller_user
setup_state_dir
if production_is_dev_install; then
	setup_dev_project_permissions
else
	production_install_binaries "$ROOT" trinityproxy-api
fi

echo "[*] Installing systemd service..."
production_install_systemd_unit "$ROOT/scripts/trinityproxy-controller.service" \
	/etc/systemd/system/trinityproxy-controller.service "$ROOT"

# Reload systemd and enable the service
echo "[*] Enabling TrinityProxy Controller service..."
if [[ -f .env.controller ]] && [[ ! -f /etc/trinityproxy/controller.env ]]; then
	production_install -d -o root -g "$CONTROLLER_GROUP" -m 750 /etc/trinityproxy
	grep -E '^export TRINITY_' .env.controller | sed 's/^export //' > /etc/trinityproxy/controller.env
	chmod 640 /etc/trinityproxy/controller.env
	chown root:"$CONTROLLER_GROUP" /etc/trinityproxy/controller.env 2>/dev/null || true
	echo "[+] Wrote /etc/trinityproxy/controller.env from .env.controller"
elif [[ -f /etc/trinityproxy/controller.env ]]; then
	echo "[*] Using existing /etc/trinityproxy/controller.env"
fi
production_systemctl daemon-reload
production_systemctl enable trinityproxy-controller
if [[ "${SKIP_START:-}" != "1" ]]; then
	production_systemctl start trinityproxy-controller
else
	echo "[*] Skipping service start (SKIP_START=1)"
fi

echo "[+] TrinityProxy Controller installed as systemd service!"
echo ""
echo "Runtime user: $CONTROLLER_USER (non-root)"
if production_is_dev_install; then
	echo "Binary:         $ROOT/build/trinityproxy-api (TRINITY_DEV=1)"
else
	echo "Binary:         $OPT_BIN_DIR/trinityproxy-api"
fi
echo "Database path:  $STATE_DIR/trinityproxy.db (via DB_PATH in unit file)"
echo ""
echo "One-time install used sudo; day-to-day service runs unprivileged."
echo ""
echo "Service Management Commands:"
echo "  sudo systemctl status trinityproxy-controller   - Check status"
echo "  sudo systemctl start trinityproxy-controller    - Start service"
echo "  sudo systemctl stop trinityproxy-controller     - Stop service"
echo "  sudo systemctl restart trinityproxy-controller  - Restart service"
echo "  sudo journalctl -u trinityproxy-controller -f   - View logs"
echo ""
echo "Controller API:   $(production_http_url "${API_PORT:-3100}")"
echo ""
echo "Set TRINITY_API_KEY and TRINITY_AGENT_KEY before exposing the API to a network."
