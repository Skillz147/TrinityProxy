#!/usr/bin/env bash
# Shared helpers for TrinityProxy production bootstrap (sourced, not executed).

export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}"

TRINITY_DIR="${TRINITY_DIR:-/etc/trinityproxy}"
CONTROLLER_ENV="${CONTROLLER_ENV:-$TRINITY_DIR/controller.env}"
STATE_DIR="${STATE_DIR:-/var/lib/trinityproxy}"
DASHBOARD_USER="${DASHBOARD_USER:-trinityproxy}"
API_PORT="${API_PORT:-3100}"
DB_PATH="${DB_PATH:-$STATE_DIR/trinityproxy.db}"
DASHBOARD_PORT="${DASHBOARD_PORT:-8081}"

OPT_PREFIX="${OPT_PREFIX:-/opt/trinityproxy}"
OPT_BIN_DIR="${OPT_BIN_DIR:-$OPT_PREFIX/bin}"

production_is_dev_install() {
	[[ "${TRINITY_DEV:-}" == "1" ]]
}

production_install_binaries() {
	local project_root="$1"
	shift
	local name src
	production_install -d -o root -g "$DASHBOARD_USER" -m 750 "$OPT_BIN_DIR"
	for name in "$@"; do
		src="$project_root/build/$name"
		if [[ ! -f "$src" ]]; then
			echo "[-] Missing binary: $src" >&2
			return 1
		fi
		echo "[*] Installing $name -> $OPT_BIN_DIR/$name"
		production_install -o root -g "$DASHBOARD_USER" -m 750 "$src" "$OPT_BIN_DIR/$name"
	done
}

production_install_systemd_unit() {
	local template="$1"
	local dest="$2"
	local project_root="$3"
	if production_is_dev_install; then
		echo "[*] Dev install (TRINITY_DEV=1): using build/ paths under $project_root"
		sed \
			-e "s|WorkingDirectory=/var/lib/trinityproxy|WorkingDirectory=$project_root|g" \
			-e "s|ExecStart=/opt/trinityproxy/bin/|ExecStart=$project_root/build/|g" \
			-e "s|ReadOnlyPaths=/opt/trinityproxy/bin|ReadOnlyPaths=$project_root/build|g" \
			"$template" >"$dest"
	else
		echo "[*] Production install: $dest (binaries under $OPT_BIN_DIR)"
		cp "$template" "$dest"
	fi
	chmod 644 "$dest"
}


production_resolve_cmd() {
	local name="$1"
	local dir p
	for dir in /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; do
		p="$dir/$name"
		if [[ -x "$p" ]]; then
			echo "$p"
			return 0
		fi
	done
	if command -v "$name" >/dev/null 2>&1; then
		command -v "$name"
		return 0
	fi
	return 1
}

production_install() {
	local bin
	bin="$(production_resolve_cmd install)" || {
		echo "[-] Error: install not found" >&2
		return 1
	}
	"$bin" "$@"
}

production_systemctl() {
	local bin
	bin="$(production_resolve_cmd systemctl)" || {
		echo "[-] Error: systemctl not found" >&2
		return 1
	}
	"$bin" "$@"
}

production_find_adduser() {
	local p
	for p in /usr/sbin/adduser /sbin/adduser; do
		if [[ -x "$p" ]]; then
			echo "$p"
			return 0
		fi
	done
	production_resolve_cmd adduser
}

production_find_useradd() {
	local p
	for p in /usr/sbin/useradd /sbin/useradd; do
		if [[ -x "$p" ]]; then
			echo "$p"
			return 0
		fi
	done
	production_resolve_cmd useradd
}

production_have_user_mgmt() {
	production_find_adduser >/dev/null 2>&1 || production_find_useradd >/dev/null 2>&1
}

production_random_hex() {
	local openssl_bin
	openssl_bin="$(production_resolve_cmd openssl)" || {
		echo "[-] Error: openssl not found" >&2
		return 1
	}
	"$openssl_bin" rand -hex 32
}

production_is_loopback_ipv4() {
	local ip="$1"
	[[ "$ip" == 127.* ]]
}

