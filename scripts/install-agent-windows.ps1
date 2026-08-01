#Requires -Version 5.1
<#
.SYNOPSIS
  Install the TrinityProxy agent on Windows as a background service.

.DESCRIPTION
  Copies trinityproxy.exe to Program Files, registers a Windows Service, and
  starts the agent with an embedded Go SOCKS5 proxy (no Dante).

  Non-interactive when these environment variables are set:
    TRINITY_NONINTERACTIVE=1
    CONTROLLER_URL=https://api.yourdomain.com
    TRINITY_AGENT_KEY=your-shared-secret

  Optional:
    TRINITY_SOCKS_PORT=1080              SOCKS listen port (default 1080)
    TRINITY_SOCKS_USER / TRINITY_SOCKS_PASSWORD  Override auto-generated SOCKS credentials
    TRINITY_LOCAL_BINARY=C:\path\to\trinityproxy.exe   Use a pre-built binary
    TRINITY_DOWNLOAD_URL=https://.../trinityproxy.exe  Download instead of copy
    TRINITY_INSTALL_DIR=C:\Program Files\TrinityProxy  Override install folder

  Run in an elevated PowerShell (Run as administrator).

  Build the Windows binary on macOS/Linux:
    make build-windows-agent
    # produces build/trinityproxy.exe
#>

[CmdletBinding()]
param(
    [string]$ControllerUrl = $env:CONTROLLER_URL,
    [string]$AgentKey = $env:TRINITY_AGENT_KEY,
    [string]$SocksPort = $env:TRINITY_SOCKS_PORT,
    [string]$LocalBinary = $env:TRINITY_LOCAL_BINARY,
    [string]$DownloadUrl = $env:TRINITY_DOWNLOAD_URL,
    [string]$InstallDir = $(if ($env:TRINITY_INSTALL_DIR) { $env:TRINITY_INSTALL_DIR } else { Join-Path ${env:ProgramFiles} "TrinityProxy" }),
    [switch]$UseScheduledTask
)

$ErrorActionPreference = "Stop"

$ServiceName = "TrinityProxyAgent"
$ServiceDisplayName = "TrinityProxy Agent"
$BinaryName = "trinityproxy.exe"
$WrapperName = "start-agent.cmd"
$DefaultSocksPort = 1080

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host ">> $Message" -ForegroundColor Cyan
}

function Write-Ok([string]$Message) {
    Write-Host "   OK: $Message" -ForegroundColor Green
}

function Write-Warn([string]$Message) {
    Write-Host "   Note: $Message" -ForegroundColor Yellow
}

function Write-Fail([string]$Message) {
    Write-Host "   ERROR: $Message" -ForegroundColor Red
}

function Test-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Test-NonInteractive {
    if ($env:TRINITY_NONINTERACTIVE -eq "1") { return $true }
    return $false
}

function Resolve-SocksPort {
    if (-not $SocksPort) {
        return $DefaultSocksPort
    }
    $parsed = 0
    if (-not [int]::TryParse($SocksPort, [ref]$parsed) -or $parsed -lt 1 -or $parsed -gt 65535) {
        throw "TRINITY_SOCKS_PORT must be a number between 1 and 65535 (got: $SocksPort)"
    }
    return $parsed
}

function Resolve-SourceBinary {
    if ($LocalBinary -and (Test-Path -LiteralPath $LocalBinary)) {
        Write-Ok "Using binary from TRINITY_LOCAL_BINARY"
        return (Resolve-Path -LiteralPath $LocalBinary).Path
    }

    $repoBinary = Join-Path (Split-Path -Parent $PSScriptRoot) "build\$BinaryName"
    if (Test-Path -LiteralPath $repoBinary) {
        Write-Ok "Using local build: build\$BinaryName"
        return (Resolve-Path -LiteralPath $repoBinary).Path
    }

    if ($DownloadUrl) {
        $tempFile = Join-Path $env:TEMP "trinityproxy-download.exe"
        Write-Step "Downloading TrinityProxy agent..."
        Write-Host "   From: $DownloadUrl"
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $tempFile -UseBasicParsing
        if (-not (Test-Path -LiteralPath $tempFile)) {
            throw "Download failed — file not found after download."
        }
        Write-Ok "Download complete"
        return $tempFile
    }

    throw @"
Could not find trinityproxy.exe.

Build it on your dev machine:
  make build-windows-agent

Then copy build/trinityproxy.exe to this PC and either:
  - Place it next to this script under build/trinityproxy.exe, or
  - Set TRINITY_LOCAL_BINARY to the full path, or
  - Set TRINITY_DOWNLOAD_URL to a direct download link.
"@
}

