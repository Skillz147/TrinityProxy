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
	production_ensure_trinityproxy_user
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

OPT_SCRIPTS_DIR="${OPT_SCRIPTS_DIR:-$OPT_PREFIX/scripts}"
SSL_SETUP_SCRIPT="${SSL_SETUP_SCRIPT:-$OPT_SCRIPTS_DIR/setup-ssl-caddy-cloudflare.sh}"

production_install_scripts() {
	local project_root="$1"
	local name src
	production_install -d -o root -g root -m 755 "$OPT_SCRIPTS_DIR"
	local lib_src="$project_root/scripts/lib/production-common.sh"
	if [[ ! -f "$lib_src" ]]; then
		echo "[-] Missing script: $lib_src" >&2
		return 1
	fi
	production_install -d -o root -g root -m 755 "$OPT_SCRIPTS_DIR/lib"
	echo "[*] Installing production-common.sh -> $OPT_SCRIPTS_DIR/lib/production-common.sh"
	production_install -o root -g root -m 644 "$lib_src" "$OPT_SCRIPTS_DIR/lib/production-common.sh"
	for name in setup-domain.sh setup-ssl-caddy-cloudflare.sh setup-ssl-caddy.sh; do
		src="$project_root/scripts/$name"
		if [[ ! -f "$src" ]]; then
			echo "[-] Missing script: $src" >&2
			return 1
		fi
		echo "[*] Installing $name -> $OPT_SCRIPTS_DIR/$name"
		production_install -o root -g root -m 755 "$src" "$OPT_SCRIPTS_DIR/$name"
	done
	production_install_ssl_provision "$project_root"
}

