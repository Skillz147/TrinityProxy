# Cloudflare DNS for TrinityProxy

Configure DNS **before** running `setup-ssl-caddy.sh`. The script prints a record checklist and waits for confirmation when run interactively.

## Recommended subdomain pattern

| Service | Example hostname | Proxies to |
|---------|------------------|------------|
| Controller API | `api.example.com` | `localhost:3100` |
| Dashboard (optional) | `dashboard.example.com` | `localhost:8081` |

Agents should use the HTTPS controller URL:

```bash
CONTROLLER_URL=https://api.example.com TRINITY_ROLE=agent make run
```

## Required records

Replace `example.com`, `SERVER_IP`, and optional `SERVER_IPV6` with your values.

### Controller API (required)

| Type | Name | Content | Proxy |
|------|------|---------|-------|
| **A** | `api` (or full `api.example.com`) | `SERVER_IP` | See below |
| **AAAA** | `api` | `SERVER_IPV6` | Optional; only if the VPS has IPv6 |

### Dashboard (optional)

| Type | Name | Content | Proxy |
|------|------|---------|-------|
| **A** | `dashboard` | `SERVER_IP` | See below |
| **AAAA** | `dashboard` | `SERVER_IPV6` | Optional |

### CNAME alternative

If you prefer a CNAME instead of a direct A record on a subdomain:

| Type | Name | Content | Notes |
|------|------|---------|-------|
| **CNAME** | `api` | `controller.example.com` | Target must already resolve to `SERVER_IP` |
| **A** | `controller` | `SERVER_IP` | Root target for CNAME chain |

TrinityProxy scripts expect **A/AAAA records pointing at the VPS** that runs the controller. CNAME is fine as long as the chain resolves to that host.

## Cloudflare proxy (orange cloud) notes

Cloudflare’s **Proxied** (orange cloud) mode terminates TLS at Cloudflare’s edge. That affects Let’s Encrypt on your server:

| Setup | Orange cloud during cert issue | Ongoing |
|-------|----------------------------------|---------|
| **Caddy HTTP-01** (default scripts) | Use **DNS only** (grey cloud) until the certificate is issued | You may enable orange cloud afterward; renewals need port 80 reachable or switch to DNS-01 |
| **Certbot DNS-01** (`certbot-dns-cloudflare`) | Orange cloud OK | Orange cloud OK |
| **Caddy only, grey cloud** | Grey cloud | Grey cloud — LE hits your server directly |

**Practical workflow for HTTP-01:**

1. Create A/AAAA records as **DNS only** (grey cloud).
2. Wait for propagation (`dig +short api.example.com` → `SERVER_IP`).
3. Run `sudo CONTROLLER_DOMAIN=... SERVER_IP=... EMAIL=... ./scripts/setup-ssl-caddy.sh`.
4. After HTTPS works, optionally enable **Proxied** on the record if you want Cloudflare WAF/CDN in front of the API.

**If you keep orange cloud enabled:** use Cloudflare’s origin certificate, or issue certs via DNS-01 — the stock scripts use HTTP-01 on ports 80/443 on the VPS.

## Verification

```bash
# Should return SERVER_IP before cert install
dig +short A api.example.com

# After SSL setup
curl -sS "https://api.example.com/health"
```

## Environment variables (setup scripts)

| Variable | Required | Example |
|----------|----------|---------|
| `CONTROLLER_DOMAIN` | Yes | `api.example.com` |
| `SERVER_IP` | Yes | `203.0.113.10` |
| `EMAIL` | Yes | `admin@example.com` |
| `DASHBOARD_DOMAIN` | No | `dashboard.example.com` |
| `SERVER_IPV6` | No | `2001:db8::1` |
| `SKIP_DNS_WAIT` | No | `1` — skip interactive “press Enter after DNS is ready” |

See also: [README.md](../README.md#https--reverse-proxy) (Deployment section).
