# TrinityProxy — Local Development Setup

This guide covers running the controller API, dashboard, and agents on your machine or against a remote VPS. Replace placeholder values (`<domain>`, `<vps-ip>`, etc.) with your own — nothing here assumes a fixed production hostname.

## Prerequisites

| Tool | Purpose |
|------|---------|
| Go 1.21+ | Build controller, agent, dashboard binaries |
| Node.js 18+ | Dashboard Vite UI (`web/dashboard`) |
| `make` | Build and run shortcuts |
| Docker Desktop (Mac) | Optional — run a Linux agent container (`make docker-agent`) |

```bash
make deps && make build    # Go dependencies + binaries (first time)
```

---

## Dashboard — one command

```bash
make start-dev
# Open http://localhost:8080
```

Press **Ctrl+C** or run `make stop` when you're done.

`make start-dev` automatically:

- Builds the dashboard binary if needed
- Installs npm dependencies on first run
- Creates your admin account (prints login once)
- Starts the Go API on `:8081` and Vite UI on `:8080`
- Syncs the agent key to `.env.controller` when one exists in the dashboard

### First time

1. Run `make start-dev`
2. Log in with the credentials printed in the terminal
3. Change your password when prompted
4. **Settings** → enter your domain → **Save**
5. **Deploy Agent** → copy the install command to your VPS

### Optional env vars

| Variable | Default | Description |
|----------|---------|-------------|
| `DASHBOARD_PORT` | `8081` | Go API bind port |
| `VITE_PORT` / `VITE_DEV_PORT` | `8080` | Vite UI port |
| `CONTROLLER_URL` | see `internal/config` | Controller the dashboard talks to |
| `DB_PATH` | `./trinityproxy.db` | Node registry SQLite path |
| `DASHBOARD_DB_PATH` | `./dashboard.db` | Dashboard auth SQLite path |

### Advanced — split terminals

If you prefer two terminals (or only need the API):

```bash
# Terminal 1
make run-dashboard

# Terminal 2
cd web/dashboard && npm run dev
```

Single-process mode (Go proxies Vite):

```bash
DASHBOARD_DEV_PROXY=1 make run-dashboard
```

Check both services: `make dashboard-up`

---

## Controller API — local

Run the SOCKS5 node registry API on port **3100**:

```bash
make run-controller
# or: make build && ./build/trinityproxy-api
```

If you saved deployment settings in the dashboard, `make start-dev` may have already written `.env.controller`. Load it before starting the controller:

```bash
set -a && source .env.controller && set +a
make run-controller
```

Verify:

```bash
curl http://127.0.0.1:3100/health
```

Set `CONTROLLER_URL` wherever clients or agents need to reach it:

```bash
export CONTROLLER_URL=http://127.0.0.1:3100
```

| Variable | Default | Description |
|----------|---------|-------------|
| `API_PORT` | `3100` | Controller listen port |
| `DB_PATH` | `./trinityproxy.db` | SQLite database path |
| `TRINITY_API_KEY` | — | Bearer token for `/api/*` |
| `TRINITY_AGENT_KEY` | — | Agent heartbeat auth |

---

## Option A — Local domain + mkcert (HTTPS)

