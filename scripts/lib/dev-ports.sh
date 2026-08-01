#!/usr/bin/env bash
# Shared helpers for TrinityProxy local dev ports (:8080, :8081, :3100).

DEV_VITE_PORT="${VITE_PORT:-8080}"
DEV_DASHBOARD_PORT="${DASHBOARD_PORT:-8081}"
DEV_CONTROLLER_PORT="${CONTROLLER_PORT:-3100}"

kill_process_tree() {
	local pid=$1
	local sig=${2:-TERM}
	if ! kill -0 "$pid" 2>/dev/null; then
		return 0
	fi

	local child
	for child in $(pgrep -P "$pid" 2>/dev/null || true); do
		kill_process_tree "$child" "$sig"
	done
	kill "-$sig" "$pid" 2>/dev/null || true
}

stop_port_force() {
	local port=$1
	local label=$2
	local pids
	pids="$(lsof -ti:"$port" 2>/dev/null || true)"
	if [[ -z "$pids" ]]; then
		return 1
	fi

	echo "[*] Stopping $label on :$port..."
	for pid in $pids; do
		kill_process_tree "$pid" TERM
	done

	local i
	for i in $(seq 1 10); do
		if ! lsof -ti:"$port" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.2
	done

	pids="$(lsof -ti:"$port" 2>/dev/null || true)"
	if [[ -n "$pids" ]]; then
		for pid in $pids; do
			kill_process_tree "$pid" KILL
		done
	fi
	return 0
}

stop_dev_pid_file() {
	local file=$1
	local label=$2
	if [[ ! -f "$file" ]]; then
		return 1
	fi

	local pid
	pid="$(cat "$file")"
	if kill -0 "$pid" 2>/dev/null; then
		echo "[*] Stopping $label (PID $pid)..."
		kill_process_tree "$pid" TERM
		sleep 0.3
		if kill -0 "$pid" 2>/dev/null; then
			kill_process_tree "$pid" KILL
		fi
	fi
	rm -f "$file"
	return 0
}

dev_ports_in_use() {
	lsof -ti:"$DEV_VITE_PORT" >/dev/null 2>&1 \
		|| lsof -ti:"$DEV_DASHBOARD_PORT" >/dev/null 2>&1 \
		|| lsof -ti:"$DEV_CONTROLLER_PORT" >/dev/null 2>&1
}