production_detect_primary_ip() {
	local ip="" hostname_bin awk_bin ip_bin curl_bin addr
	hostname_bin="$(production_resolve_cmd hostname 2>/dev/null || true)"
	awk_bin="$(production_resolve_cmd awk 2>/dev/null || true)"
	ip_bin="$(production_resolve_cmd ip 2>/dev/null || true)"
	curl_bin="$(production_resolve_cmd curl 2>/dev/null || true)"

	if [[ -n "$hostname_bin" ]]; then
		for addr in $($hostname_bin -I 2>/dev/null); do
			[[ -z "$addr" ]] && continue
			production_is_loopback_ipv4 "$addr" && continue
			ip="$addr"
			break
		done
	fi
	if [[ -z "$ip" && -n "$ip_bin" && -n "$awk_bin" ]]; then
		ip="$("$ip_bin" -4 route get 1.1.1.1 2>/dev/null | "$awk_bin" '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}')"
		production_is_loopback_ipv4 "$ip" && ip=""
	fi
	if [[ -z "$ip" && -n "$curl_bin" ]]; then
		ip="$("$curl_bin" -4 -sS --connect-timeout 3 --max-time 5 ifconfig.me 2>/dev/null | tr -d '[:space:]')"
		if [[ ! "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
			ip=""
		fi
	fi
	echo "$ip"
}

# Host clients should use in URLs (IPv4 preferred, else hostname -f).
production_resolve_access_host() {
	local ip host hostname_bin
	ip="$(production_detect_primary_ip)"
	if [[ -n "$ip" ]]; then
		echo "$ip"
		return 0
	fi
	hostname_bin="$(production_resolve_cmd hostname 2>/dev/null || true)"
	if [[ -n "$hostname_bin" ]]; then
		host="$("$hostname_bin" -f 2>/dev/null || true)"
		if [[ -n "$host" && "$host" != "localhost" ]]; then
			echo "$host"
			return 0
		fi
		host="$("$hostname_bin" 2>/dev/null || true)"
		if [[ -n "$host" ]]; then
			echo "$host"
			return 0
		fi
	fi
	return 1
}

production_http_url() {
	local port="$1"
	local host
	if host="$(production_resolve_access_host)"; then
		echo "http://${host}:${port}"
	else
		echo "http://127.0.0.1:${port}"
	fi
}

production_read_env_value() {
	local file="$1" key="$2"
	local grep_bin tail_bin
	if [[ ! -f "$file" ]]; then
		return 1
	fi
	grep_bin="$(production_resolve_cmd grep)" || return 1
	tail_bin="$(production_resolve_cmd tail)" || return 1
	local line
	line="$("$grep_bin" -E "^${key}=" "$file" | "$tail_bin" -1 || true)"
	[[ -n "$line" ]] || return 1
	echo "${line#*=}"
}

# Create a system user for controller/dashboard (idempotent).
production_create_system_user() {
	local user="${1:-$DASHBOARD_USER}"
	local home="${2:-$STATE_DIR}"
	local adduser_bin useradd_bin id_bin

	id_bin="$(production_resolve_cmd id)" || {
		echo "[-] Error: id not found" >&2
		return 1
	}
	if "$id_bin" "$user" &>/dev/null; then
		return 0
	fi

	echo "[*] Creating system user: $user"
	adduser_bin="$(production_find_adduser 2>/dev/null || true)"
	useradd_bin="$(production_find_useradd 2>/dev/null || true)"

	if [[ -n "$adduser_bin" ]]; then
		"$adduser_bin" --system --group --home "$home" --no-create-home --disabled-login "$user"
	elif [[ -n "$useradd_bin" ]]; then
		"$useradd_bin" --system --no-create-home --shell /usr/sbin/nologin \
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
	local install_bin
	install_bin="$(production_resolve_cmd install)" || {
		echo "[-] Error: install not found" >&2
		return 1
	}
	"$install_bin" -d -o "$DASHBOARD_USER" -g "$DASHBOARD_USER" -m 750 "$STATE_DIR"
}

production_ensure_controller_env() {
	echo "[*] Preparing controller secrets..."
	local install_bin chmod_bin chown_bin
	install_bin="$(production_resolve_cmd install)" || {
		echo "[-] Error: install not found" >&2
		return 1
	}
	chmod_bin="$(production_resolve_cmd chmod)" || {
		echo "[-] Error: chmod not found" >&2
		return 1
	}
	chown_bin="$(production_resolve_cmd chown 2>/dev/null || true)"

	"$install_bin" -d -m 750 "$TRINITY_DIR"

	local api_key agent_key controller_url
	api_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_API_KEY || true)"
	agent_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_AGENT_KEY || true)"
	controller_url="$(production_read_env_value "$CONTROLLER_ENV" CONTROLLER_URL || true)"

	[[ -n "$api_key" ]] || api_key="$(production_random_hex)"
	[[ -n "$agent_key" ]] || agent_key="$(production_random_hex)"

	if [[ -z "$controller_url" ]]; then
		controller_url="$(production_http_url "$API_PORT")"
	fi

	cat >"$CONTROLLER_ENV" <<EOF
TRINITY_API_KEY=${api_key}
TRINITY_AGENT_KEY=${agent_key}
API_PORT=${API_PORT}
DB_PATH=${DB_PATH}
CONTROLLER_URL=${controller_url}
EOF
	"$chmod_bin" 640 "$CONTROLLER_ENV"
	if [[ -n "$chown_bin" ]]; then
		"$chown_bin" root:"$DASHBOARD_USER" "$CONTROLLER_ENV" 2>/dev/null || "$chmod_bin" 640 "$CONTROLLER_ENV"
	else
		"$chmod_bin" 640 "$CONTROLLER_ENV"
	fi
	echo "[+] Wrote $CONTROLLER_ENV"
}

