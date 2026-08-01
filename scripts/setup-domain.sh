#!/usr/bin/env bash
#
# Interactive TrinityProxy domain + Cloudflare wildcard SSL setup (VPS/server CLI).
# Wraps scripts/setup-ssl-caddy-cloudflare.sh with guided prompts.
#
# Usage:
#   sudo ./scripts/setup-domain.sh
#   sudo make setup-domain
#   sudo ./scripts/setup-domain.sh --from-bootstrap
#
# Non-interactive (all required vars set, or --non-interactive):
#   sudo PUBLIC_DOMAIN=example.com CLOUDFLARE_API_TOKEN=... SERVER_IP=... SKIP_DNS_WAIT=1 ./scripts/setup-domain.sh
#
# Exit codes: 0 success, 1 failure, 2 skipped (no changes / continue without HTTPS)

set -euo pipefail

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

FROM_BOOTSTRAP=0
NON_INTERACTIVE=0

err() {
	echo "[!] $*" >&2
}

info() {
	echo "[*] $*"
}

usage() {
	cat <<'EOF'
Usage:
  sudo ./scripts/setup-domain.sh [--from-bootstrap] [--non-interactive]

Options:
  --from-bootstrap   Called from make start; user cancel / skip returns exit 2
  --non-interactive  Require PUBLIC_DOMAIN, CLOUDFLARE_API_TOKEN, SERVER_IP (no prompts)

Exit codes: 0 success, 1 failure, 2 skipped
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--from-bootstrap)
		FROM_BOOTSTRAP=1
		shift
		;;
	--non-interactive)
		NON_INTERACTIVE=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		err "Unknown option: $1 (try --help)"
		exit 1
		;;
	esac
done

source_production_common() {
	local candidate
	for candidate in \
		"$SCRIPT_DIR/lib/production-common.sh" \
		"$ROOT/scripts/lib/production-common.sh" \
		"${OPT_SCRIPTS_DIR:-/opt/trinityproxy/scripts}/lib/production-common.sh"; do
		if [[ -f "$candidate" ]]; then
			# shellcheck source=scripts/lib/production-common.sh
			source "$candidate"
			return 0
		fi
	done
	err "Could not find production-common.sh (re-run from git checkout or make start)."
	exit 1
}

source_production_common

SSL_SCRIPT="${SSL_SCRIPT:-$SCRIPT_DIR/setup-ssl-caddy-cloudflare.sh}"
if [[ ! -f "$SSL_SCRIPT" ]]; then
	SSL_SCRIPT="${OPT_SCRIPTS_DIR:-/opt/trinityproxy/scripts}/setup-ssl-caddy-cloudflare.sh"
fi

PUBLIC_DOMAIN="${PUBLIC_DOMAIN:-}"
CLOUDFLARE_API_TOKEN="${CLOUDFLARE_API_TOKEN:-}"
SERVER_IP="${SERVER_IP:-}"
EMAIL="${EMAIL:-}"
SKIP_DNS_WAIT="${SKIP_DNS_WAIT:-0}"
SKIP_CONFIRM="${SKIP_CONFIRM:-0}"

if [[ $NON_INTERACTIVE -eq 1 ]] || {
	[[ -n "$PUBLIC_DOMAIN" ]] && [[ -n "$CLOUDFLARE_API_TOKEN" ]] && [[ -n "$SERVER_IP" ]]
}; then
	NON_INTERACTIVE=1
	SKIP_CONFIRM="${SKIP_CONFIRM:-1}"
	if [[ "$SKIP_DNS_WAIT" != "1" ]]; then
		SKIP_DNS_WAIT=1
	fi
fi

section() {
	echo ""
	echo "================================================================="
	echo " $*"
	echo "================================================================="
	echo ""
}

exit_skipped() {
	local msg="${1:-Skipped — no changes made.}"
	info "$msg"
	exit 2
}

require_root() {
	if [[ ${EUID:-0} -ne 0 ]]; then
		err "Run as root on your VPS (e.g. sudo $0)"
		exit 1
	fi
}

