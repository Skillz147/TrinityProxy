# TrinityProxy

Distributed SOCKS5 proxy network with a central controller, web dashboard, and agent nodes. The controller registers agents via heartbeats; the dashboard manages settings, SSL, and deployment.

**Status:** [ROADMAP.md](ROADMAP.md) · [MISSING_COMPONENTS.md](MISSING_COMPONENTS.md)

---

## Local development

**Prerequisites:** Go 1.24+, Node.js 18+

```bash
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make start
```

Open **http://localhost:8080** and log in. `make start` launches:

| Service | Port |
|---------|------|
| Dashboard UI (Vite) | `:8080` |
| Dashboard API | `:8081` |
| Controller API | `:3100` |

First run prints admin credentials once. Then: change password → **Settings** (domain) → **Deploy Agent**.

Stop everything: `make stop`

**macOS agent (same machine):**

```bash
make run-agent-dev    # embedded SOCKS :1080, heartbeats to :3100
```

More dev options: [docs/DEV_SETUP.md](docs/DEV_SETUP.md)

---

## Production VPS

On your controller host:

```bash
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart          # deps + build
make install-production  # systemd: auto-restart on reboot
```

This installs two services:

| Service | Port | Role |
|---------|------|------|
| `trinityproxy-controller` | `:3100` | Node registry, heartbeats, SOCKS probes |
| `trinityproxy-dashboard` | `:8081` | Admin API + built React UI |

```bash
sudo systemctl status trinityproxy-controller trinityproxy-dashboard
```

**Then in the dashboard** (http://your-vps-ip:8081):

1. Log in → **Settings** → set your public domain → Save
2. **Settings → Cloudflare SSL** — provision HTTPS (or use `scripts/setup-ssl-caddy-cloudflare.sh`)
3. **Deploy Agent** — copy the install command for each agent VPS

Point DNS at your VPS. Caddy (via the SSL scripts) reverse-proxies `api.yourdomain.com` → `:3100` and `yourdomain.com` → `:8081`.

Install controller or dashboard alone: `make install-service` / `make install-dashboard-service`

---

## Agents

- **Linux VPS** — copy the curl one-liner from **Deploy Agent**, or:
  ```bash
  sudo CONTROLLER_URL=https://api.example.com TRINITY_AGENT_KEY=<key> ./scripts/install-agent-service.sh
  ```
- **Windows** — `make build-windows-agent`, then run `scripts/install-agent-windows.ps1` as Administrator ([docs/WINDOWS_AGENT.md](docs/WINDOWS_AGENT.md))
- **macOS dev** — `make run-agent-dev` (foreground) or `make install-agent-macos` (launchd)

---

## Online vs Healthy

| State | Meaning |
|-------|---------|
| **Online** | Heartbeat received in the last 5 minutes |
| **Healthy** | Controller completed a SOCKS5 auth probe to the agent's `ip:port` |

A node can be online but unhealthy (heartbeat works, SOCKS unreachable or bad credentials). In local dev, probes retry via `127.0.0.1` automatically.

---

## Environment variables

The five you need most often:

| Variable | Used by | Purpose |
|----------|---------|---------|
| `TRINITY_AGENT_KEY` | Controller + agents | Authenticates heartbeats |
| `CONTROLLER_URL` | Agents + dashboard | Base URL agents call (e.g. `https://api.example.com`) |
| `TRINITY_API_KEY` | API clients | Auth for `GET /api/nodes*` |
| `DASHBOARD_PORT` | Dashboard | Listen port (default `8081`) |
| `DB_PATH` | Controller | Node registry SQLite path |

Full reference: [docs/ENV.md](docs/ENV.md)

`make start` syncs `TRINITY_AGENT_KEY` from the dashboard DB into `.env.controller` automatically.

---

## Build

```bash
make build
```

| Binary | Path |
|--------|------|
| Agent / role router | `build/trinityproxy` |
| Controller API | `build/trinityproxy-api` |
| Dashboard | `build/trinityproxy-dashboard` |

---

## Troubleshooting

```bash
make stop && make start     # restart dev stack
make dashboard-up           # check :8080 and :8081
curl http://localhost:3100/health
sudo journalctl -u trinityproxy-controller -f
sudo journalctl -u trinityproxy-dashboard -f
```

| Symptom | Fix |
|---------|-----|
| Heartbeat 401 | `make sync-agent-key`, restart controller |
| Agent online, unhealthy (prod) | Open SOCKS port on agent firewall |
| Port in use | `make stop` |

---

## License

MIT — see [LICENSE](LICENSE)
