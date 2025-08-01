#!/bin/bash

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
    elif type lsb_release >/dev/null 2>&1; then
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
    
    if command -v apt-get >/dev/null 2>&1; then
        yellow "[*] Installing $package with apt-get..."
        apt-get update -y && apt-get install -y $package
    elif command -v yum >/dev/null 2>&1; then
        yellow "[*] Installing $package with yum..."
        yum install -y $package
    elif command -v dnf >/dev/null 2>&1; then
        yellow "[*] Installing $package with dnf..."
        dnf install -y $package
    elif command -v pacman >/dev/null 2>&1; then
        yellow "[*] Installing $package with pacman..."
        pacman -S --noconfirm $package
    elif command -v apk >/dev/null 2>&1; then
        yellow "[*] Installing $package with apk..."
        apk add --no-cache $package
    else
        red "[-] No supported package manager found!"
        red "[-] Please install $package manually"
        return 1
    fi
}

# Update PATH and make it persistent
update_go_path() {
    export PATH="/usr/local/go/bin:$PATH"
    
    # Update for current session
    if [ -f ~/.bashrc ]; then
        if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
            echo 'export PATH="/usr/local/go/bin:$PATH"' >> ~/.bashrc
        fi
    fi
    
    if [ -f ~/.profile ]; then
        if ! grep -q "/usr/local/go/bin" ~/.profile; then
            echo 'export PATH="/usr/local/go/bin:$PATH"' >> ~/.profile
        fi
    fi
    
    # For systems that use /etc/profile.d/
    if [ -d /etc/profile.d ]; then
        echo 'export PATH="/usr/local/go/bin:$PATH"' > /etc/profile.d/go.sh
        chmod +x /etc/profile.d/go.sh
    fi
}

detect_os

# Check Go with updated PATH first
export PATH="/usr/local/go/bin:$PATH"
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

# Check Dante SOCKS5 server
if command -v sockd >/dev/null 2>&1; then
  green "[✔] Dante (sockd) is already installed"
else
  yellow "[!] Dante not found. Installing..."
  
  # Dante package names vary by distribution
  if command -v apt-get >/dev/null 2>&1; then
    install_package "dante-server"
  elif command -v yum >/dev/null 2>&1; then
    install_package "dante"
  elif command -v dnf >/dev/null 2>&1; then
    install_package "dante"
  elif command -v pacman >/dev/null 2>&1; then
    install_package "dante"
  elif command -v apk >/dev/null 2>&1; then
    install_package "dante-server"
  else
    red "[-] Unable to install Dante automatically"
    red "[-] Please install Dante SOCKS5 server manually"
    exit 1
  fi
  
  green "[✔] Dante installed"
fi

# Check and install essential tools
essential_tools=("curl" "wget" "git")

# Add build tools based on package manager
if command -v apt-get >/dev/null 2>&1; then
    essential_tools+=("build-essential")
elif command -v yum >/dev/null 2>&1; then
    essential_tools+=("gcc" "gcc-c++" "make")
elif command -v dnf >/dev/null 2>&1; then
    essential_tools+=("gcc" "gcc-c++" "make")
elif command -v pacman >/dev/null 2>&1; then
    essential_tools+=("base-devel")
elif command -v apk >/dev/null 2>&1; then
    essential_tools+=("build-base")
fi

for tool in "${essential_tools[@]}"; do
  if command -v "$tool" >/dev/null 2>&1 || dpkg -s "$tool" >/dev/null 2>&1; then
    green "[✔] $tool is installed"
  else
    yellow "[!] $tool not found. Installing..."
    install_package "$tool"
    green "[✔] $tool installed"
  fi
done

echo ""
green "[+] TrinityProxy base setup complete. All dependencies are ready."
green "[*] Go binary location: $(which go 2>/dev/null || echo '/usr/local/go/bin/go')"
green "[*] Dante binary location: $(which sockd 2>/dev/null || echo 'sockd should be in PATH')"
