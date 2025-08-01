#!/bin/bash

# TrinityProxy Agent Service Installation Script
# Installs and configures the agent as a systemd background service

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SERVICE_NAME="trinityproxy-agent"
SERVICE_FILE="$SCRIPT_DIR/${SERVICE_NAME}.service"
SYSTEMD_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

echo "[*] Installing TrinityProxy Agent as systemd service..."

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo "[!] This script must be run as root (use sudo)"
   exit 1
fi

# Check if service file exists
if [[ ! -f "$SERVICE_FILE" ]]; then
    echo "[!] Service file not found: $SERVICE_FILE"
    exit 1
fi

# Check if binary exists
if [[ ! -f "$PROJECT_ROOT/build/trinityproxy" ]]; then
    echo "[!] TrinityProxy binary not found. Run 'make build' first."
    exit 1
fi

# Stop existing service if running
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
    echo "[*] Stopping existing $SERVICE_NAME service..."
    systemctl stop "$SERVICE_NAME"
fi

# Copy service file and update paths
echo "[*] Installing service file..."
# Create a temporary service file with correct paths
sed "s|/root/TrinityProxy|$PROJECT_ROOT|g" "$SERVICE_FILE" > "$SYSTEMD_PATH"

# Set correct permissions
chmod 644 "$SYSTEMD_PATH"

# Reload systemd and enable service

echo "[*] Reloading systemd daemon..."
systemctl daemon-reload

echo "[*] Enabling $SERVICE_NAME service..."
systemctl enable "$SERVICE_NAME"

echo "[+] TrinityProxy Agent service installed successfully!"
echo ""
echo "Service Management Commands:"
echo "  Start:   sudo systemctl start $SERVICE_NAME"
echo "  Stop:    sudo systemctl stop $SERVICE_NAME"
echo "  Status:  sudo systemctl status $SERVICE_NAME"
echo "  Logs:    sudo journalctl -u $SERVICE_NAME -f"
echo "  Restart: sudo systemctl restart $SERVICE_NAME"
echo ""
echo "The service will automatically start on boot."
echo "Run 'sudo systemctl start $SERVICE_NAME' to start it now."