production_install_ssl_provision() {
	local project_root="$1"
	local obsolete_sudoers="/etc/sudoers.d/trinityproxy-ssl"
	if [[ -f "$obsolete_sudoers" ]]; then
		echo "[*] Removing obsolete sudoers drop-in (incompatible with NoNewPrivileges): $obsolete_sudoers"
		rm -f "$obsolete_sudoers"
	fi

	local tmpfiles_src="$project_root/scripts/trinityproxy-ssl-provision.tmpfiles.conf"
	if [[ -f "$tmpfiles_src" ]]; then
		echo "[*] Installing tmpfiles for /run/trinityproxy"
		production_install -o root -g root -m 644 "$tmpfiles_src" /etc/tmpfiles.d/trinityproxy-ssl-provision.conf
		if command -v systemd-tmpfiles >/dev/null 2>&1; then
			systemd-tmpfiles --create /etc/tmpfiles.d/trinityproxy-ssl-provision.conf 2>/dev/null || true
		fi
	fi

	local unit_src="$project_root/scripts/trinityproxy-ssl-provision.service"
	if [[ ! -f "$unit_src" ]]; then
		echo "[-] Missing unit: $unit_src" >&2
		return 1
	fi
	echo "[*] Installing systemd oneshot: trinityproxy-ssl-provision.service"
	production_install_systemd_unit "$unit_src" 		/etc/systemd/system/trinityproxy-ssl-provision.service "$project_root"

	local polkit_src="$project_root/scripts/trinityproxy-ssl-provision.polkit.rules"
	if [[ -f "$polkit_src" ]]; then
		echo "[*] Installing polkit rule for dashboard SSL provision (systemctl, no sudo)"
		production_install -o root -g root -m 644 "$polkit_src" 			/etc/polkit-1/rules.d/50-trinityproxy-ssl-provision.rules
	fi
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
			-e "s|ReadOnlyPaths=/opt/trinityproxy/scripts|ReadOnlyPaths=$project_root/scripts|g" \
			-e "s|TRINITY_SCRIPTS_DIR=/opt/trinityproxy/scripts|TRINITY_SCRIPTS_DIR=$project_root/scripts|g" \
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

production_journalctl() {
	local bin
	bin="$(production_resolve_cmd journalctl 2>/dev/null || true)"
	if [[ -z "$bin" ]]; then
		echo "[-] journalctl not found" >&2
		return 1
	fi
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

# Stop/start systemd units with a wall-clock timeout (avoids hung reset/install scripts).
production_systemctl_stop_unit() {
	local unit="$1"
	local wait_secs="${2:-45}"
	local bin main_pid i
	bin="$(production_resolve_cmd systemctl)" || return 1

	if ! "$bin" is-active --quiet "$unit" 2>/dev/null; then
		return 0
	fi

	echo "[*] Stopping $unit (timeout ${wait_secs}s)..."
	if command -v timeout >/dev/null 2>&1; then
		timeout "$wait_secs" "$bin" stop "$unit" 2>/dev/null || true
	else
		"$bin" stop --no-block "$unit" 2>/dev/null || true
		i=0
		while [[ $i -lt $wait_secs ]] && "$bin" is-active --quiet "$unit" 2>/dev/null; do
			sleep 1
			i=$((i + 1))
		done
	fi

	if ! "$bin" is-active --quiet "$unit" 2>/dev/null; then
		echo "[+] $unit stopped."
		return 0
	fi

	echo "[!] $unit still active after ${wait_secs}s; forcing stop..."
	main_pid="$("$bin" show -p MainPID --value "$unit" 2>/dev/null || echo 0)"
	if [[ "$main_pid" =~ ^[0-9]+$ ]] && [[ "$main_pid" -gt 0 ]]; then
		kill -TERM "$main_pid" 2>/dev/null || true
		sleep 2
		if "$bin" is-active --quiet "$unit" 2>/dev/null; then
			kill -KILL "$main_pid" 2>/dev/null || true
		fi
	fi
	"$bin" kill -s SIGKILL "$unit" 2>/dev/null || true
	"$bin" reset-failed "$unit" 2>/dev/null || true
	if command -v timeout >/dev/null 2>&1; then
		timeout 15 "$bin" stop "$unit" 2>/dev/null || true
	else
		"$bin" stop "$unit" 2>/dev/null || true
	fi

	if "$bin" is-active --quiet "$unit" 2>/dev/null; then
		echo "[-] Error: failed to stop $unit" >&2
		return 1
	fi
	echo "[+] $unit stopped (forced)."
}

production_systemctl_start_unit() {
	local unit="$1"
	local wait_secs="${2:-60}"
	local bin i
	bin="$(production_resolve_cmd systemctl)" || return 1

	if "$bin" is-active --quiet "$unit" 2>/dev/null; then
		return 0
	fi

	echo "[*] Starting $unit (timeout ${wait_secs}s)..."
	if command -v timeout >/dev/null 2>&1; then
		if timeout "$wait_secs" "$bin" start "$unit"; then
			echo "[+] $unit started."
			return 0
		fi
	else
		if "$bin" start "$unit"; then
			echo "[+] $unit started."
			return 0
		fi
	fi

	echo "[-] Warning: systemctl start $unit did not complete within ${wait_secs}s (check: systemctl status $unit)" >&2
	return 1
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

production_find_deluser() {
	local p
	for p in /usr/sbin/deluser /sbin/deluser; do
		if [[ -x "$p" ]]; then
			echo "$p"
			return 0
		fi
	done
	production_resolve_cmd deluser 2>/dev/null || return 1
}

production_find_userdel() {
	local p
	for p in /usr/sbin/userdel /sbin/userdel; do
		if [[ -x "$p" ]]; then
			echo "$p"
			return 0
		fi
	done
	production_resolve_cmd userdel 2>/dev/null || return 1
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

production_is_ipv4() {
	local ip="$1"
	[[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

production_is_loopback_ipv4() {
	local ip="$1"
	[[ "$ip" == 127.* ]]
}

# RFC1918 and link-local (169.254.x.x).
production_is_private_ip() {
	local ip="$1"
	[[ "$ip" =~ ^10\. ]] && return 0
	[[ "$ip" =~ ^192\.168\. ]] && return 0
	[[ "$ip" =~ ^169\.254\. ]] && return 0
	if [[ "$ip" =~ ^172\.([0-9]+)\. ]]; then
		local second="${BASH_REMATCH[1]}"
		if (( second >= 16 && second <= 31 )); then
			return 0
		fi
	fi
	return 1
}

production_trim_ip() {
	local ip="$1"
	ip="${ip//$'\r'/}"
	ip="${ip//$'\n'/}"
	ip="${ip// /}"
	echo "$ip"
}

production_curl_ip() {
	local curl_bin="$1"
	local url="$2"
	local extra_args=("${@:3}")
	local ip raw
	if [[ -z "$curl_bin" ]]; then
		return 1
	fi
	raw="$("$curl_bin" -4 -sf --connect-timeout 3 --max-time 5 "${extra_args[@]}" "$url" 2>/dev/null || true)"
	ip="$(production_trim_ip "$raw")"
	if production_is_ipv4 "$ip"; then
		echo "$ip"
		return 0
	fi
	return 1
}

production_detect_gcp_external_ip() {
	local curl_bin="$1"
	production_curl_ip "$curl_bin" \
		"http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip" \
		-H "Metadata-Flavor: Google" --connect-timeout 2
}

production_detect_aws_external_ip() {
	local curl_bin="$1"
	production_curl_ip "$curl_bin" \
		"http://169.254.169.254/latest/meta-data/public-ipv4" --connect-timeout 2
}

production_detect_public_ip_via_services() {
	local curl_bin="$1"
	local url ip
	if [[ -z "$curl_bin" ]]; then
		return 1
	fi
	for url in https://ifconfig.me https://api.ipify.org https://icanhazip.com; do
		if ip="$(production_curl_ip "$curl_bin" "$url")"; then
			if ! production_is_private_ip "$ip" && ! production_is_loopback_ipv4 "$ip"; then
				echo "$ip"
				return 0
			fi
		fi
	done
	return 1
}

production_collect_hostname_ips() {
	local hostname_bin="$1"
	local addr
	if [[ -z "$hostname_bin" ]]; then
		return 0
	fi
	for addr in $($hostname_bin -I 2>/dev/null); do
		addr="$(production_trim_ip "$addr")"
		[[ -z "$addr" ]] && continue
		production_is_ipv4 "$addr" || continue
		echo "$addr"
	done
}

production_collect_ip_addr_ips() {
	local ip_bin="$1"
	local awk_bin="$2"
	local line addr
	if [[ -z "$ip_bin" || -z "$awk_bin" ]]; then
		return 0
	fi
	while IFS= read -r line; do
		addr="$(production_trim_ip "$line")"
		[[ -z "$addr" ]] && continue
		production_is_ipv4 "$addr" || continue
		echo "$addr"
	done < <("$ip_bin" -4 addr 2>/dev/null | "$awk_bin" '/inet / {print $2}' | "$awk_bin" -F/ '{print $1}')
}

production_detect_local_public_candidate_ip() {
	local hostname_bin="$1" ip_bin="$2" awk_bin="$3"
	local addr seen=""
	hostname_bin="${hostname_bin:-}"
	ip_bin="${ip_bin:-}"
	awk_bin="${awk_bin:-}"
	for addr in $(production_collect_hostname_ips "$hostname_bin") $(production_collect_ip_addr_ips "$ip_bin" "$awk_bin"); do
		production_is_loopback_ipv4 "$addr" && continue
		production_is_private_ip "$addr" && continue
		[[ "$seen" == *" $addr "* ]] && continue
		seen=" $seen $addr "
		echo "$addr"
		return 0
	done
	return 1
}

production_detect_first_non_loopback_ipv4() {
	local hostname_bin="$1" ip_bin="$2" awk_bin="$3" curl_bin="$4"
	local addr ip
	for addr in $(production_collect_hostname_ips "$hostname_bin") $(production_collect_ip_addr_ips "$ip_bin" "$awk_bin"); do
		production_is_loopback_ipv4 "$addr" && continue
		production_is_ipv4 "$addr" || continue
		echo "$addr"
		return 0
	done
	if [[ -n "$ip_bin" && -n "$awk_bin" ]]; then
		ip="$("$ip_bin" -4 route get 1.1.1.1 2>/dev/null | "$awk_bin" '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}')"
		ip="$(production_trim_ip "$ip")"
		if production_is_ipv4 "$ip" && ! production_is_loopback_ipv4 "$ip"; then
			echo "$ip"
			return 0
		fi
	fi
	return 1
}

# External-facing IP for URLs (metadata, public services, then local non-private).
production_detect_public_ip() {
	local curl_bin hostname_bin ip_bin awk_bin ip
	curl_bin="$(production_resolve_cmd curl 2>/dev/null || true)"
	hostname_bin="$(production_resolve_cmd hostname 2>/dev/null || true)"
	ip_bin="$(production_resolve_cmd ip 2>/dev/null || true)"
	awk_bin="$(production_resolve_cmd awk 2>/dev/null || true)"

	if [[ -n "$curl_bin" ]]; then
		if ip="$(production_detect_gcp_external_ip "$curl_bin")"; then
			echo "$ip"
			return 0
		fi
		if ip="$(production_detect_aws_external_ip "$curl_bin")"; then
			echo "$ip"
			return 0
		fi
		if ip="$(production_detect_public_ip_via_services "$curl_bin")"; then
			echo "$ip"
			return 0
		fi
	fi
	if ip="$(production_detect_local_public_candidate_ip "$hostname_bin" "$ip_bin" "$awk_bin")"; then
		echo "$ip"
		return 0
	fi
	return 1
}

# Prefer public IP; fall back to private/local when nothing else is available.
production_detect_primary_ip() {
	local ip hostname_bin ip_bin awk_bin curl_bin
	hostname_bin="$(production_resolve_cmd hostname 2>/dev/null || true)"
	ip_bin="$(production_resolve_cmd ip 2>/dev/null || true)"
	awk_bin="$(production_resolve_cmd awk 2>/dev/null || true)"
	curl_bin="$(production_resolve_cmd curl 2>/dev/null || true)"

	if ip="$(production_detect_public_ip)"; then
		echo "$ip"
		return 0
	fi
	if ip="$(production_detect_first_non_loopback_ipv4 "$hostname_bin" "$ip_bin" "$awk_bin" "$curl_bin")"; then
		echo "$ip"
		return 0
	fi
	return 1
}

production_warn_private_access_ip_once() {
	local ip="$1"
	if [[ -z "$ip" ]] || ! production_is_private_ip "$ip"; then
		return 0
	fi
	if [[ "${PRODUCTION_PRIVATE_IP_WARNED:-}" == "1" ]]; then
		return 0
	fi
	PRODUCTION_PRIVATE_IP_WARNED=1
	echo "[!] Could not detect public IP — using ${ip}. Set CONTROLLER_URL manually or use GCP external IP." >&2
}

# Host clients should use in URLs (public IP when possible, else hostname -f).
production_resolve_access_host() {
	local ip host hostname_bin
	ip="$(production_detect_primary_ip)"
	if [[ -n "$ip" ]]; then
		production_warn_private_access_ip_once "$ip"
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

production_ensure_bootstrap_prereqs() {
	production_ensure_trinityproxy_user
	production_ensure_state_dir
}


# Remove system user created for production (idempotent).
production_remove_system_user() {
	local user="${1:-$DASHBOARD_USER}"
	local id_bin deluser_bin userdel_bin
	id_bin="$(production_resolve_cmd id 2>/dev/null || true)"
	[[ -n "$id_bin" ]] || return 1
	if ! "$id_bin" "$user" &>/dev/null; then
		return 0
	fi
	echo "[*] Removing system user: $user"
	deluser_bin="$(production_find_deluser 2>/dev/null || true)"
	userdel_bin="$(production_find_userdel 2>/dev/null || true)"
	if [[ -n "$deluser_bin" ]]; then
		if "$deluser_bin" --remove-home "$user" 2>/dev/null; then
			return 0
		fi
		"$deluser_bin" "$user" 2>/dev/null && return 0
	elif [[ -n "$userdel_bin" ]]; then
		if "$userdel_bin" -r "$user" 2>/dev/null; then
			return 0
		fi
		"$userdel_bin" "$user" 2>/dev/null && return 0
	fi
	echo "[-] Error: could not remove user $user (deluser/userdel failed)" >&2
	return 1
}


production_ensure_state_dir() {
	production_ensure_trinityproxy_user
	local install_bin
	install_bin="$(production_resolve_cmd install)" || {
		echo "[-] Error: install not found" >&2
		return 1
	}
	"$install_bin" -d -o "$DASHBOARD_USER" -g "$DASHBOARD_USER" -m 750 "$STATE_DIR"
}

production_fixup_state_dir() {
	local chown_bin chmod_bin
	production_ensure_trinityproxy_user
	production_ensure_state_dir
	chown_bin="$(production_resolve_cmd chown 2>/dev/null || true)"
	chmod_bin="$(production_resolve_cmd chmod 2>/dev/null || true)"
	if [[ -n "$chown_bin" ]]; then
		"$chown_bin" -R "$DASHBOARD_USER:$DASHBOARD_USER" "$STATE_DIR" 2>/dev/null || true
	fi
	if [[ -n "$chmod_bin" ]]; then
		"$chmod_bin" 750 "$STATE_DIR" 2>/dev/null || true
		find "$STATE_DIR" -maxdepth 1 -type f -name '*.db' -exec "$chmod_bin" 640 {} + 2>/dev/null || true
	fi
}


production_ensure_controller_env() {
	production_ensure_trinityproxy_user
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

	"$install_bin" -d -o root -g "$DASHBOARD_USER" -m 750 "$TRINITY_DIR"

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

production_update_controller_env_domain() {
	local domain="${1:-}"
	local controller_url="${2:-}"
	domain="${domain// /}"
	if [[ -z "$domain" ]]; then
		return 0
	fi
	if [[ ! -f "$CONTROLLER_ENV" ]]; then
		production_ensure_controller_env || return 1
	fi
	local api_key agent_key api_port db_path public_domain current_url chmod_bin chown_bin
	chmod_bin="$(production_resolve_cmd chmod 2>/dev/null || true)"
	chown_bin="$(production_resolve_cmd chown 2>/dev/null || true)"
	api_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_API_KEY)"
	agent_key="$(production_read_env_value "$CONTROLLER_ENV" TRINITY_AGENT_KEY)"
	api_port="$(production_read_env_value "$CONTROLLER_ENV" API_PORT || echo "$API_PORT")"
	db_path="$(production_read_env_value "$CONTROLLER_ENV" DB_PATH || echo "$DB_PATH")"
	public_domain="$(production_read_env_value "$CONTROLLER_ENV" PUBLIC_DOMAIN)"
	current_url="$(production_read_env_value "$CONTROLLER_ENV" CONTROLLER_URL)"
	[[ -n "$public_domain" ]] || public_domain="$domain"
	if [[ -n "$controller_url" ]]; then
		current_url="$controller_url"
	elif [[ -z "$current_url" ]]; then
		current_url="https://api.${domain}"
	fi
	cat >"$CONTROLLER_ENV" <<EOF
TRINITY_API_KEY=${api_key}
TRINITY_AGENT_KEY=${agent_key}
API_PORT=${api_port}
DB_PATH=${db_path}
PUBLIC_DOMAIN=${public_domain}
CONTROLLER_URL=${current_url}
EOF
	[[ -n "$chmod_bin" ]] && "$chmod_bin" 640 "$CONTROLLER_ENV"
	if [[ -n "$chown_bin" ]]; then
		"$chown_bin" root:"$DASHBOARD_USER" "$CONTROLLER_ENV" 2>/dev/null || true
	fi
	echo "[+] Updated $CONTROLLER_ENV (PUBLIC_DOMAIN=${public_domain}, CONTROLLER_URL=${current_url})"
}

production_sync_deployment_settings() {
	local domain="${1:-${PUBLIC_DOMAIN:-}}"
	local ssl_mode="${2:-}"
	local controller_url="${3:-}"
	local script="${OPT_SCRIPTS_DIR:-/opt/trinityproxy/scripts}/sync-deployment-settings.sh"
	if [[ -n "${ROOT:-}" && -f "$ROOT/scripts/sync-deployment-settings.sh" ]]; then
		script="$ROOT/scripts/sync-deployment-settings.sh"
	fi
	if [[ ! -f "$script" ]]; then
		echo "[!] sync-deployment-settings.sh not found — dashboard domain not updated" >&2
		return 1
	fi
	if [[ -n "$domain" ]]; then
		export TRINITY_SYNC_PUBLIC_DOMAIN="$domain"
		export TRINITY_SYNC_FORCE=1
	fi
	if [[ -n "$ssl_mode" ]]; then
		export TRINITY_SYNC_SSL_MODE="$ssl_mode"
	fi
	if [[ -n "$controller_url" ]]; then
		export TRINITY_SYNC_CONTROLLER_URL="$controller_url"
	fi
	export DASHBOARD_DB_PATH="${DASHBOARD_DB_PATH:-$STATE_DIR/dashboard.db}"
	bash "$script"
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
	local public_domain
	public_domain="$(production_read_env_value "$CONTROLLER_ENV" PUBLIC_DOMAIN || true)"
	cat >"$CONTROLLER_ENV" <<EOF
TRINITY_API_KEY=${api_key}
TRINITY_AGENT_KEY=${key}
API_PORT=${api_port}
DB_PATH=${db_path}
PUBLIC_DOMAIN=${public_domain}
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

production_caddy_config_present() {
	[[ -f /etc/caddy/trinityproxy.caddy ]]
}

production_caddy_active() {
	local caddy_bin
	caddy_bin="$(production_resolve_cmd caddy 2>/dev/null || true)"
	[[ -n "$caddy_bin" ]] || return 1
	production_caddy_config_present || return 1
	production_systemctl is-active --quiet caddy 2>/dev/null
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
		echo "  Locked out? Generate new login: sudo make reset-dashboard-admin"
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
	elif production_caddy_config_present; then
		echo "Dashboard URL: ${dash_url}"
		echo "Controller API:  ${api_url}"
		if [[ -z "$ip_detected" ]]; then
			echo "  (No IPv4 auto-detected — if the URL is wrong, run: hostname -I)"
		fi
		echo ""
		echo "Caddy config present but service not running — HTTPS is not active."
		echo "  Fix permissions: sudo chown root:caddy /etc/caddy/trinityproxy.caddy"
		echo "    sudo chmod 640 /etc/caddy/trinityproxy.caddy"
		if [[ -f /etc/caddy/cloudflare.env ]]; then
			echo "    sudo chown root:caddy /etc/caddy/cloudflare.env && sudo chmod 640 /etc/caddy/cloudflare.env"
		fi
		echo "  Then: sudo systemctl restart caddy && systemctl is-active caddy"
	else
		echo "Dashboard URL: ${dash_url}"
		echo "Controller API:  ${api_url}"
		if [[ -z "$ip_detected" ]]; then
			echo "  (No IPv4 auto-detected — if the URL is wrong, run: hostname -I)"
		fi
		echo ""
		if [[ "${TRINITY_DOMAIN_SETUP_SKIPPED:-}" == "1" ]]; then
			echo "HTTPS: skipped during make start — dashboard works on ${dash_url}"
			echo "  Re-run anytime: sudo make setup-domain"
		else
			echo "HTTPS (domain + Cloudflare wildcard SSL):"
			echo "  sudo make setup-domain  (or during next: sudo make start)"
		fi
	fi
	production_print_dashboard_login_banner
	echo ""
	echo "Controller secrets file: $CONTROLLER_ENV"
	echo "  (TRINITY_API_KEY, TRINITY_AGENT_KEY, API_PORT, DB_PATH, CONTROLLER_URL)"
	echo "  CONTROLLER_URL defaults to the detected access IP (public when available)."
	echo "  Agents on the same VPC/LAN may set CONTROLLER_URL to an internal IP instead."
	if [[ -n "$ip_detected" ]] && production_is_private_ip "$ip_detected"; then
		echo "  Using private IP ${ip_detected} in URLs — edit CONTROLLER_URL for internet-facing agents."
	fi
	echo ""
	if ! production_caddy_active; then
		production_print_cloud_firewall_instructions
	fi
	echo ""
	echo "Service commands:"
	echo "  sudo systemctl status trinityproxy-controller trinityproxy-dashboard"
	echo "  sudo journalctl -u trinityproxy-controller -f"
	echo "  sudo journalctl -u trinityproxy-dashboard -f"
}

# --- Cloud / host firewall helpers (VPS dashboard reachability) ---

production_print_cloud_firewall_instructions() {
	local ip
	ip="$(production_detect_primary_ip 2>/dev/null || true)"
	echo ""
	echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
	echo "  [!] OPEN CLOUD FIREWALL (required for browser access)"
	echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
	echo ""
	echo "  GCP: VPC network → Firewall → allow tcp:${DASHBOARD_PORT}, tcp:${API_PORT} to this VM"
	echo "  Or:"
	echo "    gcloud compute firewall-rules create trinityproxy \\"
	echo "      --allow tcp:${DASHBOARD_PORT},tcp:${API_PORT} \\"
	echo "      --direction=INGRESS --source-ranges=0.0.0.0/0"
	echo ""
	echo "  AWS: Security group inbound — TCP ${DASHBOARD_PORT} (dashboard), ${API_PORT} (controller API)"
	if [[ -n "$ip" ]]; then
		echo ""
		echo "  Then open: $(production_http_url "$DASHBOARD_PORT")"
	fi
	echo ""
}

production_configure_ufw_if_active() {
	local ufw_bin
	ufw_bin="$(production_resolve_cmd ufw 2>/dev/null || true)"
	[[ -n "$ufw_bin" ]] || return 0
	if ! "$ufw_bin" status 2>/dev/null | grep -qiE '^Status:[[:space:]]*active'; then
		echo "[+] ufw installed but not active (skipping port rules)"
		return 0
	fi
	echo "[*] ufw is active — allowing TCP ${DASHBOARD_PORT} (dashboard) and ${API_PORT} (controller)..."
	if [[ "$(id -u)" -eq 0 ]]; then
		"$ufw_bin" allow "${DASHBOARD_PORT}/tcp" >/dev/null 2>&1 || "$ufw_bin" allow "${DASHBOARD_PORT}/tcp"
		"$ufw_bin" allow "${API_PORT}/tcp" >/dev/null 2>&1 || "$ufw_bin" allow "${API_PORT}/tcp"
	else
		echo "[!] Run as root to apply ufw rules: sudo ufw allow ${DASHBOARD_PORT}/tcp && sudo ufw allow ${API_PORT}/tcp"
	fi
}

production_curl_local_dashboard_ok() {
	local curl_bin port
	curl_bin="$(production_resolve_cmd curl 2>/dev/null || true)"
	port="${DASHBOARD_PORT:-8081}"
	[[ -n "$curl_bin" ]] || return 1
	if "$curl_bin" -sf --connect-timeout 3 "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
		return 0
	fi
	if "$curl_bin" -sf --connect-timeout 3 "http://127.0.0.1:${port}/" >/dev/null 2>&1; then
		return 0
	fi
	return 1
}

production_curl_public_dashboard_ok() {
	local curl_bin port host
	curl_bin="$(production_resolve_cmd curl 2>/dev/null || true)"
	port="${DASHBOARD_PORT:-8081}"
	host="$(production_detect_primary_ip 2>/dev/null || true)"
	[[ -n "$curl_bin" && -n "$host" ]] || return 1
	if production_is_private_ip "$host"; then
		return 1
	fi
	if "$curl_bin" -sf --connect-timeout 3 "http://${host}:${port}/health" >/dev/null 2>&1; then
		return 0
	fi
	if "$curl_bin" -sf --connect-timeout 3 "http://${host}:${port}/" >/dev/null 2>&1; then
		return 0
	fi
	return 1
}

# Returns 0 if local dashboard responds; prints guidance when cloud firewall may block public access.
production_verify_dashboard_reachable() {
	local port="${DASHBOARD_PORT:-8081}"
	if production_curl_local_dashboard_ok; then
		echo "[+] Dashboard responds at http://127.0.0.1:${port}/"
		if production_curl_public_dashboard_ok; then
			echo "[+] Dashboard reachable via detected public IP on :${port}"
		else
			echo ""
			echo "[!] Local dashboard OK, but public URL may be blocked by your cloud firewall."
			production_print_cloud_firewall_instructions
		fi
		return 0
	fi
	echo "[!] Dashboard not responding on 127.0.0.1:${port}"
	echo "    Check: sudo systemctl status trinityproxy-dashboard"
	echo "    Logs:  sudo journalctl -u trinityproxy-dashboard -n 40 --no-pager"
	return 1
}
