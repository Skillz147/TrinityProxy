#!/bin/bash

set -e

echo "[*] Installing TrinityProxy Controller as systemd service..."

# Ensure we're in the right directory
if [ ! -f "cmd/api/enhanced_main.go" ]; then
    echo "[-] Error: Run this from the TrinityProxy root directory"
    exit 1
fi

# Build the binary first
echo "[*] Building TrinityProxy..."
export PATH="/usr/local/go/bin:$PATH"
go build -o build/trinityproxy-api ./cmd/api/

# Copy the service file
echo "[*] Installing systemd service..."
cp scripts/trinityproxy-controller.service /etc/systemd/system/

# Update the service file with the correct path
CURRENT_DIR=$(pwd)
sed -i "s|WorkingDirectory=/root/TrinityProxy|WorkingDirectory=$CURRENT_DIR|g" /etc/systemd/system/trinityproxy-controller.service
sed -i "s|ExecStart=/usr/local/go/bin/go run ./cmd/api/enhanced_main.go|ExecStart=$CURRENT_DIR/build/trinityproxy-api|g" /etc/systemd/system/trinityproxy-controller.service

# Reload systemd and enable the service
echo "[*] Enabling TrinityProxy Controller service..."
systemctl daemon-reload
systemctl enable trinityproxy-controller
systemctl start trinityproxy-controller

echo "[+] TrinityProxy Controller installed as systemd service!"
echo ""
echo "🚀 Service Management Commands:"
echo "  sudo systemctl status trinityproxy-controller   - Check status"
echo "  sudo systemctl start trinityproxy-controller    - Start service"
echo "  sudo systemctl stop trinityproxy-controller     - Stop service"
echo "  sudo systemctl restart trinityproxy-controller  - Restart service"
echo "  sudo journalctl -u trinityproxy-controller -f   - View logs"
echo ""
echo "✅ TrinityProxy API is now running in the background!"
echo "🌐 Access your API at: https://api.sauronstore.com"
