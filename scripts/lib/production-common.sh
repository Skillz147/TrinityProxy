#!/usr/bin/env bash
# Shared helpers for TrinityProxy production bootstrap (sourced, not executed).

TRINITY_DIR="${TRINITY_DIR:-/etc/trinityproxy}"
CONTROLLER_ENV="${CONTROLLER_ENV:-$TRINITY_DIR/controller.env}"
STATE_DIR="${STATE_DIR:-/var/lib/trinityproxy}"
DASHBOARD_USER="${DASHBOARD_USER:-trinityproxy}"
API_PORT="${API_PORT:-3100}"
DB_PATH="${DB_PATH:-$STATE_DIR/trinityproxy.db}"
DASHBOARD_PORT="${DASHBOARD_PORT:-8081}"

production_random_hex() {
	openssl rand -hex 32
}

production_detect_primary_ip() {
	local ip=""
	if command -v hostname >/dev/null 2>&1; then
		ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
	fi
	if [[ -z "$ip" ]] && command -v ip >/dev/null 2>&1; then
		ip="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}')"
	fi
	echo "$ip"
}

production_read_env_value() {
	local file="$1" key="$2"
	if [[ ! -f "$file" ]]; then
		return 1
	fi
	local line
	line="$(grep -E "^${key}=" "$file" | tail -1 || true)"
	[[ -n "$line" ]] || return 1
	echo "${line#*=}"
}

# Create a system user for controller/dashboard (idempotent).
production_create_system_user() {
	local user="${1:-$DASHBOARD_USER}"
	local home="${2:-$STATE_DIR}"

	if id "$user" &>/dev/null; then
		return 0
	fi

	echo "[*] Creating system user: $user"
	if command -v adduser >/dev/null 2>&1; then
		adduser --system --group --home "$home" --no-create-home --disabled-login "$user"
	elif command -v useradd >/dev/null 2>&1; then
		useradd --system --no-create-home --shell /usr/sbin/nologin \
			--home-dir "$home" "$user"
	else
		echo "[-] Error: neither adduser nor useradd available (install adduser or passwd)"
		return 1
	fi
}

production_ensure_trinityproxy_user() {
	production_create_system_user "$DASHBOARD_USER" "$STATE_DIR"
}

production_ensure_state_dir() {
	install -d -o "$DASHBOARD_USER" -g "$DASHBOARD_USER" -m 750 "$STATE_DIR"
}

production_ensure_controller_env() {
	echo "[*] Preparing controller secrets..."
	install -d -m 750 "$TRINITY_DIR"

	local api_key agent_key controller_url
	api_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_API_KEY || true)"
	agent_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_AGENT_KEY || true)"
	controller_url="$(production_read_env_value "$CONTROLLER_ENV" CONTROLLER_URL || true)"

	[[ -n "$api_key" ]] || api_key="$(production_random_hex)"
	[[ -n "$agent_key" ]] || agent_key="$(production_random_hex)"

	if [[ -z "$controller_url" ]]; then
		local ip
		ip="$(production_detect_primary_ip)"
		if [[ -n "$ip" ]]; then
			controller_url="http://${ip}:${API_PORT}"
		else
			controller_url="http://127.0.0.1:${API_PORT}"
		fi
	fi

	cat >"$CONTROLLER_ENV" <<EOF
TRINITY_API_KEY=${api_key}
TRINITY_AGENT_KEY=${agent_key}
API_PORT=${API_PORT}
DB_PATH=${DB_PATH}
CONTROLLER_URL=${controller_url}
EOF
	chmod 640 "$CONTROLLER_ENV"
	chown root:"$DASHBOARD_USER" "$CONTROLLER_ENV" 2>/dev/null || chmod 640 "$CONTROLLER_ENV"
	echo "[+] Wrote $CONTROLLER_ENV"
}

production_sync_agent_key_to_controller_env() {
	local db="$STATE_DIR/dashboard.db"
	if [[ ! -f "$db" ]] || ! command -v sqlite3 >/dev/null 2>&1; then
		return 0
	fi
	local key
	key="$(sqlite3 "$db" "SELECT agent_key FROM dashboard_deployment WHERE id = 1;" 2>/dev/null || true)"
	key="${key//$'\r'/}"
	key="${key//$'\n'/}"
	if [[ -z "$key" ]]; then
		return 0
	fi
	local current
	current="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_AGENT_KEY || true)"
	if [[ "$current" == "$key" ]]; then
		return 0
	fi
	local api_key controller_url api_port db_path
	api_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_API_KEY)"
	controller_url="$(production_read_env_value "$CONTROLLER_ENV" CONTROLLER_URL)"
	api_port="$(production_read_env_value "$CONTROLLER_ENV" API_PORT || echo "$API_PORT")"
	db_path="$(production_read_env_value "$CONTROLLER_ENV" DB_PATH || echo "$DB_PATH")"
	cat >"$CONTROLLER_ENV" <<EOF
TRINITY_API_KEY=${api_key}
TRINITY_AGENT_KEY=${key}
API_PORT=${api_port}
DB_PATH=${db_path}
CONTROLLER_URL=${controller_url}
EOF
	chmod 640 "$CONTROLLER_ENV"
	chown root:"$DASHBOARD_USER" "$CONTROLLER_ENV" 2>/dev/null || true
	echo "[+] Synced TRINITY_AGENT_KEY from dashboard DB to $CONTROLLER_ENV"
}

