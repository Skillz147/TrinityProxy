#!/usr/bin/env bash
# Safely add or remove TrinityProxy dev entries in /etc/hosts (idempotent).
#
# Usage:
#   sudo ./scripts/dev-hosts-helper.sh add [domain]
#   sudo ./scripts/dev-hosts-helper.sh remove [domain]
#   ./scripts/dev-hosts-helper.sh status [domain]
#
# Default domain: trinityproxy.local (also manages api.<domain>)

set -euo pipefail

ACTION="${1:-}"
DOMAIN="${2:-${DOMAIN:-trinityproxy.local}}"
API_DOMAIN="api.${DOMAIN}"
HOSTS_FILE="/etc/hosts"
MARKER="# TrinityProxy dev"
HOSTS_LINE="127.0.0.1 ${DOMAIN} ${API_DOMAIN} ${MARKER}"

green() { echo -e "\033[32m$1\033[0m"; }
yellow() { echo -e "\033[33m$1\033[0m"; }
red() { echo -e "\033[31m$1\033[0m"; }

usage() {
    cat <<EOF
Usage: $0 <add|remove|status> [domain]

Manage /etc/hosts entries for local TrinityProxy development.

Commands:
  add      Append 127.0.0.1 <domain> api.<domain> (idempotent)
  remove   Delete TrinityProxy dev lines for the domain (idempotent)
  status   Show whether the entry is present

Arguments:
  domain   Base local domain (default: trinityproxy.local)

Environment:
  DOMAIN   Same as positional domain argument

Examples:
  sudo $0 add
  sudo $0 add myproxy.test
  sudo $0 remove trinityproxy.local
  $0 status

Note: add/remove require write access to ${HOSTS_FILE} (typically sudo).
EOF
}

require_hosts_readable() {
    if [[ ! -r "${HOSTS_FILE}" ]]; then
        red "[-] Cannot read ${HOSTS_FILE}. Try: sudo $0 ${ACTION} ${DOMAIN}"
        exit 1
    fi
}

require_hosts_writable() {
    if [[ ! -w "${HOSTS_FILE}" ]]; then
        red "[-] Cannot write ${HOSTS_FILE}. Run with sudo:"
        red "    sudo $0 ${ACTION} ${DOMAIN}"
        exit 1
    fi
}

entry_present() {
    grep -E "^[[:space:]]*127\.0\.0\.1[[:space:]]+.*\\b${DOMAIN//./\\.}\\b" "${HOSTS_FILE}" \
        | grep -qE "\\b${API_DOMAIN//./\\.}\\b"
}

marker_present() {
    grep -Fq "${MARKER}" "${HOSTS_FILE}" 2>/dev/null \
        && grep -E "^[[:space:]]*127\.0\.0\.1[[:space:]]+.*\\b${DOMAIN//./\\.}\\b" "${HOSTS_FILE}" \
            | grep -qF "${MARKER}"
}

cmd_status() {
    require_hosts_readable
    echo "[*] Checking ${HOSTS_FILE} for ${DOMAIN} / ${API_DOMAIN}"
    if entry_present; then
        green "[+] Entry present:"
        grep -E "^[[:space:]]*127\.0\.0\.1[[:space:]]+.*\\b${DOMAIN//./\\.}\\b" "${HOSTS_FILE}" \
            | grep -E "\\b${API_DOMAIN//./\\.}\\b" || true
    else
        yellow "[!] Entry not found"
        echo "    Expected: ${HOSTS_LINE}"
    fi
}

cmd_add() {
    require_hosts_writable

    if entry_present; then
        green "[+] Hosts entry already present (no changes)"
        cmd_status
        return 0
    fi

    printf '%s\n' "${HOSTS_LINE}" >> "${HOSTS_FILE}"
    green "[+] Added hosts entry:"
    echo "    ${HOSTS_LINE}"
}

cmd_remove() {
    require_hosts_writable

    if ! entry_present && ! grep -Fq "${MARKER}" "${HOSTS_FILE}" 2>/dev/null; then
        green "[+] No TrinityProxy dev entry to remove (already clean)"
        return 0
    fi

    local tmp backup
    tmp="$(mktemp)"
    backup="${HOSTS_FILE}.trinityproxy.bak.$(date +%Y%m%d%H%M%S)"

    # Drop lines with our marker, or legacy lines matching domain + api subdomain.
    awk -v domain="${DOMAIN}" -v api="${API_DOMAIN}" -v marker="${MARKER}" '
        $0 ~ marker { next }
        $1 == "127.0.0.1" && $0 ~ ("(^|[[:space:]])" domain "([[:space:]]|$)") && $0 ~ ("(^|[[:space:]])" api "([[:space:]]|$)") { next }
        { print }
    ' "${HOSTS_FILE}" > "${tmp}"

    if cmp -s "${tmp}" "${HOSTS_FILE}"; then
        rm -f "${tmp}"
        green "[+] No matching entry found (already clean)"
        return 0
    fi

    cp "${HOSTS_FILE}" "${backup}"
    cp "${tmp}" "${HOSTS_FILE}"
    rm -f "${tmp}"

    green "[+] Removed TrinityProxy dev hosts entry for ${DOMAIN}"
    yellow "[*] Backup saved: ${backup}"
}

main() {
    case "${ACTION}" in
        add)
            cmd_add
            ;;
        remove|delete|rm)
            cmd_remove
            ;;
        status|check)
            cmd_status
            ;;
        -h|--help|help|"")
            usage
            exit 0
            ;;
        *)
            red "[-] Unknown action: ${ACTION}"
            usage >&2
            exit 1
            ;;
    esac
}

main "$@"
