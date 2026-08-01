# Environment variables

Compact reference for TrinityProxy. Defaults shown where applicable.

## Production secrets (`/etc/trinityproxy/`)

`sudo make start` (or `make install-production`) auto-generates secrets on the VPS:

| File | Contents |
|------|----------|
| `/etc/trinityproxy/controller.env` | `TRINITY_API_KEY`, `TRINITY_AGENT_KEY`, `CONTROLLER_URL` (mode 640) |
| `/etc/trinityproxy/dashboard-admin.txt` | Dashboard login username/password (mode 600, printed once at install) |

Both controller and dashboard systemd units load `controller.env`. Re-running `make start` is safe — existing keys are preserved unless the dashboard DB generates a new agent key.

State databases: `/var/lib/trinityproxy/trinityproxy.db` (nodes), `/var/lib/trinityproxy/dashboard.db` (auth/settings).

## Controller API (`trinityproxy-api`, port `3100`)

| Variable | Default | Description |
|----------|---------|-------------|
| `API_PORT` | `3100` | Listen port |
| `DB_PATH` | `./trinityproxy.db` | Node registry SQLite |
| `TRINITY_AGENT_KEY` | — | Validates `POST /api/heartbeat` |
| `TRINITY_API_KEY` | — | Auth for `GET /api/nodes*` |
| `TRINITY_ADMIN_KEY` | falls back to `TRINITY_API_KEY` | `GET /api/nodes/admin` |
| `PROBE_INTERVAL` | `60s` | Background SOCKS probe interval |
| `HEARTBEAT_INTERVAL` | `60s` | Documented for agents |
| `TRINITY_ENV` | — | `production` disables dev probe fallback |
| `TRINITY_NONINTERACTIVE` | — | `1` = production probe mode |
| `LOG_FORMAT` | `json` | Set `text` for readable logs |

Manual key generation: `openssl rand -hex 32`

## Dashboard (`trinityproxy-dashboard`, port `8081`)

| Variable | Default | Description |
|----------|---------|-------------|
| `DASHBOARD_PORT` | `8081` | Listen port |
| `DASHBOARD_BIND_ADDR` | `0.0.0.0` | Bind address |
| `DASHBOARD_DB_PATH` | `./dashboard.db` | Auth and settings SQLite |
| `DASHBOARD_URL` | `http://localhost:8080` | Public URL for links |
| `DASHBOARD_STATIC_DIR` | — | Serve built UI from `web/dashboard/dist` (production) |
| `DASHBOARD_DEV_PROXY` | `false` | Proxy Vite when `true` |
| `DASHBOARD_SESSION_TTL` | `24h` | Login session lifetime |
| `CONTROLLER_URL` | — | Shown in deploy commands |
| `TRINITY_AGENT_KEY` | — | Shown in deploy commands |
| `SERVER_PUBLIC_IP` | auto | Cloudflare DNS hints |

## Agent

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROLLER_URL` | — | Controller base URL (required) |
| `TRINITY_AGENT_KEY` | — | Heartbeat auth (required in prod) |
| `TRINITY_ROLE` | — | `agent` or `controller` |
| `TRINITY_SKIP_INSTALLER` | — | `1` = embedded Go SOCKS (macOS/Windows/dev) |
| `TRINITY_SOCKS_PORT` | `1080` | Embedded SOCKS listen port |
| `TRINITY_DEVICE_CLASS` | auto | Dashboard label (`desktop`, `vps`, …) |

## Bridge dashboard → controller (dev)

```bash
make sync-agent-key    # writes .env.controller from dashboard.db
```

`make start-dev` does this automatically when a key exists.