function Ensure-FirewallRule([int]$Port) {
    $ruleName = "TrinityProxy SOCKS5 (TCP $Port)"
    $existing = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Ok "Firewall rule already exists: $ruleName"
        return
    }

    Write-Step "Opening Windows Firewall for SOCKS port $Port..."
    New-NetFirewallRule `
        -DisplayName $ruleName `
        -Direction Inbound `
        -Protocol TCP `
        -LocalPort $Port `
        -Action Allow `
        -Profile Any | Out-Null
    Write-Ok "Firewall rule added for inbound TCP port $Port"
}

function Write-WrapperScript([string]$TargetDir, [int]$Port) {
    $wrapperPath = Join-Path $TargetDir $WrapperName
    $exePath = Join-Path $TargetDir $BinaryName

    $lines = @(
        "@echo off",
        "rem TrinityProxy agent launcher — do not edit; re-run install-agent-windows.ps1 to update",
        "set TRINITY_ROLE=agent",
        "set TRINITY_NONINTERACTIVE=1",
        "set TRINITY_SKIP_INSTALLER=1",
        ("set TRINITY_SOCKS_PORT=" + $Port),
        ("set CONTROLLER_URL=" + $ControllerUrl),
        ("set TRINITY_AGENT_KEY=" + $AgentKey),
        'cd /d "%~dp0"',
        ('"' + $exePath + '"')
    )

    if ($env:TRINITY_SOCKS_USER) {
        $lines = $lines[0..($lines.Length - 2)] + ("set TRINITY_SOCKS_USER=" + $env:TRINITY_SOCKS_USER) + $lines[-1]
    }
    if ($env:TRINITY_SOCKS_PASSWORD) {
        $lines = $lines[0..($lines.Length - 2)] + ("set TRINITY_SOCKS_PASSWORD=" + $env:TRINITY_SOCKS_PASSWORD) + $lines[-1]
    }
    if ($env:TRINITY_DEVICE_CLASS) {
        $lines = $lines[0..($lines.Length - 2)] + ("set TRINITY_DEVICE_CLASS=" + $env:TRINITY_DEVICE_CLASS) + $lines[-1]
    }

    Set-Content -Path $wrapperPath -Value ($lines -join "`r`n") -Encoding ASCII
    return $wrapperPath
}

function Install-WindowsService([string]$WrapperPath) {
    $existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Step "Updating existing Windows service..."
        if ($existing.Status -eq "Running") {
            Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
            Start-Sleep -Seconds 2
        }
        & sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 2
    }

    Write-Step "Registering Windows service..."
    $binArg = "`"$WrapperPath`""
    & sc.exe create $ServiceName binPath= $binArg start= auto DisplayName= $ServiceDisplayName | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "sc.exe create failed (exit $LASTEXITCODE). Try running this script as Administrator."
    }

    & sc.exe description $ServiceName "TrinityProxy agent — embedded SOCKS5 proxy and controller heartbeats" | Out-Null
    & sc.exe failure $ServiceName reset= 86400 actions= restart/60000/restart/60000/restart/60000 | Out-Null
    Write-Ok "Service registered: $ServiceName"
}

function Install-ScheduledTaskFallback([string]$WrapperPath) {
    Write-Warn "Using a scheduled task instead of a Windows service (fallback mode)."
    $taskName = $ServiceName

    $existing = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($existing) {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    }

    $action = New-ScheduledTaskAction -Execute "cmd.exe" -Argument "/c `"$WrapperPath`"" -WorkingDirectory $InstallDir
    $triggerBoot = New-ScheduledTaskTrigger -AtStartup
    $triggerLogon = New-ScheduledTaskTrigger -AtLogOn
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger @($triggerBoot, $triggerLogon) -Settings $settings -Principal $principal -Description "TrinityProxy agent — embedded SOCKS5 proxy and controller heartbeats" | Out-Null
    Start-ScheduledTask -TaskName $taskName
    Write-Ok "Scheduled task registered and started: $taskName"
}