normalize_domain() {
	local d="$1"
	d="${d#http://}"
	d="${d#https://}"
	d="${d%%/*}"
	d="${d// /}"
	if command -v tr >/dev/null 2>&1; then
		d="$(echo "$d" | tr '[:upper:]' '[:lower:]')"
	fi
	d="${d%.}"
	echo "$d"
}

validate_domain() {
	local d="$1"
	if [[ -z "$d" ]]; then
		err "Domain cannot be empty."
		return 1
	fi
	if [[ "$d" == *"/"* ]] || [[ "$d" == *" "* ]]; then
		err "Enter a bare domain only (e.g. example.com), not a URL."
		return 1
	fi
	if [[ "$d" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$ ]]; then
		return 0
	fi
	err "Invalid domain: $d"
	return 1
}

detect_server_ip() {
	local ip src=""
	if [[ -n "$SERVER_IP" ]]; then
		if production_is_ipv4 "$SERVER_IP"; then
			info "Using SERVER_IP from environment: $SERVER_IP"
			return 0
		fi
		err "SERVER_IP is set but not a valid IPv4 address: $SERVER_IP"
		return 1
	fi

	if ip="$(production_detect_public_ip 2>/dev/null || true)"; then
		SERVER_IP="$ip"
		src="cloud metadata / public IP lookup"
	elif ip="$(production_detect_primary_ip 2>/dev/null || true)"; then
		SERVER_IP="$ip"
		src="local network interface"
	else
		return 1
	fi

	info "Detected server IPv4 ($src): $SERVER_IP"
	if production_is_private_ip "$SERVER_IP"; then
		echo ""
		err "Detected IP looks private ($SERVER_IP). Cloudflare A records need your VPS public IPv4."
		echo "    Confirm in your cloud console (GCP external IP, AWS Elastic IP, etc.)."
	fi
	return 0
}

prompt_server_ip() {
	local default="$SERVER_IP" input=""
	if [[ -n "$default" ]]; then
		read -r -p "[?] Server public IPv4 for DNS A records [$default]: " input
		input="${input:-$default}"
	else
		read -r -p "[?] Server public IPv4 for DNS A records: " input
	fi
	input="$(production_trim_ip "$input")"
	if ! production_is_ipv4 "$input"; then
		err "Invalid IPv4: $input"
		return 1
	fi
	SERVER_IP="$input"
	return 0
}

prompt_domain() {
	local default input
	default=""
	if [[ -n "$PUBLIC_DOMAIN" ]]; then
		default="$(normalize_domain "$PUBLIC_DOMAIN")"
	fi
	if [[ -n "$default" ]]; then
		read -r -p "[?] Public domain (bare, e.g. example.com) [$default]: " input
		input="${input:-$default}"
	else
		read -r -p "[?] Public domain (bare, e.g. example.com): " input
	fi
	PUBLIC_DOMAIN="$(normalize_domain "$input")"
	validate_domain "$PUBLIC_DOMAIN"
}

print_cloudflare_dns_checklist() {
	local domain="$1" ip="$2"
	section "Cloudflare DNS checklist for wildcard SSL"
	cat <<EOF
  1. Add A record: ${domain} → ${ip} (Proxied / orange cloud)
  2. Add A record: api.${domain} → ${ip} (Proxied / orange cloud)
  3. API token needs: Zone DNS Edit + Zone Read (template: Edit zone DNS)
  4. Wildcard cert covers *.${domain} via DNS-01

  Orange cloud (proxied) is recommended — visitors hit Cloudflare; origin stays ${ip}.

  Verify after saving DNS:
    dig +short A ${domain}
    dig +short A api.${domain}
EOF
}

wait_for_dns_ready() {
	if [[ "$SKIP_DNS_WAIT" == "1" ]]; then
		info "SKIP_DNS_WAIT=1 — skipping DNS confirmation pause"
		return 0
	fi
	if [[ ! -t 0 ]]; then
		info "Non-interactive stdin — set SKIP_DNS_WAIT=1 to skip DNS wait"
		return 0
	fi
	echo ""
	read -r -p "[?] Press Enter when DNS records are saved in Cloudflare: " _
}

