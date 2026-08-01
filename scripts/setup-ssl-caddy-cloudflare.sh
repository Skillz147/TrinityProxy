#!/bin/bash
#
# Caddy reverse proxy + Cloudflare DNS-01 wildcard Let's Encrypt for TrinityProxy.
# Issues certs for *.{PUBLIC_DOMAIN} and {PUBLIC_DOMAIN}; proxies api.{domain} → :3100
# and {domain} → :8081 (dashboard). Supports orange cloud (proxied) DNS records.
#
# Run on VPS (this is the only supported SSL setup path — not the dashboard UI):
#   sudo PUBLIC_DOMAIN=example.com CLOUDFLARE_API_TOKEN=your_token SERVER_IP=203.0.113.10 \
#     EMAIL=ssl@example.com SKIP_DNS_WAIT=1 /opt/trinityproxy/scripts/setup-ssl-caddy-cloudflare.sh
#
# From a git checkout on the VPS, use ./scripts/setup-ssl-caddy-cloudflare.sh instead.
# The API token is stored in /etc/caddy/cloudflare.env (mode 600) for automatic renewal.

set -euo pipefail

PUBLIC_DOMAIN="${PUBLIC_DOMAIN:-}"
CLOUDFLARE_API_TOKEN="${CLOUDFLARE_API_TOKEN:-}"
SERVER_IP="${SERVER_IP:-}"
SERVER_IPV6="${SERVER_IPV6:-}"
EMAIL="${EMAIL:-}"
SKIP_DNS_WAIT="${SKIP_DNS_WAIT:-0}"

CONTROLLER_UPSTREAM="${CONTROLLER_UPSTREAM:-127.0.0.1:3100}"
DASHBOARD_UPSTREAM="${DASHBOARD_UPSTREAM:-127.0.0.1:8081}"

API_HOST=""
CADDYFILE="/etc/caddy/Caddyfile"
CADDY_SITE="/etc/caddy/trinityproxy.caddy"
CADDY_ENV="/etc/caddy/cloudflare.env"
CADDY_DROPIN="/etc/systemd/system/caddy.service.d/cloudflare.env.conf"

print_usage() {
    cat <<'EOF'
Usage:
  sudo PUBLIC_DOMAIN=example.com CLOUDFLARE_API_TOKEN=... SERVER_IP=203.0.113.10 \
    EMAIL=ssl@example.com ./scripts/setup-ssl-caddy-cloudflare.sh

Optional:
  SERVER_IPV6=2001:db8::1
  SKIP_DNS_WAIT=1
  CONTROLLER_UPSTREAM=127.0.0.1:3100
  DASHBOARD_UPSTREAM=127.0.0.1:8081

DNS: proxied A records for api.{domain} and {domain} → SERVER_IP (orange cloud OK).
EOF
}

require_env() {
    local missing=0
    if [[ -z "$PUBLIC_DOMAIN" ]]; then
        echo "[!] PUBLIC_DOMAIN is required (bare domain, e.g. example.com)"
        missing=1
    fi
    if [[ -z "$CLOUDFLARE_API_TOKEN" ]]; then
        echo "[!] CLOUDFLARE_API_TOKEN is required for DNS-01 wildcard issuance"
        missing=1
    fi
    if [[ -z "$SERVER_IP" ]]; then
        echo "[!] SERVER_IP is required (VPS public IPv4 for proxied A records)"
        missing=1
    fi
    if [[ -z "$EMAIL" ]]; then
        EMAIL="ssl@${PUBLIC_DOMAIN}"
        echo "[*] EMAIL not set — using ${EMAIL}"
    fi
    if [[ "$missing" -ne 0 ]]; then
        echo ""
        echo "[!] SSL setup is server-only — configure these env vars and re-run this script on the VPS."
        print_usage
        exit 1
    fi
    API_HOST="api.${PUBLIC_DOMAIN}"
}

print_dns_checklist() {
    echo ""
    echo "================================================================="
    echo " Cloudflare DNS — proxied A records (orange cloud)"
    echo "================================================================="
    echo ""
    echo "  Type   Name / Host              Value              Proxy"
    echo "  ----   --------------------     ----------------   --------"
    echo "  A      ${API_HOST}.              ${SERVER_IP}       Proxied"
    if [[ -n "$SERVER_IPV6" ]]; then
        echo "  AAAA   ${API_HOST}.              ${SERVER_IPV6}       Proxied"
    fi
    echo "  A      ${PUBLIC_DOMAIN}.         ${SERVER_IP}       Proxied"
    if [[ -n "$SERVER_IPV6" ]]; then
        echo "  AAAA   ${PUBLIC_DOMAIN}.         ${SERVER_IPV6}       Proxied"
    fi
    echo ""
    echo "Wildcard cert covers *.${PUBLIC_DOMAIN} and ${PUBLIC_DOMAIN} via DNS-01."
    echo "Visitors see Cloudflare IPs; origin ${SERVER_IP} stays in DNS only."
    echo ""
    echo "Verify:"
    echo "  dig +short A ${API_HOST}"
    echo "  dig +short A ${PUBLIC_DOMAIN}"
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
    read -r -p "[?] Press Enter after proxied A records point to ${SERVER_IP}: " _
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
            echo "    Stop the conflicting service (e.g. nginx, apache2) or free the ports."
        fi
    fi
    mention_cloud_firewall_hint
}

