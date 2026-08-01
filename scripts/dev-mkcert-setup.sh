#!/usr/bin/env bash
# Generate local TLS certificates with mkcert for TrinityProxy development.
#
# Usage:
#   ./scripts/dev-mkcert-setup.sh [domain]
#   DOMAIN=myapp.local ./scripts/dev-mkcert-setup.sh
#
# Default domain: trinityproxy.local
# Creates certs for: <domain> and api.<domain>
# Writes PEM files to: .dev/certs/ (override with CERT_DIR)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

green() { echo -e "\033[32m$1\033[0m"; }
yellow() { echo -e "\033[33m$1\033[0m"; }
red() { echo -e "\033[31m$1\033[0m"; }

usage() {
    cat <<EOF
Usage: $0 [domain]

Generate mkcert TLS certificates for local TrinityProxy development.

Arguments:
  domain    Base local domain (default: trinityproxy.local)
            Also generates api.<domain>

Environment:
  CERT_DIR  Output directory for PEM files (default: .dev/certs)
  DOMAIN    Same as positional domain argument

Examples:
  $0
  $0 myproxy.test
  DOMAIN=trinityproxy.local CERT_DIR=/tmp/certs $0

After running:
  ./scripts/dev-hosts-helper.sh add ${DOMAIN}
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    DOMAIN="${DOMAIN:-trinityproxy.local}"
    usage
    exit 0
fi

DOMAIN="${1:-${DOMAIN:-trinityproxy.local}}"
API_DOMAIN="api.${DOMAIN}"
CERT_DIR="${CERT_DIR:-${REPO_ROOT}/.dev/certs}"
MARKER="# TrinityProxy dev"

ensure_mkcert() {
    if command -v mkcert >/dev/null 2>&1; then
        return 0
    fi

    yellow "[*] mkcert not found — attempting installation hints..."

    if [[ "$(uname -s)" == "Darwin" ]]; then
        if command -v brew >/dev/null 2>&1; then
            yellow "[*] Installing mkcert via Homebrew..."
            brew install mkcert nss
            return 0
        fi
        red "[-] Install Homebrew, then run: brew install mkcert nss"
        exit 1
    fi

    if command -v apt-get >/dev/null 2>&1; then
        yellow "[*] Installing mkcert via apt..."
        sudo apt-get update -y
        if apt-cache show mkcert >/dev/null 2>&1; then
            sudo apt-get install -y mkcert libnss3-tools
            return 0
        fi
        if command -v snap >/dev/null 2>&1; then
            yellow "[*] mkcert not in apt — trying snap..."
            sudo snap install mkcert
            return 0
        fi
        red "[-] Install mkcert manually:"
        red "    sudo apt-get install libnss3-tools"
        red "    See https://github.com/FiloSottile/mkcert#installation"
        exit 1
    fi

    if command -v dnf >/dev/null 2>&1; then
        yellow "[*] Try: sudo dnf install mkcert  (or build from source)"
    elif command -v pacman >/dev/null 2>&1; then
        yellow "[*] Try: sudo pacman -S mkcert"
    fi

    red "[-] mkcert is required. See https://github.com/FiloSottile/mkcert#installation"
    exit 1
}

main() {
    echo "[+] TrinityProxy mkcert setup"
    echo "============================"
    echo "[*] Domain:     ${DOMAIN}"
    echo "[*] API domain: ${API_DOMAIN}"
    echo "[*] Cert dir:   ${CERT_DIR}"
    echo ""

    ensure_mkcert

    yellow "[*] Installing local CA (may prompt for sudo)..."
    mkcert -install

    mkdir -p "${CERT_DIR}"

    yellow "[*] Generating certificates..."
    (
        cd "${CERT_DIR}"
        mkcert -cert-file "${DOMAIN}+1.pem" -key-file "${DOMAIN}+1-key.pem" \
            "${DOMAIN}" "${API_DOMAIN}"
    )

    local cert_file key_file ca_root
    cert_file="${CERT_DIR}/${DOMAIN}+1.pem"
    key_file="${CERT_DIR}/${DOMAIN}+1-key.pem"
    ca_root="$(mkcert -CAROOT)/rootCA.pem"

    echo ""
    green "[+] Certificates ready"
    echo ""
    echo "  CERT_FILE=${cert_file}"
    echo "  KEY_FILE=${key_file}"
    echo "  MKCERT_CA=${ca_root}"
    echo ""
    echo "Caddyfile example:"
    echo "  ${API_DOMAIN} {"
    echo "    tls ${cert_file} ${key_file}"
    echo "    reverse_proxy 127.0.0.1:3100"
    echo "  }"
    echo ""
    echo "/etc/hosts entry (${MARKER}):"
    echo "  127.0.0.1 ${DOMAIN} ${API_DOMAIN} ${MARKER}"
    echo ""
    echo "Add it with:"
    echo "  ./scripts/dev-hosts-helper.sh add ${DOMAIN}"
    echo ""
    echo "Controller URL suggestions:"
    echo "  # Plain HTTP (no TLS terminator):"
    echo "  export CONTROLLER_URL=http://${API_DOMAIN}:3100"
    echo ""
    echo "  # HTTPS via local reverse proxy (Caddy on :443):"
    echo "  export CONTROLLER_URL=https://${API_DOMAIN}"
    echo ""
    echo "  # Dashboard API (separate from controller):"
    echo "  export CONTROLLER_URL=http://${API_DOMAIN}:3100 make run-dashboard"
    echo ""
    yellow "[!] Trust the mkcert CA in browsers that use their own trust store."
    yellow "[!] Do not commit ${CERT_DIR}/ — keep certs local only."
}

main "$@"
