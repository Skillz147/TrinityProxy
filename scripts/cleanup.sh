#!/bin/bash

# TrinityProxy Cleanup Script
# Removes old installation for clean reinstall

NONINTERACTIVE=0
if [[ "${TRINITY_NONINTERACTIVE:-}" == "1" ]]; then
    NONINTERACTIVE=1
fi

usage() {
    cat <<EOF
Usage: $0 [--yes]

Options:
  --yes, -y                 Skip confirmation prompts (for scripted cleanup)
  TRINITY_NONINTERACTIVE=1  Same as --yes
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --yes|-y)
            NONINTERACTIVE=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

echo "[+] TrinityProxy Cleanup Script"
echo "==============================="
echo "[!] This will remove ALL TrinityProxy files and services"
echo "[!] Make sure to backup any important data first"
echo ""

# Ask for confirmation
if [[ "$NONINTERACTIVE" != "1" ]]; then
    read -p "[?] Are you sure you want to remove TrinityProxy? (y/N): " confirm
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        echo "[*] Cleanup cancelled"
        exit 0
    fi
else
    echo "[*] Non-interactive mode: proceeding without confirmation"
fi

echo ""
echo "[*] Starting TrinityProxy cleanup..."

# Colors for output
green() { echo -e "\e[32m$1\e[0m"; }
yellow() { echo -e "\e[33m$1\e[0m"; }
red() { echo -e "\e[31m$1\e[0m"; }

# Stop all TrinityProxy services (current + legacy)
SERVICES=(
    trinityproxy-agent
    trinityproxy-controller
    trinityproxy
)

echo "[*] Stopping TrinityProxy services..."
for svc in "${SERVICES[@]}"; do
    systemctl stop "$svc" 2>/dev/null || true
    systemctl disable "$svc" 2>/dev/null || true
done
green "[✔] Services stopped"

# Remove systemd service files
UNIT_FILES=(
    /etc/systemd/system/trinityproxy-agent.service
    /etc/systemd/system/trinityproxy-controller.service
    /etc/systemd/system/trinityproxy.service
)

echo "[*] Removing systemd service files..."
for unit in "${UNIT_FILES[@]}"; do
    rm -f "$unit"
done
systemctl daemon-reload
green "[✔] Systemd service files removed"

# Remove configuration files
echo "[*] Removing configuration files..."
rm -f /etc/danted.conf
rm -f /etc/trinityproxy-username
rm -f /etc/trinityproxy-password
rm -f /etc/trinityproxy-port
green "[✔] Configuration files removed"

# Remove NGINX configuration (if exists)
echo "[*] Removing NGINX configuration..."
if [ -f "/etc/nginx/sites-available/trinityproxy-api" ]; then
    rm -f /etc/nginx/sites-available/trinityproxy-api
    green "[✔] NGINX site config removed"
else
    yellow "[!] NGINX site config not found (may not have been created)"
fi

if [ -L "/etc/nginx/sites-enabled/trinityproxy-api" ]; then
    rm -f /etc/nginx/sites-enabled/trinityproxy-api
    green "[✔] NGINX site symlink removed"
else
    yellow "[!] NGINX site symlink not found"
fi

if command -v nginx >/dev/null 2>&1; then
    if nginx -t 2>/dev/null; then
        systemctl reload nginx 2>/dev/null || true
        green "[✔] NGINX reloaded successfully"
    else
        yellow "[!] NGINX config test failed, but continuing cleanup"
    fi
else
    yellow "[!] NGINX not installed"
fi

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

# Optional: Remove Dante server (ask user unless non-interactive)
remove_dante=false
if [[ "$NONINTERACTIVE" == "1" ]]; then
    yellow "[!] Non-interactive mode: keeping Dante server"
else
    echo ""
    read -p "[?] Remove Dante SOCKS5 server? (y/N): " remove_dante_answer
    if [[ "$remove_dante_answer" =~ ^[Yy]$ ]]; then
        remove_dante=true
    fi
fi

if [[ "$remove_dante" == "true" ]]; then
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

# Optional: Remove Go installation (ask user unless non-interactive)
remove_go=false
if [[ "$NONINTERACTIVE" == "1" ]]; then
    yellow "[!] Non-interactive mode: keeping Go installation"
else
    echo ""
    read -p "[?] Remove Go installation? (y/N): " remove_go_answer
    if [[ "$remove_go_answer" =~ ^[Yy]$ ]]; then
        remove_go=true
    fi
fi

if [[ "$remove_go" == "true" ]]; then
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
