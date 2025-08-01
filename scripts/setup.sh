#!/bin/bash

echo "[+] Starting TrinityProxy setup..."

set -e  # Exit if any command fails

# Colors
green() { echo -e "\e[32m$1\e[0m"; }
yellow() { echo -e "\e[33m$1\e[0m"; }

# Check Go
if command -v go >/dev/null 2>&1; then
  green "[✔] Go is already installed: $(go version)"
else
  yellow "[!] Go not found. Installing..."
  GO_VERSION=1.22.3
  cd /tmp
  wget https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf go$GO_VERSION.linux-amd64.tar.gz
  echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
  source ~/.profile
  green "[✔] Go installed: $(go version)"
fi

# Check Dante
if command -v sockd >/dev/null 2>&1; then
  green "[✔] Dante (sockd) is already installed"
else
  yellow "[!] Dante not found. Installing..."
  apt update -y
  apt install -y dante-server
  green "[✔] Dante installed"
fi

# Check other tools
for tool in curl wget git build-essential; do
  if dpkg -s "$tool" >/dev/null 2>&1; then
    green "[✔] $tool is installed"
  else
    yellow "[!] $tool not found. Installing..."
    apt install -y "$tool"
    green "[✔] $tool installed"
  fi
done

echo ""
green "[+] TrinityProxy base setup complete. All dependencies are ready."