Use a `.local` or `.test` domain resolved via `/etc/hosts`, with trusted TLS certs from [mkcert](https://github.com/FiloSottile/mkcert).

### 1. Generate certificates

```bash
./scripts/dev-mkcert-setup.sh                  # default: trinityproxy.local
./scripts/dev-mkcert-setup.sh myproxy.test     # custom base domain
```

Certs are written to `.dev/certs/` (do not commit). The script prints `CERT_FILE`, `KEY_FILE`, and an example Caddy snippet.

### 2. Add `/etc/hosts` entries

```bash
sudo ./scripts/dev-hosts-helper.sh add trinityproxy.local
# Check: ./scripts/dev-hosts-helper.sh status trinityproxy.local
# Remove: sudo ./scripts/dev-hosts-helper.sh remove trinityproxy.local
```

Expected line:

```
127.0.0.1 trinityproxy.local api.trinityproxy.local # TrinityProxy dev
```

### 3. Point services at the controller

**Without a local TLS terminator** (direct HTTP to the Go API):

```bash
export CONTROLLER_URL=http://api.trinityproxy.local:3100
make run-controller
```

**With Caddy terminating TLS on `:443`:**

```bash
export CONTROLLER_URL=https://api.trinityproxy.local
# Reverse-proxy :443 → 127.0.0.1:3100 using CERT_FILE / KEY_FILE from mkcert setup
```

Set `CONTROLLER_URL` before `make start-dev` so deploy/bootstrap scripts reference your local controller.

---

## Option B — Remote VPS (no local DNS)

When the controller runs on a VPS, use its IP or DNS name directly:

```bash
export CONTROLLER_URL=http://<vps-ip>:3100
make start-dev    # dashboard on your laptop, controller on VPS
```

Examples:

```bash
curl http://<vps-ip>:3100/health
curl -H "Authorization: Bearer $TRINITY_API_KEY" http://<vps-ip>:3100/api/nodes
```

Agent on another machine:

```bash
CONTROLLER_URL=http://<vps-ip>:3100 TRINITY_ROLE=agent make run
```

No `/etc/hosts` or mkcert changes are required for this path.

---

## Agent — local against controller

### macOS (dev — embedded SOCKS)

Dante and systemd are Linux-only. On macOS, skip `install-agent-service.sh` and run the agent in the foreground:

```bash
# Terminal 1 — controller (loads .env.controller if present)
make start-controller

# Terminal 2 — agent with embedded SOCKS (no Dante)
make run-agent-dev
```

`make run-agent-dev` automatically:

- Sets `TRINITY_ROLE=agent`, `TRINITY_SKIP_INSTALLER=1`, and `TRINITY_DEVICE_CLASS=desktop`
- Starts embedded SOCKS5 on port **1080** (`TRINITY_SOCKS_PORT`, credentials `dev`/`dev`)
- Loads `TRINITY_AGENT_KEY` from `.env.controller` when present
- Defaults `CONTROLLER_URL` to `http://127.0.0.1:3100`
- Runs `./build/trinityproxy` in the foreground (Ctrl+C to stop)

Use `make run-agent` only on a Linux VPS — it installs the systemd service and Dante.

### Windows agent

Cross-compile the agent binary, copy it to a Windows PC, and run the PowerShell installer as Administrator:

```bash
make build-windows-agent   # produces build/trinityproxy.exe
```

See **[docs/WINDOWS_AGENT.md](WINDOWS_AGENT.md)** for the 3-step install (build → run `scripts/install-agent-windows.ps1` as Administrator → confirm in dashboard). The PS1 script installs the Windows service and points it at your controller; `trinityproxy.exe` is the agent binary it deploys. Windows agents run embedded SOCKS5 (Go-based, no Dante).

### Health probes (local dev)

Agents on the same machine as the controller may report a **public WAN IP** that is not reachable locally. The controller retries SOCKS probes via `127.0.0.1` when the WAN dial fails (enabled by default; disable with `TRINITY_ENV=production` or `TRINITY_PROBE_LOCAL_FALLBACK=0`). Set `SERVER_PUBLIC_IP` on the controller when agents share its public IP in production. Remote VPS agents are probed on their public IP first — loopback is only used after that dial fails.

### Docker — Linux agent on Mac (recommended)

Simulates a real Linux VPS agent without installing Dante on macOS. Requires [Docker Desktop](https://www.docker.com/products/docker-desktop/).

```bash
# Terminal 1 — dashboard
make start-dev

# Terminal 2 — controller API (loads .env.controller)
make start-controller

# Terminal 3 — Linux agent container
make docker-agent
```

The container:

- Runs on Debian with Dante SOCKS (foreground, no systemd)
- Sends heartbeats to `http://host.docker.internal:3100` on your Mac
- Uses `TRINITY_AGENT_KEY` from `.env.controller`

After ~60 seconds, open the dashboard **Agents** page to see the node.

```bash
docker logs -f trinityproxy-agent-dev   # follow agent logs
make docker-agent-down                  # stop and remove container
```

### Linux / manual

```bash
export CONTROLLER_URL=http://127.0.0.1:3100   # or your mkcert/VPS URL
export TRINITY_AGENT_KEY=<shared-secret>      # if controller requires it
TRINITY_ROLE=agent make run                   # interactive
# or on a VPS:
make run-agent                                # systemd + Dante
```

Heartbeats POST to `{CONTROLLER_URL}/api/heartbeat` every 60s (override with `HEARTBEAT_INTERVAL`).

---

## Quick reference

| Service | Default port | Start command |
|---------|--------------|---------------|
| Dashboard (API + UI) | 8080 / 8081 | `make start-dev` |
| Controller API | 3100 | `make run-controller` |
| Agent (macOS dev) | 1080 (SOCKS) | `make run-agent-dev` |
| Agent (Docker/Linux sim) | — | `make docker-agent` |
| Agent (Linux VPS) | — | `make run-agent` |
| Agent (Windows) | — | `make build-windows-agent` + `scripts/install-agent-windows.ps1` |
| Dashboard Go API only | 8081 | `make run-dashboard` |
| Dashboard Vite UI only | 8080 | `cd web/dashboard && npm run dev` |

| Command | Purpose |
|---------|---------|
| `make start-dev` | Start dashboard dev (recommended) |
| `make stop` | Stop dashboard dev servers |
| `make run-agent-dev` | macOS/local agent — embedded SOCKS :1080, foreground |
| `make docker-agent` | Linux agent in Docker — simulates VPS on Mac |
| `make docker-agent-down` | Stop/remove Docker agent container |
| `make run-agent` | Linux VPS agent — systemd + Dante |
| `make dashboard-up` | Check :8080 and :8081 are running |
| `scripts/dev-mkcert-setup.sh [domain]` | Install mkcert, generate local TLS certs |
| `scripts/dev-hosts-helper.sh add\|remove\|status [domain]` | Manage `/etc/hosts` safely |

---

## Troubleshooting

**Port already in use**

```bash
make stop
# or manually:
lsof -ti:8080 -ti:8081 -ti:3100
```

**Dashboard API on wrong port**

If something stale is bound to `:8080`, run `make stop` and try `make start-dev` again.

**Vite cannot reach API**

Ensure `make start-dev` is running (or Terminal 1 has `make run-dashboard`) and `VITE_API_PROXY_TARGET` matches (default `http://127.0.0.1:8081`).

**mkcert not trusted**

Run `mkcert -install` again. Firefox may need the NSS tools package (`brew install nss` on macOS).

**Hosts helper permission denied**

`add` and `remove` need sudo: `sudo ./scripts/dev-hosts-helper.sh add <domain>`.
