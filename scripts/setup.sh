#!/bin/bash

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/production-common.sh
source "$ROOT/scripts/lib/production-common.sh"

echo "[+] Starting TrinityProxy setup..."

set -e  # Exit if any command fails

# Colors
green() { echo -e "\e[32m$1\e[0m"; }
yellow() { echo -e "\e[33m$1\e[0m"; }
red() { echo -e "\e[31m$1\e[0m"; }

# Detect OS and package manager
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$NAME
        VER=$VERSION_ID
    elif production_resolve_cmd lsb_release >/dev/null 2>&1; then
        OS=$(lsb_release -si)
        VER=$(lsb_release -sr)
    elif [ -f /etc/redhat-release ]; then
        OS="CentOS"
        VER=$(cat /etc/redhat-release | sed 's/.*release //' | sed 's/ .*//')
    else
        OS=$(uname -s)
        VER=$(uname -r)
    fi
    
    echo "[*] Detected OS: $OS $VER"
}

# Install package based on OS
install_package() {
    local package=$1
    local apt_get yum dnf pacman apk

    apt_get="$(production_resolve_cmd apt-get 2>/dev/null || true)"
    yum="$(production_resolve_cmd yum 2>/dev/null || true)"
    dnf="$(production_resolve_cmd dnf 2>/dev/null || true)"
    pacman="$(production_resolve_cmd pacman 2>/dev/null || true)"
    apk="$(production_resolve_cmd apk 2>/dev/null || true)"

    if [[ -n "$apt_get" ]]; then
        yellow "[*] Installing $package with apt-get..."
        "$apt_get" update -y && "$apt_get" install -y "$package" || return 1
    elif [[ -n "$yum" ]]; then
        yellow "[*] Installing $package with yum..."
        "$yum" install -y "$package" || return 1
    elif [[ -n "$dnf" ]]; then
        yellow "[*] Installing $package with dnf..."
        "$dnf" install -y "$package" || return 1
    elif [[ -n "$pacman" ]]; then
        yellow "[*] Installing $package with pacman..."
        "$pacman" -S --noconfirm "$package" || return 1
    elif [[ -n "$apk" ]]; then
        yellow "[*] Installing $package with apk..."
        "$apk" add --no-cache "$package" || return 1
    else
        red "[-] No supported package manager found!"
        red "[-] Please install $package manually"
        return 1
    fi
}

# Update PATH and make it persistent
update_go_path() {
    export PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
    
    # Update for current session
    if [ -f ~/.bashrc ]; then
        if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
            echo 'export PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"' >> ~/.bashrc
        fi
    fi
    
    if [ -f ~/.profile ]; then
        if ! grep -q "/usr/local/go/bin" ~/.profile; then
            echo 'export PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"' >> ~/.profile
        fi
    fi
    
    # For systems that use /etc/profile.d/
    if [ -d /etc/profile.d ]; then
        echo 'export PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"' > /etc/profile.d/go.sh
        chmod +x /etc/profile.d/go.sh
    fi
}

detect_os

# Check Go with updated PATH first
export PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"
if command -v go >/dev/null 2>&1; then
  green "[✔] Go is already installed: $(go version)"
else
  yellow "[!] Go not found. Installing..."
  GO_VERSION=1.24.3
  
  # Clean up any partial downloads
  rm -f /tmp/go*.tar.gz*
  
  cd /tmp
  wget -O go$GO_VERSION.linux-amd64.tar.gz https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz
  
  # Remove existing Go installation
  rm -rf /usr/local/go
  
  # Extract new Go
  tar -C /usr/local -xzf go$GO_VERSION.linux-amd64.tar.gz
  
  # Update PATH
  update_go_path
  
  # Verify installation
  if /usr/local/go/bin/go version >/dev/null 2>&1; then
    green "[✔] Go installed: $(/usr/local/go/bin/go version)"
  else
    red "[-] Go installation failed!"
    exit 1
  fi
fi

# Controller-only bootstrap sets TRINITY_SKIP_DANTE=1 (no sockd on controller VPS).
install_dante() {
  local pkg="$1"
  yellow "[*] Trying Dante package: $pkg"
  if install_package "$pkg"; then
    return 0
  fi
  return 1
}

if [[ "${TRINITY_SKIP_DANTE:-}" == "1" ]]; then
  yellow "[*] Skipping Dante (TRINITY_SKIP_DANTE=1 — controller-only host)"
elif command -v sockd >/dev/null 2>&1; then
  green "[✔] Dante (sockd) is already installed"