prompt_cloudflare_token() {
	local token=""
	if [[ -n "$CLOUDFLARE_API_TOKEN" ]]; then
		info "Using CLOUDFLARE_API_TOKEN from environment"
		return 0
	fi
	echo ""
	echo "Create token: Cloudflare Dashboard → My Profile → API Tokens → Edit zone DNS"
	read -r -s -p "[?] Cloudflare API token (input hidden): " token
	echo ""
	if [[ -z "$token" ]]; then
		err "Cloudflare API token is required for DNS-01 wildcard issuance."
		return 1
	fi
	CLOUDFLARE_API_TOKEN="$token"
	return 0
}

prompt_email() {
	local default input
	if [[ -n "$EMAIL" ]]; then
		return 0
	fi
	default="ssl@${PUBLIC_DOMAIN}"
	read -r -p "[?] ACME / Let's Encrypt contact email [$default]: " input
	EMAIL="${input:-$default}"
}

confirm_summary() {
	if [[ "$SKIP_CONFIRM" == "1" ]]; then
		return 0
	fi
	if [[ ! -t 0 ]]; then
		info "Non-interactive — proceeding (set SKIP_CONFIRM=1 to suppress)"
		return 0
	fi
	section "Confirm setup"
	cat <<EOF
  Domain:     ${PUBLIC_DOMAIN}
  API host:   api.${PUBLIC_DOMAIN}
  Server IP:  ${SERVER_IP}
  Email:      ${EMAIL}
  SSL engine: ${SSL_SCRIPT}
EOF
	echo ""
	local ans=""
	read -r -p "[?] Proceed with Caddy + Cloudflare DNS-01 provisioning? [y/N]: " ans
	case "$ans" in
	y | Y | yes | YES) return 0 ;;
	*)
		exit_skipped "Cancelled — no changes made."
		;;
	esac
}

handle_ssl_failure() {
	local ssl_rc="$1"
	if [[ -n "${PUBLIC_DOMAIN:-}" ]]; then
		if declare -F production_update_controller_env_domain >/dev/null 2>&1; then
			production_update_controller_env_domain "$PUBLIC_DOMAIN" || true
		fi
		sync_dashboard_domain_settings "none" ""
	fi
	err "SSL / Caddy provisioning failed (exit ${ssl_rc})."
	echo ""
	echo "[!] Caddy journal (last 20 lines):"
	if type production_journalctl >/dev/null 2>&1; then
		production_journalctl -u caddy -n 20 --no-pager 2>/dev/null || true
	elif command -v journalctl >/dev/null 2>&1; then
		journalctl -u caddy -n 20 --no-pager 2>/dev/null || true
	else
		echo "    Check: sudo journalctl -u caddy -n 20 --no-pager"
	fi
	echo ""
	echo "    Re-run later: sudo ${OPT_SCRIPTS_DIR:-/opt/trinityproxy/scripts}/setup-domain.sh"
	echo "                  or: sudo ./scripts/setup-domain.sh"
	echo ""

	if [[ $FROM_BOOTSTRAP -eq 1 ]] && [[ -t 0 ]]; then
		local ans=""
		read -r -p "[?] Continue bootstrap without HTTPS? (dashboard stays on :8081) [Y/n]: " ans
		case "$ans" in
		n | N | no | NO)
			err "Bootstrap stopped — fix SSL or skip domain setup and re-run make start."
			exit 1
			;;
		*)
			exit_skipped "Continuing without HTTPS."
			;;
		esac
	fi

	exit 1
}


