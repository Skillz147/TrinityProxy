# TrinityProxy
### Enterprise-Grade SOCKS5 Proxy Network Management System

[![Go Version](https://img.shields.io/badge/Go-1.24.3-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)](Makefile)

TrinityProxy is a distributed SOCKS5 proxy network built on a **Controller–Agent** architecture. A central API registers agent nodes, tracks health via heartbeats and SOCKS probes, and exposes REST endpoints for discovery. Agents run SOCKS5 locally (Dante on Linux VPS, embedded Go SOCKS on macOS/Windows) and report metadata on a fixed interval.

**Implementation status and backlog:** [ROADMAP.md](ROADMAP.md) · [MISSING_COMPONENTS.md](MISSING_COMPONENTS.md)

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         Controller VPS (production)                      │
│                                                                          │
│  ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────────┐  │
│  │  Dashboard UI   │   │  Dashboard API  │   │   Controller API    │  │
│  │    :8080 *      │──►│     :8081       │   │       :3100           │  │
│  │  (Vite dev)     │   │  (admin/auth)   │   │  (nodes/heartbeat)  │  │
│  └─────────────────┘   └─────────────────┘   └─────────────────────┘  │
│           │                     │                        │               │
│           └─────────────────────┴────────────────────────┘               │
│                                 │                                        │
│                          Caddy :443 (TLS)                                │
│              api.example.com → :3100    example.com → :8081              │
└──────────────────────────────────────────────────────────────────────────┘
          ▲ heartbeat + SOCKS probes                    ▲ heartbeats
          │                                             │
┌─────────┴─────────┐                         ┌─────────┴─────────┐
│   Linux Agent VPS │                         │  macOS / Windows  │
│  Dante SOCKS5     │                         │  Embedded SOCKS5  │
│  + systemd        │                         │  (Go, no Dante)   │
└───────────────────┘                         └───────────────────┘
```

\* In production, Caddy serves the built dashboard on your apex domain (`:8081` upstream). During local dev, Vite runs on `:8080` and proxies `/api` to the dashboard API on `:8081`.

### Core components

| Component | Port | Role |
|-----------|------|------|
| **Dashboard UI** | `:8080` (dev) | React admin UI — agents, settings, SSL provisioning |
| **Dashboard API** | `:8081` | Auth, deployment settings, stats, Cloudflare SSL orchestration |
| **Controller API** | `:3100` | Node registry, heartbeats, SOCKS health probes, `/api/nodes*` |
| **Agent (Linux)** | random `20000–59999` | Dante SOCKS5 + systemd heartbeat |
| **Agent (macOS/Windows)** | `:1080` default | Embedded Go SOCKS5 + heartbeat |
| **Database** | — | SQLite — `trinityproxy.db` (nodes), `dashboard.db` (auth/settings) |

---

## Quick Start (development)

### Prerequisites

- **Go 1.24.3+**
- **Node.js 18+** (dashboard UI)
- **Linux VPS with root** (production agents only; macOS/Windows use embedded SOCKS)

```bash
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart
```

### 1. Dashboard — one command

```bash
make start
# Open http://localhost:8080
```

Press **Ctrl+C** or run `make stop` when done.

**First time:**

1. `make start` — creates your admin login (credentials printed once)
2. Log in and **change your password** (required on first login)
3. **Settings** → enter your domain → **Save**
4. **Deploy Agent** → copy the install command for your platform

The dashboard runs a Go API on `:8081` and Vite on `:8080`. Agent keys sync to `.env.controller` automatically when configured.

See [docs/DEV_SETUP.md](docs/DEV_SETUP.md) for split-terminal and mkcert options.

### 2. Controller API

```bash
make start-controller
# equivalent: make sync-agent-key && make run-controller
```

Listens on **`:3100`**. Loads `TRINITY_AGENT_KEY` from `.env.controller` when present.

```bash
curl http://localhost:3100/health
curl http://localhost:3100/metrics
curl -H "Authorization: Bearer $TRINITY_API_KEY" http://localhost:3100/api/nodes
```

### 3. Local agent (macOS)

Dante and systemd are Linux-only. On macOS, use embedded SOCKS in the foreground:

```bash
# Terminal 1
make start-controller

# Terminal 2
make run-agent-dev
```

`make run-agent-dev` sets `TRINITY_SKIP_INSTALLER=1`, starts embedded SOCKS5 on **`:1080`** (`dev`/`dev` by default), and heartbeats to `http://127.0.0.1:3100`.

For a persistent macOS agent, use `make install-agent-macos` — see [Agent platforms](#agent-platforms) below.

---

## Production deployment (VPS)

### Controller host

```bash
# On your VPS
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart
make install-service          # systemd: trinityproxy-controller → :3100
sudo systemctl start trinityproxy-controller
```

> **Note:** `make vps-setup` installs dependencies and the controller systemd unit only. It does **not** configure TLS. Use the dashboard Cloudflare SSL flow or the Caddy scripts below for HTTPS.

### Dashboard + SSL (recommended)

1. Run the dashboard on the VPS (or tunnel to it during setup):

   ```bash
   make build-dashboard
   DASHBOARD_PORT=8081 ./build/trinityproxy-dashboard
   ```

   For production, serve the built UI from the dashboard binary or place `web/dashboard/dist` behind Caddy on `:8081`.

2. Open the dashboard → **Settings**:
   - Set your **public domain** (e.g. `example.com`)
   - Save deployment settings (generates `TRINITY_AGENT_KEY`)

3. **Settings → Cloudflare SSL** modal:
   - Create proxied (**orange cloud**) **A** records:
     - `api.example.com` → your VPS IP
     - `example.com` → your VPS IP (dashboard apex)
   - Create a Cloudflare API token with **Zone → DNS → Edit**
   - Paste the token and provision — Caddy issues a **DNS-01 wildcard** cert for `*.example.com` and `example.com`

4. Sync the agent key to the controller:

   ```bash
   make sync-agent-key
   # Add TRINITY_AGENT_KEY to /etc/systemd/system/trinityproxy-controller.service, then:
   sudo systemctl daemon-reload && sudo systemctl restart trinityproxy-controller
   ```

### SSL without the dashboard (Caddy scripts)

TrinityProxy uses **Caddy only** — no nginx or certbot in the default path.

| Script | Use case |
|--------|----------|
| `scripts/setup-ssl-caddy-cloudflare.sh` | **Production** — Cloudflare DNS-01 wildcard; proxied A records OK |
| `scripts/setup-ssl-caddy.sh` | Simple HTTP-01; requires **grey cloud** during initial issuance |

Cloudflare DNS-01 (wildcard + orange cloud):

```bash
sudo PUBLIC_DOMAIN=example.com CLOUDFLARE_API_TOKEN=... SERVER_IP=203.0.113.10 \
  EMAIL=ssl@example.com SKIP_DNS_WAIT=1 ./scripts/setup-ssl-caddy-cloudflare.sh
```

DNS checklist: [scripts/setup-cloudflare-dns.md](scripts/setup-cloudflare-dns.md)

After HTTPS is live, agents use `CONTROLLER_URL=https://api.example.com`.

### Deploy agents

From the dashboard **Deploy Agent** page, copy the platform-specific command. Summary:

| Platform | Command / script |
|----------|------------------|
| **Linux VPS** | `scripts/install-agent-service.sh` or curl bootstrap from Deploy page |
| **macOS** | `make install-agent-macos` or `scripts/install-agent-macos.sh` |
| **Windows** | `make build-windows-agent` → run `scripts/install-agent-windows.ps1` as Administrator |
| **Docker (Mac dev)** | `make docker-agent` |

---

## Agent platforms

### Linux VPS (production)

Installs Dante SOCKS5, generates credentials, registers a systemd service:

```bash
sudo CONTROLLER_URL=https://api.example.com TRINITY_AGENT_KEY=<key> \
  ./scripts/install-agent-service.sh
```

Or use the one-liner from the dashboard **Deploy Agent** page (curl bootstrap).

`make run-agent` on Linux performs the same setup interactively.

### macOS

**Dev (foreground):** `make run-agent-dev` — embedded SOCKS on `:1080`, no install.

**Persistent (launchd):**

```bash
make build
CONTROLLER_URL=https://api.example.com TRINITY_AGENT_KEY=<key> make install-agent-macos
```

### Windows

1. **Build** the agent binary (on any machine with Go):

   ```bash
   make build-windows-agent   # → build/trinityproxy.exe
   ```

2. Copy `trinityproxy.exe` (and optionally the repo) to the Windows PC.

3. **Run the installer as Administrator** — the PowerShell script installs the Windows service and configures it to talk to your controller; `trinityproxy.exe` is the agent binary the script installs:

   ```powershell
   $env:CONTROLLER_URL = "https://api.example.com"
   $env:TRINITY_AGENT_KEY = "<key-from-dashboard>"
   .\scripts\install-agent-windows.ps1
   ```

   The `.ps1` installer copies `trinityproxy.exe` to `C:\Program Files\TrinityProxy`, writes environment config, and registers the **TrinityProxyAgent** Windows service with embedded SOCKS5.

Full guide: [docs/WINDOWS_AGENT.md](docs/WINDOWS_AGENT.md)

---

## Health: Online vs Healthy

The dashboard and API track two independent states:

| State | Meaning |
|-------|---------|
| **Online** | Agent sent a heartbeat within the last **5 minutes** (`is_online`) |
| **Healthy** | Controller completed a **SOCKS5 TCP connect + username/password auth** probe to the node's reported `ip:port` (`is_healthy`) |

A node can be **online but unhealthy** (heartbeat works, SOCKS unreachable or bad credentials).

### How probes work

- After each heartbeat and on a background interval (`PROBE_INTERVAL`, default **60s**), the controller probes each online node.
- `GET /api/nodes/random` returns only nodes that are both online **and** SOCKS-healthy (results cached 30s per node).
- Failed probes increment `probe_failures` in `GET /metrics`.

### Dev (same machine) vs production (NAT)

| Environment | Probe behavior |
|-------------|----------------|
| **Local dev** | When `TRINITY_ENV` is not `production`/`prod` and `TRINITY_NONINTERACTIVE` is unset, probes **retry via `127.0.0.1`** if the agent's public WAN IP is unreachable locally (no NAT hairpin needed). This is why `make run-agent-dev` on the same Mac as the controller shows **Healthy**. |
| **Production** | `TRINITY_NONINTERACTIVE=1` (systemd) or `TRINITY_ENV=production` disables local fallback. Probes connect to the agent's **public IP only** — the SOCKS port must be reachable from the controller (open firewall, no CGNAT-only egress without port forwarding). |

---

## Controller API reference

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/api/heartbeat` | POST | Agent key | Agent heartbeat / registration |
| `/api/nodes` | GET | API key | List online nodes (no passwords) |
| `/api/nodes/admin` | GET | Admin key | Full credentials |
| `/api/nodes/country?country=US` | GET | API key | Filter by country |
| `/api/nodes/random` | GET | API key | Random SOCKS-healthy online node |
| `/health` | GET | — | Controller health |
| `/metrics` | GET | — | JSON counters |

Example heartbeat:

```bash
curl -X POST http://localhost:3100/api/heartbeat \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $TRINITY_AGENT_KEY" \
  -d '{"ip":"203.0.113.1","port":45023,"username":"u_abc","password":"secret","country":"US"}'
```

There is **no CLI client** in the repository. Use `curl` or your own client.

### Using a proxy node

```bash
curl --socks5 username:password@vps-ip:port http://httpbin.org/ip
export SOCKS_PROXY="socks5://username:password@vps-ip:port"
```

---

## Environment variables

### Controller API

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROLLER_URL` | — | Base URL agents use (set in dashboard Settings or env) |
| `API_PORT` | `3100` | Controller listen port |
| `DB_PATH` | `./trinityproxy.db` | Node registry SQLite path |
| `HEARTBEAT_INTERVAL` | `60s` | Agent heartbeat interval (agents read this too) |
| `PROBE_INTERVAL` | `60s` | Background SOCKS health probe interval |
| `TRINITY_API_KEY` | — | Client auth for `GET /api/nodes*` |
| `TRINITY_AGENT_KEY` | — | Agent auth for `POST /api/heartbeat` |
| `TRINITY_ADMIN_KEY` | — | Admin export; falls back to `TRINITY_API_KEY` |
| `TRINITY_ENV` | — | `production` or `prod` disables dev probe fallback |
| `TRINITY_NONINTERACTIVE` | — | `1` skips prompts; enables production probe mode |
| `LOG_FORMAT` | `json` | Set `text` for human-readable logs |

When `TRINITY_API_KEY` or `TRINITY_AGENT_KEY` is unset, the controller logs a warning and allows those endpoints without auth. **Set both before exposing to any network.**

```bash
openssl rand -hex 32   # TRINITY_API_KEY
openssl rand -hex 32   # TRINITY_AGENT_KEY (different value)
```

### Dashboard

| Variable | Default | Description |
|----------|---------|-------------|
| `DASHBOARD_PORT` | `8081` | Dashboard API bind port |
| `DASHBOARD_BIND_ADDR` | `0.0.0.0` | Dashboard API bind address |
| `DASHBOARD_DB_PATH` | `./dashboard.db` | Auth and settings SQLite |
| `DASHBOARD_URL` | `http://localhost:8080` | Public URL for links/redirects |
| `DASHBOARD_DEV_PROXY` | `false` | Go process proxies Vite when `true` |
| `DASHBOARD_SESSION_TTL` | `24h` | Login session lifetime |
| `DASHBOARD_ADMIN_USERNAME` | `admin` | Bootstrap admin username |
| `SERVER_PUBLIC_IP` | auto | VPS public IP for Cloudflare DNS hints |

### Agent

| Variable | Default | Description |
|----------|---------|-------------|
| `TRINITY_ROLE` | — | `controller` or `agent` |
| `TRINITY_SKIP_INSTALLER` | — | `1` = embedded Go SOCKS (macOS/Windows/dev) |
| `TRINITY_SOCKS_PORT` | `1080` | Embedded SOCKS listen port |
| `TRINITY_SOCKS_USER` | auto | Override SOCKS username |
| `TRINITY_SOCKS_PASSWORD` | auto | Override SOCKS password |
| `TRINITY_DEVICE_CLASS` | auto | Dashboard label: `desktop`, `vps`, `macos`, … |
| `TRINITY_NETWORK_TYPE` | — | Optional network label in heartbeat |
| `TRINITY_DATA_DIR` | install dir | Credential file location (Windows/macOS) |
| `TRINITY_ROOT` | — | Override path to `build/` binaries |

Bridge dashboard → controller:

```bash
make sync-agent-key    # writes TRINITY_AGENT_KEY to .env.controller
```

---

## Building

```bash
make build
```

| Binary | Path |
|--------|------|
| Role router (agent/controller) | `build/trinityproxy` |
| Agent installer (Dante) | `build/installer` |
| Controller API | `build/trinityproxy-api` |
| Dashboard API | `build/trinityproxy-dashboard` |
| Windows agent | `build/trinityproxy.exe` |

---

## Systemd services

```bash
make install-service          # controller → trinityproxy-controller
make install-agent-service    # agent → trinityproxy-agent

sudo systemctl status trinityproxy-controller
sudo journalctl -u trinityproxy-agent -f
```

Set `TRINITY_AGENT_KEY` and `CONTROLLER_URL` in the service unit before install when the controller requires heartbeat auth.

---

## Security

- Each agent generates unique username/password (Linux: `/etc/trinityproxy-*` mode `600`; Windows/macOS: install directory)
- SOCKS ports are randomized on Linux (20000–59999); embedded agents default to `:1080`
- Public GET endpoints omit passwords; use `GET /api/nodes/admin` with an admin key for exports
- Set `TRINITY_API_KEY` and `TRINITY_AGENT_KEY` in production; `/health` and `/metrics` remain open
- Dante template requires username auth only

---

## Project structure

```
TrinityProxy/
├── main.go                         # Role router; embedded SOCKS on agent
├── cmd/
│   ├── api/enhanced_main.go        # Controller API (:3100)
│   ├── dashboard/main.go           # Dashboard API (:8081)
│   └── installer/installer.go      # Dante + credential setup (Linux)
├── internal/
│   ├── config/                     # Env-based configuration
│   ├── dashboard/                  # Auth, stats, deployment, Cloudflare SSL
│   ├── proxy/                      # Embedded Go SOCKS5 server
│   ├── health/                     # SOCKS probes + background prober
│   └── storage/                    # SQLite node registry
├── web/dashboard/                  # React admin UI (Vite)
├── scripts/                        # Caddy SSL, systemd, agent installers
├── docs/DEV_SETUP.md
├── docs/WINDOWS_AGENT.md
├── ROADMAP.md
└── MISSING_COMPONENTS.md
```

---

## Troubleshooting

```bash
make check-deps
make status
make dashboard-up               # check :8080 and :8081
sudo systemctl status trinityproxy-controller
sudo journalctl -u trinityproxy-agent -f
sudo tail -f /var/log/danted.log   # Linux Dante logs
make clean && make build
```

| Symptom | Check |
|---------|-------|
| Agent online but unhealthy (prod) | Firewall on agent VPS; SOCKS port reachable from controller |
| Agent online but unhealthy (dev) | Use `make run-agent-dev` on same machine; dev probe fallback is automatic |
| Heartbeat 401 | `TRINITY_AGENT_KEY` mismatch — run `make sync-agent-key` |
| Dashboard can't reach controller | `CONTROLLER_URL` in Settings; controller running on `:3100` |

---

## Development

```bash
make format
make lint
make test
make dev-controller
make dev-agent
```

Full local setup: [docs/DEV_SETUP.md](docs/DEV_SETUP.md)

---

## Contributing

1. Fork the repository
2. Create a feature branch
3. Run `make format` and `make test`
4. Update docs for behavior changes
5. Open a Pull Request

---

## License

MIT License — see [LICENSE](LICENSE).

## Support

- **Issues:** [GitHub Issues](https://github.com/Skillz147/TrinityProxy/issues)
- **Roadmap:** [ROADMAP.md](ROADMAP.md)
- **Commands:** `make help`
