#!/bin/bash
#
# Install TrinityProxy Agent as a systemd service (requires root).
# Initial SOCKS/Dante setup runs once via build/installer (root); heartbeat runtime
# runs as dedicated user "trinityproxy-agent".
#
# Non-interactive env (optional, injected into systemd unit):
#   CONTROLLER_URL, TRINITY_AGENT_KEY, TRINITY_DEVICE_CLASS
#
# Usage:
#   sudo CONTROLLER_URL=http://controller:3100 TRINITY_AGENT_KEY=... ./scripts/install-agent-service.sh

set -euo pipefail

AGENT_USER="trinityproxy-agent"
AGENT_GROUP="trinityproxy-agent"
STATE_DIR="/var/lib/trinityproxy-agent"
CREDENTIAL_PATHS=(
    /etc/trinityproxy-username
    /etc/trinityproxy-password
    /etc/trinityproxy-port
)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SERVICE_NAME="trinityproxy-agent"
SERVICE_FILE="$SCRIPT_DIR/${SERVICE_NAME}.service"
SYSTEMD_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

DISTRO_FAMILY="unknown"
PKG_HINT="install dante-server (provides sockd) using your distro package manager"

fail() {
    echo "[!] $*" >&2
    exit 1
}

hint() {
    echo "[*] $*"
}

detect_platform() {
    local uname_s
    uname_s="$(uname -s)"

    if [[ "$uname_s" != "Linux" ]]; then
        fail "This script requires Linux with systemd.
On macOS use: scripts/install-agent-macos.sh
  CONTROLLER_URL=http://127.0.0.1:3100 TRINITY_AGENT_KEY=... ./scripts/install-agent-macos.sh"
    fi

    if ! command -v systemctl >/dev/null 2>&1; then
        fail "systemd not found. TrinityProxy agent service requires systemd (systemctl).
Alpine/OpenRC and other init systems are not supported by this script.
On macOS use: scripts/install-agent-macos.sh"
    fi

    if [[ ! -d /run/systemd/system ]] && ! systemctl --version >/dev/null 2>&1; then
        fail "systemd does not appear to be running as PID 1.
Start systemd or use a supported Linux VPS image, then re-run this script."
    fi
}

detect_distro() {
    if [[ -f /etc/os-release ]]; then
        # shellcheck source=/dev/null
        . /etc/os-release
        local id_like="${ID_LIKE:-}"
        local id="${ID:-}"

        case "$id" in
            debian|ubuntu|linuxmint|pop|elementary|raspbian)
                DISTRO_FAMILY="debian"
                PKG_HINT="sudo apt-get update && sudo apt-get install -y dante-server"
                ;;
            alpine)
                DISTRO_FAMILY="alpine"
                PKG_HINT="sudo apk add --no-cache dante-server"
                ;;
            fedora|centos|rhel|rocky|almalinux|ol|amzn)
                DISTRO_FAMILY="rhel"
                if command -v dnf >/dev/null 2>&1; then
                    PKG_HINT="sudo dnf install -y dante-server"
                else
                    PKG_HINT="sudo yum install -y dante-server"
                fi
                ;;
            arch|manjaro)
                DISTRO_FAMILY="arch"
                PKG_HINT="sudo pacman -S --noconfirm dante"
                ;;
            opensuse*|sles)
                DISTRO_FAMILY="suse"
                PKG_HINT="sudo zypper install -y dante-server"
                ;;
            *)
                if [[ "$id_like" == *debian* ]] || [[ "$id_like" == *ubuntu* ]]; then
                    DISTRO_FAMILY="debian"
                    PKG_HINT="sudo apt-get update && sudo apt-get install -y dante-server"
                elif [[ "$id_like" == *rhel* ]] || [[ "$id_like" == *fedora* ]]; then
                    DISTRO_FAMILY="rhel"
                    if command -v dnf >/dev/null 2>&1; then
                        PKG_HINT="sudo dnf install -y dante-server"
                    else
                        PKG_HINT="sudo yum install -y dante-server"
                    fi
                elif [[ "$id_like" == *alpine* ]]; then
                    DISTRO_FAMILY="alpine"
                    PKG_HINT="sudo apk add --no-cache dante-server"
                fi
                ;;
        esac

        echo "[*] Detected: ${PRETTY_NAME:-Linux} (family: ${DISTRO_FAMILY})"
    else
        echo "[!] Could not read /etc/os-release — assuming generic Linux"
        if command -v apt-get >/dev/null 2>&1; then
            DISTRO_FAMILY="debian"
            PKG_HINT="sudo apt-get update && sudo apt-get install -y dante-server"
        elif command -v apk >/dev/null 2>&1; then
            DISTRO_FAMILY="alpine"
            PKG_HINT="sudo apk add --no-cache dante-server"
        elif command -v dnf >/dev/null 2>&1; then
            DISTRO_FAMILY="rhel"
            PKG_HINT="sudo dnf install -y dante-server"
        elif command -v yum >/dev/null 2>&1; then
            DISTRO_FAMILY="rhel"
            PKG_HINT="sudo yum install -y dante-server"
        fi
    fi
}

