#!/bin/bash
#
# Caddy reverse proxy + automatic Let's Encrypt for TrinityProxy.
# Proxies CONTROLLER_DOMAIN → localhost:3100; optional DASHBOARD_DOMAIN → :8081.
# Requires root. Idempotent when re-run with the same env vars.
#
# Usage (non-interactive):
#   sudo CONTROLLER_DOMAIN=api.example.com SERVER_IP=203.0.113.10 EMAIL=admin@example.com \
#     ./scripts/setup-ssl-caddy.sh
#
# Optional dashboard:
#   sudo CONTROLLER_DOMAIN=api.example.com DASHBOARD_DOMAIN=dashboard.example.com \
#     SERVER_IP=203.0.113.10 EMAIL=admin@example.com ./scripts/setup-ssl-caddy.sh
#
# DNS checklist: scripts/setup-cloudflare-dns.md

set -euo pipefail

CONTROLLER_DOMAIN="${CONTROLLER_DOMAIN:-}"
DASHBOARD_DOMAIN="${DASHBOARD_DOMAIN:-}"
SERVER_IP="${SERVER_IP:-}"
SERVER_IPV6="${SERVER_IPV6:-}"
EMAIL="${EMAIL:-}"
SKIP_DNS_WAIT="${SKIP_DNS_WAIT:-0}"

CONTROLLER_UPSTREAM="${CONTROLLER_UPSTREAM:-127.0.0.1:3100}"
DASHBOARD_UPSTREAM="${DASHBOARD_UPSTREAM:-127.0.0.1:8081}"

CADDYFILE="/etc/caddy/Caddyfile"
CADDY_SITE="/etc/caddy/trinityproxy.caddy"

print_usage() {
    cat <<'EOF'
Usage:
  sudo CONTROLLER_DOMAIN=api.example.com SERVER_IP=203.0.113.10 EMAIL=you@example.com \
    ./scripts/setup-ssl-caddy.sh

Optional:
  DASHBOARD_DOMAIN=dashboard.example.com
  SERVER_IPV6=2001:db8::1          (shown in DNS checklist only)
  SKIP_DNS_WAIT=1                  skip "press Enter after DNS" prompt
  CONTROLLER_UPSTREAM=127.0.0.1:3100
  DASHBOARD_UPSTREAM=127.0.0.1:8081

See scripts/setup-cloudflare-dns.md for Cloudflare A/AAAA/CNAME and proxy notes.
EOF
}

require_env() {
    local missing=0
    if [[ -z "$CONTROLLER_DOMAIN" ]]; then
        echo "[!] CONTROLLER_DOMAIN is required (e.g. api.example.com)"
        missing=1
    fi
    if [[ -z "$SERVER_IP" ]]; then
        echo "[!] SERVER_IP is required (VPS public IPv4 for DNS A record)"
        missing=1
    fi
    if [[ -z "$EMAIL" ]]; then
        echo "[!] EMAIL is required for Let's Encrypt registration"
        missing=1
    fi
    if [[ "$missing" -ne 0 ]]; then
        echo ""
        print_usage
        exit 1
    fi
}

print_dns_checklist() {
    echo ""
    echo "================================================================="
    echo " DNS records — create BEFORE certificate issuance"
    echo "================================================================="
    echo ""
    echo "Controller API (required):"
    echo "  Type   Name / Host              Value"
    echo "  ----   --------------------     -------------------------"
    echo "  A      ${CONTROLLER_DOMAIN}.     ${SERVER_IP}"
    if [[ -n "$SERVER_IPV6" ]]; then
        echo "  AAAA   ${CONTROLLER_DOMAIN}.     ${SERVER_IPV6}"
    fi
    echo ""
    if [[ -n "$DASHBOARD_DOMAIN" ]]; then
        echo "Dashboard (optional):"
        echo "  Type   Name / Host              Value"
        echo "  ----   --------------------     -------------------------"
        echo "  A      ${DASHBOARD_DOMAIN}.     ${SERVER_IP}"
        if [[ -n "$SERVER_IPV6" ]]; then
            echo "  AAAA   ${DASHBOARD_DOMAIN}.     ${SERVER_IPV6}"
        fi
        echo ""
    fi
    echo "Cloudflare: use DNS only (grey cloud) for HTTP-01 until certs are issued."
    echo "Details: scripts/setup-cloudflare-dns.md"
    echo ""
    echo "Verify propagation:"
    echo "  dig +short A ${CONTROLLER_DOMAIN}"
    if [[ -n "$DASHBOARD_DOMAIN" ]]; then
        echo "  dig +short A ${DASHBOARD_DOMAIN}"
    fi
    echo "================================================================="
    echo ""
}

wait_for_dns_ready() {
    if [[ "$SKIP_DNS_WAIT" == "1" ]]; then
        echo "[*] SKIP_DNS_WAIT=1 — proceeding without interactive DNS confirmation"
        return 0
    fi
    if [[ ! -t 0 ]]; then
        echo "[*] Non-TTY stdin — proceeding (set SKIP_DNS_WAIT=1 to suppress this message)"
        return 0
    fi
    read -r -p "[?] Press Enter after DNS records point ${CONTROLLER_DOMAIN} → ${SERVER_IP}: " _
}