production_init_dashboard_admin() {
	echo "[*] Initializing dashboard admin + agent key..."
	production_ensure_trinityproxy_user
	production_ensure_state_dir

	local ip dashboard_url
	ip="$(production_detect_primary_ip)"
	if [[ -n "$ip" ]]; then
		dashboard_url="http://${ip}:${DASHBOARD_PORT}"
	else
		dashboard_url="http://127.0.0.1:${DASHBOARD_PORT}"
	fi

	set -a
	# shellcheck disable=SC1090
	source "$CONTROLLER_ENV"
	set +a

	export DASHBOARD_DB_PATH="$STATE_DIR/dashboard.db"
	export DB_PATH="$DB_PATH"
	export DASHBOARD_URL="$dashboard_url"
	export TRINITY_BOOTSTRAP_DEFER_PRINT=1

	PRODUCTION_DASHBOARD_ADMIN_CREATED=""
	PRODUCTION_DASHBOARD_URL=""
	PRODUCTION_DASHBOARD_USERNAME=""
	PRODUCTION_DASHBOARD_PASSWORD=""

	while IFS= read -r line; do
		case "$line" in
		TRINITY_BOOTSTRAP_CREATED=*) PRODUCTION_DASHBOARD_ADMIN_CREATED="${line#TRINITY_BOOTSTRAP_CREATED=}" ;;
		TRINITY_BOOTSTRAP_URL=*) PRODUCTION_DASHBOARD_URL="${line#TRINITY_BOOTSTRAP_URL=}" ;;
		TRINITY_BOOTSTRAP_USERNAME=*) PRODUCTION_DASHBOARD_USERNAME="${line#TRINITY_BOOTSTRAP_USERNAME=}" ;;
		TRINITY_BOOTSTRAP_PASSWORD=*) PRODUCTION_DASHBOARD_PASSWORD="${line#TRINITY_BOOTSTRAP_PASSWORD=}" ;;
		esac
	done < <(./build/trinityproxy-dashboard --init-only)

	if [[ -f "$STATE_DIR/dashboard.db" ]]; then
		chown "$DASHBOARD_USER:$DASHBOARD_USER" "$STATE_DIR/dashboard.db"
		chmod 640 "$STATE_DIR/dashboard.db"
	fi
}

production_caddy_active() {
	[[ -f /etc/caddy/trinityproxy.caddy ]] && command -v caddy >/dev/null 2>&1
}

production_print_dashboard_login_banner() {
	local sep="============================================================"
	echo ""
	echo "$sep"
	echo "  DASHBOARD LOGIN — save these credentials"
	echo "$sep"
	if [[ "${PRODUCTION_DASHBOARD_ADMIN_CREATED:-}" == "1" ]]; then
		echo ""
		echo "  Dashboard URL:  ${PRODUCTION_DASHBOARD_URL}"
		echo "  Username:       ${PRODUCTION_DASHBOARD_USERNAME}"
		echo "  Password:       ${PRODUCTION_DASHBOARD_PASSWORD}"
		echo ""
		echo "  First login requires a password change."
	else
		echo ""
		echo "  Dashboard admin already exists — no new password was generated."
		echo "  Use your existing password or reset the dashboard database."
	fi
	echo "$sep"
}


production_print_summary() {
	local ip
	ip="$(production_detect_primary_ip)"
	echo ""
	echo "============================================"
	echo "  TrinityProxy production bootstrap complete"
	echo "============================================"
	echo ""
	echo "Services (auto-start on reboot):"
	echo "  trinityproxy-controller  → :${API_PORT}"
	echo "  trinityproxy-dashboard   → :${DASHBOARD_PORT} (API + embedded UI)"
	echo ""
	if production_caddy_active; then
		echo "Dashboard URL: https://<your-domain> (Caddy reverse proxy active)"
		echo "Controller API:  https://api.<your-domain> (via Caddy)"
		echo "  (Direct HTTP:  http://${ip:-127.0.0.1}:${DASHBOARD_PORT})"
	else
		if [[ -n "$ip" ]]; then
			echo "Dashboard URL: http://${ip}:${DASHBOARD_PORT}"
		else
			echo "Dashboard URL: http://127.0.0.1:${DASHBOARD_PORT}"
		fi
		echo "Controller API:  http://${ip:-127.0.0.1}:${API_PORT}"
		echo ""
		echo "HTTPS: Settings → Cloudflare SSL in dashboard, or:"
		echo "  sudo ./scripts/setup-ssl-caddy-cloudflare.sh"
	fi
	production_print_dashboard_login_banner
	echo ""
	echo "Controller secrets file: $CONTROLLER_ENV"
	echo "  (TRINITY_API_KEY, TRINITY_AGENT_KEY, API_PORT, DB_PATH, CONTROLLER_URL)"
	echo ""
	echo "Service commands:"
	echo "  sudo systemctl status trinityproxy-controller trinityproxy-dashboard"
	echo "  sudo journalctl -u trinityproxy-controller -f"
	echo "  sudo journalctl -u trinityproxy-dashboard -f"
}