else
  yellow "[!] Dante not found. Installing (optional for controller-only VPS)..."

  DANTE_OK=0
  if production_resolve_cmd apt-get >/dev/null 2>&1; then
    # Debian 13+ may not ship dante-server — try alternatives, then continue without it
    for pkg in dante-server dante dante-server-standalone; do
      if install_dante "$pkg"; then DANTE_OK=1; break; fi
    done
  elif production_resolve_cmd yum >/dev/null 2>&1; then
    for pkg in dante dante-server; do
      if install_dante "$pkg"; then DANTE_OK=1; break; fi
    done
  elif production_resolve_cmd dnf >/dev/null 2>&1; then
    for pkg in dante dante-server; do
      if install_dante "$pkg"; then DANTE_OK=1; break; fi
    done
  elif production_resolve_cmd pacman >/dev/null 2>&1; then
    if install_dante "dante"; then DANTE_OK=1; fi
  elif production_resolve_cmd apk >/dev/null 2>&1; then
    for pkg in dante-server dante; do
      if install_dante "$pkg"; then DANTE_OK=1; break; fi
    done
  fi

  if command -v sockd >/dev/null 2>&1; then
    green "[✔] Dante installed"
  elif [[ "$DANTE_OK" -eq 1 ]]; then
    yellow "[!] Dante package installed but sockd not in PATH — verify manually"
  else
    yellow "[!] Dante not installed (no matching package on this OS)."
    yellow "[*] Controller-only VPS: safe to skip — install Dante on agent nodes only."
    yellow "[*] Agent VPS: install Dante manually or use an older Debian/Ubuntu release."
  fi
fi

# Check and install essential tools
essential_tools=("curl" "wget" "git" "make")

# Add build tools based on package manager
if production_resolve_cmd apt-get >/dev/null 2>&1; then
    essential_tools+=("build-essential")
elif production_resolve_cmd yum >/dev/null 2>&1; then
    essential_tools+=("gcc" "gcc-c++" "make")
elif production_resolve_cmd dnf >/dev/null 2>&1; then
    essential_tools+=("gcc" "gcc-c++" "make")
elif production_resolve_cmd pacman >/dev/null 2>&1; then
    essential_tools+=("base-devel")
elif production_resolve_cmd apk >/dev/null 2>&1; then
    essential_tools+=("build-base")
fi

for tool in "${essential_tools[@]}"; do
  if command -v "$tool" >/dev/null 2>&1 || { dpkg_bin="$(production_resolve_cmd dpkg 2>/dev/null || true)"; [[ -n "$dpkg_bin" ]] && "$dpkg_bin" -s "$tool" >/dev/null 2>&1; }; then
    green "[✔] $tool is installed"
  else
    yellow "[!] $tool not found. Installing..."
    install_package "$tool"
    green "[✔] $tool installed"
  fi
done

# User management (systemd service users on minimal images)
if production_have_user_mgmt; then
  green "[✔] adduser/useradd is available"
else
  yellow "[!] adduser/useradd not found. Installing..."
  if production_resolve_cmd apt-get >/dev/null 2>&1; then
    install_package adduser || install_package passwd || true
  elif production_resolve_cmd yum >/dev/null 2>&1 || production_resolve_cmd dnf >/dev/null 2>&1; then
    install_package shadow-utils || true
  elif production_resolve_cmd apk >/dev/null 2>&1; then
    install_package shadow || true
  fi
fi

# OpenSSL (secret generation)
if production_resolve_cmd openssl >/dev/null 2>&1; then
  green "[✔] openssl is installed"
else
  yellow "[!] openssl not found. Installing..."
  install_package openssl || true
fi

# SQLite CLI (agent key sync, dashboard DB inspection)
if production_resolve_cmd sqlite3 >/dev/null 2>&1; then
  green "[✔] sqlite3 is installed"
else
  yellow "[!] sqlite3 not found. Installing..."
  if production_resolve_cmd apt-get >/dev/null 2>&1; then
    install_package sqlite3
  elif production_resolve_cmd yum >/dev/null 2>&1 || production_resolve_cmd dnf >/dev/null 2>&1; then
    install_package sqlite
  elif production_resolve_cmd pacman >/dev/null 2>&1; then
    install_package sqlite
  elif production_resolve_cmd apk >/dev/null 2>&1; then
    install_package sqlite
  else
    yellow "[!] Install sqlite3 manually for production key sync"
  fi
fi

echo ""
green "[+] TrinityProxy base setup complete. All dependencies are ready."
green "[*] Go binary location: $(which go 2>/dev/null || echo '/usr/local/go/bin/go')"
green "[*] Dante binary location: $(which sockd 2>/dev/null || echo 'sockd should be in PATH')"
