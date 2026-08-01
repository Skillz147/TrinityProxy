# TrinityProxy — Implementation Gaps

> See [ROADMAP.md](ROADMAP.md) for the active implementation plan and task tracking.

Last reconciled with code: Aug 2026 (dashboard shipped, Caddy + Cloudflare SSL, embedded SOCKS).

## ✅ Shipped

### Core platform
- Controller API (`:3100`) — heartbeats, node registry, SOCKS health probes, metrics
- Dashboard (`:8080` UI / `:8081` API) — auth, agents fleet view, Settings, Deploy Agent, Cloudflare SSL modal
- Embedded Go SOCKS5 — macOS, Windows, and local dev (`TRINITY_SKIP_INSTALLER=1`)
- Linux production agents — Dante + systemd via `install-agent-service.sh`
- Caddy reverse proxy — `setup-ssl-caddy.sh` (HTTP-01) and `setup-ssl-caddy-cloudflare.sh` (DNS-01 wildcard)
- Health probes — online vs healthy; dev local-fallback for same-machine agents
- API key auth — `TRINITY_API_KEY`, `TRINITY_AGENT_KEY`, `TRINITY_ADMIN_KEY`
- Structured logging (`log/slog`), 42+ unit tests (`go test ./...`)

### Agent installers
- Linux: `scripts/install-agent-service.sh` + curl bootstrap from dashboard
- macOS: `scripts/install-agent-macos.sh` (launchd + embedded SOCKS)
- Windows: `scripts/install-agent-windows.ps1` (service + embedded SOCKS)
- Docker dev agent: `make docker-agent`

---

## ❌ Real gaps (not yet implemented)

### Security & hardening
- [ ] Rate limiting on API endpoints
- [ ] Encrypted credential storage at rest (SQLite fields)
- [ ] Automatic credential rotation
- [ ] Optional HTTPS-only enforcement in Go (TLS currently terminates at Caddy)

### Observability & operations
- [ ] Per-node performance metrics (latency, bandwidth)
- [ ] Usage statistics and analytics in dashboard
- [ ] Backup and recovery procedures / tooling
- [ ] Multi-region deployment tooling

### Product & integrations
- [ ] CLI client (removed; use `curl` or custom client against API)
- [ ] Client libraries (Go, Python, etc.)
- [ ] Smart proxy selection and failover beyond `/api/nodes/random`
- [ ] Proxy testing utilities in dashboard
- [ ] Docker images and CI/CD pipeline

### Minor / polish
- [ ] Align env var names with ROADMAP `TRINITY_*` convention (`API_PORT` vs `TRINITY_API_PORT`) — documented, not blocking
- [ ] Graceful node failure handling beyond 5-minute offline timeout
