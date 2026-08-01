#!/bin/bash
#
# Install TrinityProxy Controller as a systemd service (requires root).
# Runtime runs as dedicated user "trinityproxy"; this script is one-time sudo setup.

set -e

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
    if ! id "$CONTROLLER_USER" &>/dev/null; then
        echo "[*] Creating system user: $CONTROLLER_USER"
        useradd --system --no-create-home --shell /usr/sbin/nologin \
            --home-dir "$STATE_DIR" "$CONTROLLER_USER"
    else
        echo "[*] System user $CONTROLLER_USER already exists"
    fi
}

setup_state_dir() {
    echo "[*] Preparing controller state directory: $STATE_DIR"
    install -d -o "$CONTROLLER_USER" -g "$CONTROLLER_GROUP" -m 750 "$STATE_DIR"
}

setup_project_permissions() {
    local project_root
    project_root="$(pwd)"
    echo "[*] Setting project permissions for $CONTROLLER_USER (read/execute only)"
    # Controller needs read+execute on repo root and build artifacts; no write to source tree.
    chmod o+rX "$project_root" "$project_root/build" 2>/dev/null || true
    chmod o+r "$project_root/build/trinityproxy-api" 2>/dev/null || true
}

# Build binaries via Makefile (canonical paths: build/trinityproxy-api, etc.)
echo "[*] Building TrinityProxy..."
export PATH="/usr/local/go/bin:$PATH"
make build

create_controller_user
setup_state_dir
setup_project_permissions

# Copy the service file
echo "[*] Installing systemd service..."
CURRENT_DIR="$(pwd)"
sed \
    -e "s|WorkingDirectory=/root/TrinityProxy|WorkingDirectory=$CURRENT_DIR|g" \
    -e "s|ExecStart=/root/TrinityProxy/build/trinityproxy-api|ExecStart=$CURRENT_DIR/build/trinityproxy-api|g" \
    -e "s|ReadOnlyPaths=/root/TrinityProxy/build|ReadOnlyPaths=$CURRENT_DIR/build|g" \
    scripts/trinityproxy-controller.service > /etc/systemd/system/trinityproxy-controller.service

chmod 644 /etc/systemd/system/trinityproxy-controller.service

# Reload systemd and enable the service
echo "[*] Enabling TrinityProxy Controller service..."
systemctl daemon-reload
systemctl enable trinityproxy-controller
systemctl start trinityproxy-controller

echo "[+] TrinityProxy Controller installed as systemd service!"
echo ""
echo "Runtime user: $CONTROLLER_USER (non-root)"
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
echo "Set TRINITY_API_KEY and TRINITY_AGENT_KEY before exposing the API to a network."