sync_dashboard_domain_settings() {
	local ssl_mode="${1:-}"
	local controller_url="${2:-}"
	if declare -F production_sync_deployment_settings >/dev/null 2>&1; then
		production_sync_deployment_settings "$PUBLIC_DOMAIN" "$ssl_mode" "$controller_url" || 			err "Dashboard settings sync failed — run: sudo make sync-deployment-settings"
		return 0
	fi
	local script="${SSL_SCRIPT%/*}/sync-deployment-settings.sh"
	if [[ -f "$script" ]]; then
		export TRINITY_SYNC_PUBLIC_DOMAIN="$PUBLIC_DOMAIN"
		export TRINITY_SYNC_FORCE=1
		[[ -n "$ssl_mode" ]] && export TRINITY_SYNC_SSL_MODE="$ssl_mode"
		[[ -n "$controller_url" ]] && export TRINITY_SYNC_CONTROLLER_URL="$controller_url"
		bash "$script" || err "Dashboard settings sync failed — run: sudo make sync-deployment-settings"
	fi
}

run_ssl_engine() {
	if [[ ! -f "$SSL_SCRIPT" ]]; then
		err "SSL setup script not found: $SSL_SCRIPT"
		err "Run from a git checkout or install production scripts (make start)."
		exit 1
	fi
	chmod +x "$SSL_SCRIPT" 2>/dev/null || true

	export PUBLIC_DOMAIN CLOUDFLARE_API_TOKEN SERVER_IP EMAIL
	export SKIP_DNS_WAIT=1

	info "Running SSL provisioning engine..."
	echo ""
	set +e
	bash "$SSL_SCRIPT"
	local ssl_rc=$?
	set -e
	if [[ $ssl_rc -ne 0 ]]; then
		handle_ssl_failure "$ssl_rc"
	fi

	sync_dashboard_domain_settings "caddy" "https://api.${PUBLIC_DOMAIN}"
}

main() {
	section "TrinityProxy domain + Cloudflare wildcard SSL"
	echo "  Interactive setup for HTTPS on your VPS (Caddy reverse proxy + DNS-01)."
	echo "  Safe to re-run — updates Caddy config and renewal token as needed."
	echo ""

	require_root

	if ! detect_server_ip; then
		SERVER_IP=""
		err "Could not detect a public IPv4 address automatically."
		echo "    Enter your VPS public IPv4 when prompted."
	fi

	if [[ $NON_INTERACTIVE -eq 1 ]]; then
		if [[ -z "$SERVER_IP" ]] || ! production_is_ipv4 "$SERVER_IP"; then
			err "SERVER_IP is required for non-interactive mode."
			exit 1
		fi
		if [[ -z "$PUBLIC_DOMAIN" ]]; then
			err "PUBLIC_DOMAIN is required for non-interactive mode."
			exit 1
		fi
		PUBLIC_DOMAIN="$(normalize_domain "$PUBLIC_DOMAIN")"
		validate_domain "$PUBLIC_DOMAIN" || exit 1
		if [[ -z "$CLOUDFLARE_API_TOKEN" ]]; then
			err "CLOUDFLARE_API_TOKEN is required for non-interactive mode."
			exit 1
		fi
	else
		if [[ -t 0 ]]; then
			while ! prompt_server_ip; do
				echo "    Try again or export SERVER_IP=... before running."
			done
		else
			if [[ -z "$SERVER_IP" ]] || ! production_is_ipv4 "$SERVER_IP"; then
				err "SERVER_IP is required when stdin is not a TTY."
				exit 1
			fi
		fi

		if [[ -t 0 ]]; then
			while ! prompt_domain; do
				:
			done
		else
			if [[ -z "$PUBLIC_DOMAIN" ]]; then
				err "PUBLIC_DOMAIN is required when stdin is not a TTY."
				exit 1
			fi
			PUBLIC_DOMAIN="$(normalize_domain "$PUBLIC_DOMAIN")"
			validate_domain "$PUBLIC_DOMAIN"
		fi
	fi

	EMAIL="${EMAIL:-ssl@${PUBLIC_DOMAIN}}"

	print_cloudflare_dns_checklist "$PUBLIC_DOMAIN" "$SERVER_IP"
	wait_for_dns_ready

	prompt_cloudflare_token || exit 1
	prompt_email
	confirm_summary
	run_ssl_engine

	echo ""
	info "Done. Set agents: CONTROLLER_URL=https://api.${PUBLIC_DOMAIN}"
}

main
