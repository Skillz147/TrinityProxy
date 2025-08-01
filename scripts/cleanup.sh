#!/bin/bash

# TrinityProxy Cleanup Script
# Removes old installation for clean reinstall

echo "[+] TrinityProxy Cleanup Script"
echo "==============================="
echo "[!] This will remove ALL TrinityProxy files and services"
echo "[!] Make sure to backup any important data first"
echo ""

# Ask for confirmation
read -p "[?] Are you sure you want to remove TrinityProxy? (y/N): " confirm
if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
    echo "[*] Cleanup cancelled"
    exit 0
fi

echo ""
echo "[*] Starting TrinityProxy cleanup..."

# Colors for output
green() { echo -e "\e[32m$1\e[0m"; }
yellow() { echo -e "\e[33m$1\e[0m"; }
red() { echo -e "\e[31m$1\e[0m"; }

# Stop all TrinityProxy services
echo "[*] Stopping TrinityProxy services..."
systemctl stop trinityproxy 2>/dev/null || true
systemctl disable trinityproxy 2>/dev/null || true
green "[✔] Services stopped"

# Remove systemd service file
echo "[*] Removing systemd service..."
rm -f /etc/systemd/system/trinityproxy.service
systemctl daemon-reload
green "[✔] Systemd service removed"

# Remove configuration files
echo "[*] Removing configuration files..."
rm -f /etc/danted.conf
rm -f /etc/trinityproxy-username
rm -f /etc/trinityproxy-password
rm -f /etc/trinityproxy-port
green "[✔] Configuration files removed"

# Remove NGINX configuration (if exists)
echo "[*] Removing NGINX configuration..."
rm -f /etc/nginx/sites-available/trinityproxy-api
rm -f /etc/nginx/sites-enabled/trinityproxy-api
if command -v nginx >/dev/null 2>&1; then
    nginx -t && systemctl reload nginx 2>/dev/null || true
fi
green "[✔] NGINX configuration removed"

# Remove log files
echo "[*] Removing log files..."
rm -f /var/log/danted.log
rm -f /var/log/trinityproxy-*.log
green "[✔] Log files removed"

# Remove TrinityProxy directory
echo "[*] Removing TrinityProxy directory..."
if [ -d "/root/TrinityProxy" ]; then
    rm -rf /root/TrinityProxy
    green "[✔] /root/TrinityProxy removed"
fi

if [ -d "~/TrinityProxy" ]; then
    rm -rf ~/TrinityProxy
    green "[✔] ~/TrinityProxy removed"
fi

# Remove any TrinityProxy processes
echo "[*] Killing any running TrinityProxy processes..."
pkill -f trinityproxy 2>/dev/null || true
pkill -f "go run.*TrinityProxy" 2>/dev/null || true
green "[✔] Processes terminated"

# Clean environment variables
echo "[*] Cleaning environment variables..."
unset TRINITY_ROLE
unset CONTROLLER_URL

# Remove from shell profiles
for profile in ~/.bashrc ~/.zshrc ~/.profile ~/.config/fish/config.fish; do
    if [ -f "$profile" ]; then
        sed -i '/TRINITY_ROLE/d' "$profile" 2>/dev/null || true
        sed -i '/TrinityProxy/d' "$profile" 2>/dev/null || true
    fi
done

# Remove from system profile
rm -f /etc/profile.d/trinityproxy.sh

green "[✔] Environment variables cleaned"

# Optional: Remove Dante server (ask user)
echo ""
read -p "[?] Remove Dante SOCKS5 server? (y/N): " remove_dante
if [[ "$remove_dante" =~ ^[Yy]$ ]]; then
    echo "[*] Removing Dante server..."
    if command -v apt-get >/dev/null 2>&1; then
        apt-get remove --purge -y dante-server 2>/dev/null || true
    elif command -v yum >/dev/null 2>&1; then
        yum remove -y dante 2>/dev/null || true
    elif command -v dnf >/dev/null 2>&1; then
        dnf remove -y dante 2>/dev/null || true
    elif command -v pacman >/dev/null 2>&1; then
        pacman -Rs --noconfirm dante 2>/dev/null || true
    fi
    green "[✔] Dante server removed"
else
    yellow "[!] Dante server kept (can be used by new installation)"
fi

# Optional: Remove Go installation (ask user)
echo ""
read -p "[?] Remove Go installation? (y/N): " remove_go
if [[ "$remove_go" =~ ^[Yy]$ ]]; then
    echo "[*] Removing Go installation..."
    rm -rf /usr/local/go
    sed -i '/\/usr\/local\/go\/bin/d' ~/.bashrc 2>/dev/null || true
    sed -i '/\/usr\/local\/go\/bin/d' ~/.profile 2>/dev/null || true
    rm -f /etc/profile.d/go.sh
    green "[✔] Go installation removed"
else
    yellow "[!] Go installation kept (will be reused by new installation)"
fi

echo ""
green "[+] TrinityProxy cleanup completed!"
echo ""
echo "You can now install the latest version:"
echo "  git clone https://github.com/Skillz147/TrinityProxy.git"
echo "  cd TrinityProxy"
echo "  make vps-setup"
echo ""
yellow "[!] Remember to restart your terminal or run 'source ~/.bashrc' to refresh environment"