production_sync_agent_key_to_controller_env() {
	local db="$STATE_DIR/dashboard.db"
	local sqlite3_bin
	sqlite3_bin="$(production_resolve_cmd sqlite3 2>/dev/null || true)"
	if [[ ! -f "$db" ]] || [[ -z "$sqlite3_bin" ]]; then
		return 0
	fi
	local key
	key="$("$sqlite3_bin" "$db" "SELECT agent_key FROM dashboard_deployment WHERE id = 1;" 2>/dev/null || true)"
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
	local api_key controller_url api_port db_path chmod_bin chown_bin
	chmod_bin="$(production_resolve_cmd chmod)" || return 1
	chown_bin="$(production_resolve_cmd chown 2>/dev/null || true)"
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
	"$chmod_bin" 640 "$CONTROLLER_ENV"
	if [[ -n "$chown_bin" ]]; then
		"$chown_bin" root:"$DASHBOARD_USER" "$CONTROLLER_ENV" 2>/dev/null || true
	fi
	echo "[+] Synced TRINITY_AGENT_KEY from dashboard DB to $CONTROLLER_ENV"
}

production_init_dashboard_admin() {
	echo "[*] Initializing dashboard admin + agent key..."
	production_ensure_trinityproxy_user
	production_ensure_state_dir

	local dashboard_url
	dashboard_url="$(production_http_url "$DASHBOARD_PORT")"

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
		local chown_bin chmod_bin
		chown_bin="$(production_resolve_cmd chown 2>/dev/null || true)"
		chmod_bin="$(production_resolve_cmd chmod 2>/dev/null || true)"
		if [[ -n "$chown_bin" ]]; then
			"$chown_bin" "$DASHBOARD_USER:$DASHBOARD_USER" "$STATE_DIR/dashboard.db"
		fi
		if [[ -n "$chmod_bin" ]]; then
			"$chmod_bin" 640 "$STATE_DIR/dashboard.db"
		fi
	fi
}

production_caddy_active() {
	local caddy_bin
	caddy_bin="$(production_resolve_cmd caddy 2>/dev/null || true)"
	[[ -f /etc/caddy/trinityproxy.caddy ]] && [[ -n "$caddy_bin" ]]
}

production_print_dashboard_login_banner() {
	local sep="============================================================"
	echo ""
	echo "$sep"
	echo "  DASHBOARD LOGIN — save these credentials"
	echo "$sep"
	if [[ "${PRODUCTION_DASHBOARD_ADMIN_CREATED:-}" == "1" ]]; then
		local dash_url="${PRODUCTION_DASHBOARD_URL:-}"
		if [[ -z "$dash_url" || "$dash_url" == *"<"* ]]; then
			dash_url="$(production_http_url "$DASHBOARD_PORT")"
		fi
		echo ""
		echo "  Dashboard URL:  ${dash_url}"
		echo "  Username:       ${PRODUCTION_DASHBOARD_USERNAME}"
		echo "  Password:       ${PRODUCTION_DASHBOARD_PASSWORD}"
		echo ""
		echo "  First login requires a password change."
	else
		echo ""
		echo "  Dashboard URL:  $(production_http_url "$DASHBOARD_PORT")"
		echo "  Dashboard admin already exists — no new password was generated."
		echo "  Use your existing password or reset the dashboard database."
	fi
	echo "$sep"
}


production_print_summary() {
	local dash_url api_url ip_detected
	dash_url="$(production_http_url "$DASHBOARD_PORT")"
	api_url="$(production_http_url "$API_PORT")"
	ip_detected="$(production_detect_primary_ip)"
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
		echo "  (Direct HTTP:  ${dash_url})"
	else
		echo "Dashboard URL: ${dash_url}"
		echo "Controller API:  ${api_url}"
		if [[ -z "$ip_detected" ]]; then
			echo "  (No IPv4 auto-detected — if the URL is wrong, run: hostname -I)"
		fi
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