check_dependencies() {
    local missing=0

    if ! command -v useradd >/dev/null 2>&1; then
        echo "[!] useradd not found — required to create the $AGENT_USER system user"
        case "$DISTRO_FAMILY" in
            alpine)
                hint "Install shadow: sudo apk add --no-cache shadow"
                ;;
            debian)
                hint "Install passwd: sudo apt-get install -y passwd"
                ;;
            *)
                hint "Install your distro's user-management package (often shadow-utils or passwd)"
                ;;
        esac
        missing=1
    fi

    if ! command -v sockd >/dev/null 2>&1 && ! command -v danted >/dev/null 2>&1; then
        echo "[!] Dante SOCKS server not found (sockd/danted not in PATH)"
        hint "Install with: $PKG_HINT"
        hint "Or run 'make install-dante' from the project root"
        missing=1
    fi

    if [[ $missing -ne 0 ]]; then
        fail "Missing dependencies — install the packages above and re-run this script."
    fi
}

create_agent_user() {
    if ! id "$AGENT_USER" &>/dev/null; then
        echo "[*] Creating system user: $AGENT_USER"
        useradd --system --no-create-home --shell /usr/sbin/nologin \
            --home-dir "$STATE_DIR" "$AGENT_USER"
    else
        echo "[*] System user $AGENT_USER already exists"
    fi
    install -d -o "$AGENT_USER" -g "$AGENT_GROUP" -m 750 "$STATE_DIR"
}

setup_project_permissions() {
    echo "[*] Setting project permissions for $AGENT_USER (read/execute only)"
    chmod o+rX "$PROJECT_ROOT" "$PROJECT_ROOT/build" 2>/dev/null || true
    chmod o+r "$PROJECT_ROOT/build/trinityproxy" "$PROJECT_ROOT/build/installer" 2>/dev/null || true
}

set_credential_permissions() {
    echo "[*] Setting credential file ownership for $AGENT_USER"
    for path in "${CREDENTIAL_PATHS[@]}"; do
        if [[ -f "$path" ]]; then
            chown root:"$AGENT_GROUP" "$path"
            chmod 640 "$path"
        fi
    done
}

run_installer_once() {
    echo "[*] Running one-time SOCKS installer as root (writes /etc/trinityproxy-*)..."
    "$PROJECT_ROOT/build/installer"
    set_credential_permissions
}

install_service_file() {
    local tmp
    tmp="$(mktemp)"
    sed -e "s|/root/TrinityProxy|$PROJECT_ROOT|g" "$SERVICE_FILE" > "$tmp"

    awk -v url="${CONTROLLER_URL:-}" -v key="${TRINITY_AGENT_KEY:-}" -v devclass="${TRINITY_DEVICE_CLASS:-}" '
        /Environment=TRINITY_NONINTERACTIVE=1/ {
            print
            if (url != "") print "Environment=CONTROLLER_URL=" url
            if (key != "") print "Environment=TRINITY_AGENT_KEY=" key
            if (devclass != "") print "Environment=TRINITY_DEVICE_CLASS=" devclass
            next
        }
        { print }
    ' "$tmp" > "${tmp}.new"
    mv "${tmp}.new" "$tmp"

    install -m 644 "$tmp" "$SYSTEMD_PATH"
    rm -f "$tmp"
}

echo "[*] Installing TrinityProxy Agent as systemd service..."

detect_platform

if [[ $EUID -ne 0 ]]; then
    fail "This script must be run as root (use sudo)"
fi

detect_distro
check_dependencies

if [[ ! -f "$SERVICE_FILE" ]]; then
    fail "Service file not found: $SERVICE_FILE"
fi

if [[ ! -f "$PROJECT_ROOT/build/trinityproxy" ]]; then
    fail "TrinityProxy binary not found at $PROJECT_ROOT/build/trinityproxy
Run 'make build' on this host, or copy a cross-compiled binary:
  make build-linux-amd64   # → build/trinityproxy-linux-amd64
  make build-linux-arm64   # → build/trinityproxy-linux-arm64
Then install as build/trinityproxy before running this script."
fi

if [[ ! -f "$PROJECT_ROOT/build/installer" ]]; then
    fail "Installer binary not found. Run 'make build' first."
fi

if [[ -n "${CONTROLLER_URL:-}" ]]; then
    echo "[*] Controller URL: $CONTROLLER_URL"
else
    echo "[!] CONTROLLER_URL not set — configure in $SYSTEMD_PATH or export before install"
fi

if [[ -z "${TRINITY_AGENT_KEY:-}" ]]; then
    echo "[!] TRINITY_AGENT_KEY unset — heartbeats will be unauthenticated (dev mode)"
fi

if [[ -n "${TRINITY_DEVICE_CLASS:-}" ]]; then
    echo "[*] Device class: $TRINITY_DEVICE_CLASS"
fi

# Stop existing service if running
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
    echo "[*] Stopping existing $SERVICE_NAME service..."
    systemctl stop "$SERVICE_NAME"
fi

create_agent_user
setup_project_permissions
run_installer_once

echo "[*] Installing service file..."
install_service_file

echo "[*] Reloading systemd daemon..."
systemctl daemon-reload

echo "[*] Enabling $SERVICE_NAME service..."
systemctl enable "$SERVICE_NAME"

echo "[+] TrinityProxy Agent service installed successfully!"
echo ""
echo "Runtime user: $AGENT_USER (non-root heartbeat)"
echo "Distro family: $DISTRO_FAMILY"
echo "Install step: build/installer ran once as root for /etc/ and Dante setup"
echo "Credentials:  root:${AGENT_GROUP} mode 640 on /etc/trinityproxy-*"
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