mention_cloud_firewall_hint() {
    if curl -sf -m 2 -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/id >/dev/null 2>&1; then
        echo ""
        echo "[*] GCP VM detected: open TCP 80 and 443 on the VPC firewall / network tags"
        echo "    (e.g. gcloud compute firewall-rules create ... --allow=tcp:80,tcp:443)"
        echo "    Cloudflare orange-cloud still needs origin ports 80/443 reachable for HTTPS."
    fi
}

ensure_cloudflare_dns_module() {
    if caddy list-modules 2>/dev/null | grep -q 'dns.providers.cloudflare'; then
        echo "[*] Caddy Cloudflare DNS module already present"
        return 0
    fi

    echo "[*] Adding Caddy Cloudflare DNS module (required for DNS-01 wildcard)..."
    systemctl stop caddy 2>/dev/null || true
    if ! caddy add-package github.com/caddy-dns/cloudflare; then
        echo "[!] Failed to install Caddy Cloudflare DNS module (github.com/caddy-dns/cloudflare)"
        echo "    Check network access and retry, or use a Caddy build that includes the plugin."
        exit 1
    fi
    systemctl daemon-reload
    if ! caddy list-modules 2>/dev/null | grep -q 'dns.providers.cloudflare'; then
        echo "[!] Cloudflare DNS module still missing after add-package"
        exit 1
    fi
}

write_cloudflare_env() {
    echo "[*] Writing Cloudflare token env file: ${CADDY_ENV}"
    install -d -m 755 /etc/caddy
    umask 077
    printf 'CLOUDFLARE_API_TOKEN=%s\n' "$CLOUDFLARE_API_TOKEN" >"$CADDY_ENV"
    chmod 600 "$CADDY_ENV"
    if id caddy >/dev/null 2>&1; then
        chown root:caddy "$CADDY_ENV" 2>/dev/null || chown root:root "$CADDY_ENV"
    else
        chown root:root "$CADDY_ENV"
    fi
}

configure_systemd_env() {
    echo "[*] Configuring Caddy systemd EnvironmentFile for renewal"
    install -d -m 755 /etc/systemd/system/caddy.service.d
    cat >"$CADDY_DROPIN" <<EOF
[Service]
EnvironmentFile=${CADDY_ENV}
EOF
    systemctl daemon-reload
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
*.${PUBLIC_DOMAIN}, ${PUBLIC_DOMAIN} {
	tls {
		dns cloudflare {env.CLOUDFLARE_API_TOKEN}
	}

	@api host ${API_HOST}
	handle @api {
		encode gzip
		reverse_proxy ${CONTROLLER_UPSTREAM}
	}

	@dashboard host ${PUBLIC_DOMAIN}
	handle @dashboard {
		encode gzip
		reverse_proxy ${DASHBOARD_UPSTREAM}
	}
}
EOF
}

enable_caddy() {
    ensure_caddy_runtime_dirs

    if [[ -f "$CADDY_ENV" ]]; then
        set -a
        # shellcheck disable=SC1090
        source "$CADDY_ENV"
        set +a
    fi

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
    echo "[*] TrinityProxy Caddy + Cloudflare DNS-01 wildcard SSL setup"

    if [[ ${EUID:-0} -ne 0 ]]; then
        echo "[!] This script must be run as root on your VPS (use sudo)"
        echo "    Example: sudo PUBLIC_DOMAIN=example.com CLOUDFLARE_API_TOKEN=... SERVER_IP=... \\"
        echo "      EMAIL=ssl@example.com SKIP_DNS_WAIT=1 $0"
        exit 1
    fi

    require_env
    print_dns_checklist
    wait_for_dns_ready

    ensure_caddy_package
    ensure_cloudflare_dns_module
    write_cloudflare_env
    configure_systemd_env
    open_firewall_ports
    mention_cloud_firewall_hint
    write_global_caddyfile
    write_site_config
    enable_caddy

    echo ""
    echo "[+] Wildcard SSL and reverse proxy configured."
    echo ""
    echo "Certificate: *.${PUBLIC_DOMAIN}, ${PUBLIC_DOMAIN} (DNS-01 via Cloudflare)"
    echo "Controller:  https://${API_HOST}/  → ${CONTROLLER_UPSTREAM}"
    echo "Dashboard:   https://${PUBLIC_DOMAIN}/  → ${DASHBOARD_UPSTREAM}"
    echo ""
    echo "Token file:  ${CADDY_ENV} (used for automatic renewal — not stored in dashboard DB)"
    echo "Agent env:   CONTROLLER_URL=https://${API_HOST}"
    echo ""
    echo "Verify:"
    echo "  curl -sS https://${API_HOST}/health"
    echo "  systemctl status caddy"
    echo "  journalctl -u caddy -f"
}

main "$@"