function Start-AgentBackground([string]$WrapperPath) {
    if ($UseScheduledTask) {
        Install-ScheduledTaskFallback -WrapperPath $WrapperPath
        return
    }

    try {
        Install-WindowsService -WrapperPath $WrapperPath
        Write-Step "Starting the agent..."
        & sc.exe start $ServiceName | Out-Null
        Start-Sleep -Seconds 3

        $svc = Get-Service -Name $ServiceName -ErrorAction Stop
        if ($svc.Status -eq "Running") {
            Write-Ok "Agent is running in the background"
            return
        }

        throw "Service installed but not running (status: $($svc.Status))."
    }
    catch {
        Write-Warn "Windows service setup failed: $($_.Exception.Message)"
        Write-Warn "Falling back to a scheduled task..."
        Install-ScheduledTaskFallback -WrapperPath $WrapperPath
    }
}

# --- Main ---

Write-Host ""
Write-Host "========================================" -ForegroundColor White
Write-Host "  TrinityProxy — Windows Agent Setup" -ForegroundColor White
Write-Host "========================================" -ForegroundColor White

if (-not (Test-Admin)) {
    Write-Fail "This installer must run as Administrator."
    Write-Host ""
    Write-Host "Right-click PowerShell and choose 'Run as administrator', then run this script again."
    exit 1
}

$nonInteractive = Test-NonInteractive
$socksPort = Resolve-SocksPort

if (-not $ControllerUrl) {
    if ($nonInteractive) {
        Write-Fail "CONTROLLER_URL is required when TRINITY_NONINTERACTIVE=1"
        exit 1
    }
    $ControllerUrl = Read-Host "Enter your controller URL (example: https://api.example.com)"
}

if (-not $AgentKey) {
    if ($nonInteractive) {
        Write-Fail "TRINITY_AGENT_KEY is required when TRINITY_NONINTERACTIVE=1"
        exit 1
    }
    $secure = Read-Host "Enter your agent key (from the dashboard Deploy Agent page)" -AsSecureString
    $AgentKey = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
        [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    )
}

$ControllerUrl = $ControllerUrl.Trim().TrimEnd("/")
if (-not ($ControllerUrl -match "^https?://")) {
    Write-Fail "CONTROLLER_URL must start with http:// or https://"
    exit 1
}

Write-Step "Preparing install folder..."
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Write-Ok "Install folder: $InstallDir"

Write-Step "Locating TrinityProxy binary..."
$sourceBinary = Resolve-SourceBinary
$targetBinary = Join-Path $InstallDir $BinaryName
Copy-Item -LiteralPath $sourceBinary -Destination $targetBinary -Force
Write-Ok "Installed $BinaryName"

Ensure-FirewallRule -Port $socksPort

Write-Step "Writing agent configuration..."
$wrapperPath = Write-WrapperScript -TargetDir $InstallDir -Port $socksPort
Write-Ok "Launcher script ready (TRINITY_SKIP_INSTALLER=1, TRINITY_SOCKS_PORT=$socksPort)"

Start-AgentBackground -WrapperPath $wrapperPath

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Setup complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Your Windows PC is now reporting to:"
Write-Host "  $ControllerUrl"
Write-Host ""
Write-Host "Embedded SOCKS5 proxy listens on TCP port $socksPort."
Write-Host "SOCKS credentials are auto-generated on first run and saved in:"
Write-Host "  $InstallDir\trinityproxy-username"
Write-Host "  $InstallDir\trinityproxy-password"
Write-Host ""
Write-Host "The agent runs in the background and starts automatically when Windows boots."
Write-Host "Open your TrinityProxy dashboard Agents page — the node should appear within about a minute."
Write-Host ""
Write-Host "Useful commands (run PowerShell as Administrator):"
if (-not $UseScheduledTask -and (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue)) {
    Write-Host "  Check status:  Get-Service $ServiceName"
    Write-Host "  View in Services: services.msc  (look for '$ServiceDisplayName')"
    Write-Host "  Stop agent:      Stop-Service $ServiceName"
    Write-Host "  Start agent:     Start-Service $ServiceName"
    Write-Host "  Remove agent:    sc.exe delete $ServiceName"
}
else {
    Write-Host "  Check task:      Get-ScheduledTask -TaskName $ServiceName"
    Write-Host "  Stop agent:      Stop-ScheduledTask -TaskName $ServiceName"
    Write-Host "  Start agent:     Start-ScheduledTask -TaskName $ServiceName"
    Write-Host "  Remove agent:    Unregister-ScheduledTask -TaskName $ServiceName -Confirm:`$false"
}
Write-Host ""
Write-Host "Test SOCKS locally (replace USER/PASS from credential files above):"
Write-Host "  curl --proxy socks5://USER:PASS@127.0.0.1:$socksPort https://api.ipify.org"
Write-Host ""
