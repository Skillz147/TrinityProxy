# TrinityProxy — Windows Agent Install

Install the TrinityProxy agent on a Windows PC so it reports to your controller **and** runs a local embedded SOCKS5 proxy (Go-based — no Dante).

The agent:
1. Starts an embedded SOCKS5 listener on an **auto-selected free port** in `10800–10999` (override with `TRINITY_SOCKS_PORT`)
2. Sends heartbeats every 60 seconds with the node's public IP, SOCKS port, and **install-time generated credentials**

## Pre-built binary (recommended)

No Go required on the Windows PC. After the [Release agent binaries](https://github.com/Skillz147/TrinityProxy/actions/workflows/release-binaries.yml) workflow has run on `main`, download:

`https://github.com/Skillz147/TrinityProxy/releases/download/latest/trinityproxy-windows-amd64.exe`

The dashboard one-liner and `scripts/install-agent-windows.ps1` use this URL automatically.

---

## Step 1 — Build the agent binary (optional)

On a machine with Go installed (macOS, Linux, or Windows):

```bash
make build-windows-agent
```

This creates `build/trinityproxy.exe`. Copy that file to your Windows PC (USB drive, cloud storage, or SCP).

### Testing on Windows (dev)

On the Windows machine itself you can also build directly:

```powershell
go build -o build\trinityproxy.exe .
```

Run in the foreground for quick testing (no service install):

```powershell
$env:TRINITY_ROLE = "agent"
$env:TRINITY_DEV = "1"
$env:TRINITY_NONINTERACTIVE = "1"
$env:TRINITY_SKIP_INSTALLER = "1"
$env:TRINITY_SOCKS_PORT = "1080"
$env:TRINITY_SOCKS_USER = "dev"
$env:TRINITY_SOCKS_PASSWORD = "dev"
$env:CONTROLLER_URL = "https://api.yourdomain.com"
$env:TRINITY_AGENT_KEY = "paste-key-from-dashboard"
.\build\trinityproxy.exe
```

For local dev testing only, `TRINITY_DEV=1` with `dev`/`dev` credentials is fine. Production installs generate random credentials automatically.

---

## Step 2 — Run the installer (as Administrator)

1. Copy the whole `TrinityProxy` folder to the Windows PC, **or** place `trinityproxy.exe` anywhere and note the path.
2. Right-click **PowerShell** → **Run as administrator**.
3. Run one of the options below.

**From the project folder** (if you copied the repo):

```powershell
cd C:\path\to\TrinityProxy
$env:CONTROLLER_URL = "https://api.yourdomain.com"
$env:TRINITY_AGENT_KEY = "paste-key-from-dashboard"
$env:TRINITY_NONINTERACTIVE = "1"
.\scripts\install-agent-windows.ps1
```

The installer auto-picks a free SOCKS port (10800–10999), generates random credentials, opens Windows Firewall for that port only, and writes config to `C:\Program Files\TrinityProxy`.

**With a standalone `.exe`** (no repo folder):

```powershell
$env:CONTROLLER_URL = "https://api.yourdomain.com"
$env:TRINITY_AGENT_KEY = "paste-key-from-dashboard"
$env:TRINITY_NONINTERACTIVE = "1"
$env:TRINITY_LOCAL_BINARY = "C:\Downloads\trinityproxy.exe"
.\install-agent-windows.ps1
```

Get `CONTROLLER_URL` and `TRINITY_AGENT_KEY` from your dashboard **Settings → Deploy Agent** page.

The installer:
- Copies `trinityproxy.exe` to `C:\Program Files\TrinityProxy`
- Writes `start-agent.cmd` with required environment variables
- Registers a Windows Service (`TrinityProxyAgent`)
- Adds an inbound Windows Firewall rule for the SOCKS port

---

## Step 3 — Confirm in the dashboard

1. Open your TrinityProxy dashboard.
2. Go to **Agents**.
3. Within about a minute, your Windows PC should appear as a node with SOCKS credentials.

Retrieve local credentials (if needed):

```powershell
Get-Content "C:\Program Files\TrinityProxy\trinityproxy-username"
Get-Content "C:\Program Files\TrinityProxy\trinityproxy-password"
```

---

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `CONTROLLER_URL` | Yes | Controller base URL (e.g. `https://api.example.com`) |
| `TRINITY_AGENT_KEY` | Yes (prod) | Heartbeat auth key from dashboard Settings |
| `TRINITY_ROLE` | Yes | Must be `agent` (set automatically by installer) |
| `TRINITY_NONINTERACTIVE` | Yes | Set to `1` for service/unattended mode |
| `TRINITY_SKIP_INSTALLER` | Yes | Set to `1` — use embedded Go SOCKS instead of Dante |
| `TRINITY_SOCKS_PORT` | No | Explicit SOCKS port; default is auto-picked free port in `10800–10999` |
| `TRINITY_SOCKS_USER` | No | Override auto-generated SOCKS username (16-char hex by default) |
| `TRINITY_SOCKS_PASSWORD` | No | Override auto-generated SOCKS password (32-char hex by default) |
| `TRINITY_DEV` | No | Set to `1` for local dev only (`dev`/`dev` credentials on `:1080`) |
| `TRINITY_DEVICE_CLASS` | No | Label in dashboard (default `desktop` on Windows) |

---

## Managing the agent

The installer registers a Windows Service named **TrinityProxy Agent**. Open **services.msc** to view it.

In an elevated PowerShell:

```powershell
Get-Service TrinityProxyAgent          # check status
Stop-Service TrinityProxyAgent          # stop
Start-Service TrinityProxyAgent         # start
sc.exe delete TrinityProxyAgent         # uninstall service (then delete C:\Program Files\TrinityProxy)
```

If the service could not be created, the installer falls back to a **scheduled task** with the same name. Use `Get-ScheduledTask -TaskName TrinityProxyAgent` to manage it.

---

## Cross-compile from macOS/Linux

```bash
GOOS=windows GOARCH=amd64 go build -o build/trinityproxy.exe .
# same as: make build-windows-agent
```

Copy `build/trinityproxy.exe` to Windows and run the installer script above.

---

## Troubleshooting

| Problem | What to try |
|---------|-------------|
| “Run as Administrator” error | Right-click PowerShell → Run as administrator |
| Binary not found | Set `TRINITY_LOCAL_BINARY` to the full path of `trinityproxy.exe` |
| Node not in dashboard | Check `CONTROLLER_URL` and `TRINITY_AGENT_KEY` match dashboard Settings |
| Service won’t start | Re-run the installer; or use `-UseScheduledTask` for task-based startup |
| SOCKS connection refused | Confirm firewall rule exists; check `TRINITY_SOCKS_PORT` matches |
| Wrong SOCKS credentials | Read `trinityproxy-username` / `trinityproxy-password` in install folder |

```powershell
# Force scheduled-task mode instead of Windows Service
.\scripts\install-agent-windows.ps1 -UseScheduledTask

# Test SOCKS through the local proxy
curl --proxy socks5://USER:PASS@127.0.0.1:1080 https://api.ipify.org
```