install_caddy() {
    if command -v caddy >/dev/null 2>&1; then
        echo "[*] Caddy already installed: $(caddy version 2>/dev/null | head -n1 || true)"
        return 0
    fi

    echo "[*] Installing Caddy..."
    if command -v apt-get >/dev/null 2>&1; then
        apt-get update -y
        apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl
        curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
            | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
        curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
            | tee /etc/apt/sources.list.d/caddy-stable.list
        apt-get update -y
        apt-get install -y caddy
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y 'dnf-command(copr)'
        dnf copr enable -y @caddy/caddy
        dnf install -y caddy
    elif command -v yum >/dev/null 2>&1; then
        yum install -y yum-plugin-copr
        yum copr enable -y @caddy/caddy
        yum install -y caddy
    else
        echo "[!] Unsupported package manager. Install Caddy manually: https://caddyserver.com/docs/install"
        exit 1
    fi
}


ensure_caddy_package() {
    if ! command -v caddy >/dev/null 2>&1; then
        install_caddy
    fi
    if ! command -v caddy >/dev/null 2>&1; then
        echo "[!] Caddy binary not found after install attempt"
        exit 1
    fi
    if ! systemctl list-unit-files caddy.service >/dev/null 2>&1; then
        echo "[!] caddy.service unit not found — reinstall the caddy package"
        exit 1
    fi
}

ensure_caddy_runtime_dirs() {
    echo "[*] Ensuring Caddy data and config directories..."
    install -d -m 755 /etc/caddy
    install -d -m 755 /var/lib/caddy
    if id caddy >/dev/null 2>&1; then
        chown -R caddy:caddy /var/lib/caddy 2>/dev/null || true
    fi
}

print_caddy_service_diagnostics() {
    echo ""
    echo "[!] caddy.service failed — recent journal entries:"
    journalctl -xeu caddy.service -n 30 --no-pager 2>/dev/null || journalctl -u caddy.service -n 30 --no-pager 2>/dev/null || true
    echo ""
    if command -v ss >/dev/null 2>&1; then
        if ss -tlnH 2>/dev/null | grep -qE ':80 |:443 '; then
            echo "[!] Something is already listening on port 80 and/or 443:"
            ss -tlnp 2>/dev/null | grep -E ':80 |:443 ' || true
        fi
    fi
    if curl -sf -m 2 -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/id >/dev/null 2>&1; then
        echo ""
        echo "[*] GCP VM: ensure VPC firewall allows tcp:80 and tcp:443 to this instance."
    fi
}

open_firewall_ports() {
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi active; then
        echo "[*] Opening HTTP/HTTPS in ufw..."
        ufw allow 80/tcp
        ufw allow 443/tcp
    fi
}

write_global_caddyfile() {
    if [[ -f "$CADDYFILE" ]] && grep -q 'import trinityproxy.caddy' "$CADDYFILE" 2>/dev/null; then
        echo "[*] Global Caddyfile already imports trinityproxy site config"
        return 0
    fi

    echo "[*] Writing global Caddyfile: $CADDYFILE"
    install -d -m 755 /etc/caddy
    cat >"$CADDYFILE" <<EOF
{
	email ${EMAIL}
}

import trinityproxy.caddy
EOF
}

write_site_config() {
    echo "[*] Writing site config: $CADDY_SITE"
    install -d -m 755 /etc/caddy

    cat >"$CADDY_SITE" <<EOF
${CONTROLLER_DOMAIN} {
	encode gzip
	reverse_proxy ${CONTROLLER_UPSTREAM}
}
EOF

    if [[ -n "$DASHBOARD_DOMAIN" ]]; then
        cat >>"$CADDY_SITE" <<EOF

${DASHBOARD_DOMAIN} {
	encode gzip
	reverse_proxy ${DASHBOARD_UPSTREAM}
}
EOF
    fi
}

enable_caddy() {
    ensure_caddy_runtime_dirs

    echo "[*] Validating Caddy configuration..."
    if ! caddy validate --config "$CADDYFILE"; then
        echo "[!] Caddy configuration validation failed"
        exit 1
    fi

    systemctl daemon-reload
    echo "[*] Enabling and starting Caddy..."
    systemctl enable caddy

    if systemctl is-active --quiet caddy 2>/dev/null; then
        if systemctl reload caddy; then
            echo "[*] Caddy reloaded"
            return 0
        fi
        echo "[*] Reload failed — attempting restart..."
    fi

    if systemctl restart caddy; then
        echo "[*] Caddy started"
        return 0
    fi

    print_caddy_service_diagnostics
    exit 1
}

main() {
    echo "[*] TrinityProxy Caddy SSL / reverse proxy setup"

    if [[ ${EUID:-0} -ne 0 ]]; then
        echo "[!] This script must be run as root (use sudo)"
        exit 1
    fi

    require_env
    print_dns_checklist
    wait_for_dns_ready

    ensure_caddy_package
    open_firewall_ports
    write_global_caddyfile
    write_site_config
    enable_caddy

    echo ""
    echo "[+] Caddy reverse proxy configured."
    echo ""
    echo "Controller API: https://${CONTROLLER_DOMAIN}/  → ${CONTROLLER_UPSTREAM}"
    if [[ -n "$DASHBOARD_DOMAIN" ]]; then
        echo "Dashboard:      https://${DASHBOARD_DOMAIN}/  → ${DASHBOARD_UPSTREAM}"
    fi
    echo ""
    echo "Let's Encrypt: automatic issue and renewal via Caddy"
    echo "Agent env:     CONTROLLER_URL=https://${CONTROLLER_DOMAIN}"
    echo ""
    echo "Verify:"
    echo "  curl -sS https://${CONTROLLER_DOMAIN}/health"
    echo "  systemctl status caddy"
    echo "  journalctl -u caddy -f"
}

main "$@"
