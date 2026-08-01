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

  Paste-and-run: use the one-liner from the dashboard Deploy Agent page (elevated PowerShell).
  Paste-and-run downloads this script from GitHub, extracts the repo to %TEMP%\TrinityProxy (no Git), then builds with Go if needed or uses a GitHub Release binary.

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

if (-not $UseScheduledTask -and $env:TRINITY_USE_SCHEDULED_TASK -eq "1") {
    $UseScheduledTask = $true
}

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


function Test-InRepoScriptsDir {
    if (-not $PSScriptRoot) { return $false }
    if ((Split-Path -Leaf $PSScriptRoot) -ne "scripts") { return $false }
    $repoRoot = Split-Path -Parent $PSScriptRoot
    return (Test-Path -LiteralPath (Join-Path $repoRoot "go.mod"))
}

function Try-Get-GitHubReleaseBinaryUrl {
    $repo = if ($env:TRINITY_GITHUB_REPO) { $env:TRINITY_GITHUB_REPO.Trim() } else { "Skillz147/TrinityProxy" }
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ "User-Agent" = "TrinityProxy-Installer" } -UseBasicParsing
        foreach ($asset in $release.assets) {
            if ($asset.name -match '(?i)trinityproxy.*\.exe$') {
                return $asset.browser_download_url
            }
        }
    }
    catch {
        return $null
    }
    return $null
}

function Invoke-BootstrapRepoAndReenter {
    $branch = if ($env:TRINITY_REPO_BRANCH) { $env:TRINITY_REPO_BRANCH.Trim() } else { "main" }
    $zipURL = if ($env:TRINITY_REPO_ZIP_URL) { $env:TRINITY_REPO_ZIP_URL.Trim() } else { "https://github.com/Skillz147/TrinityProxy/archive/refs/heads/$branch.zip" }
    $cloneDir = Join-Path $env:TEMP "TrinityProxy"
    $installer = Join-Path $cloneDir "scripts\install-agent-windows.ps1"

    Write-Step "Preparing TrinityProxy installer (one-time download)..."
    if (-not (Test-Path -LiteralPath $installer)) {
        $zipFile = Join-Path $env:TEMP "TrinityProxy-src.zip"
        Write-Host "   Downloading: $zipURL"
        Invoke-WebRequest -Uri $zipURL -OutFile $zipFile -UseBasicParsing
        if (-not (Test-Path -LiteralPath $zipFile)) {
            Write-Fail "Download failed — archive not found."
            exit 1
        }

        $extractRoot = Join-Path $env:TEMP "TrinityProxy-extract"
        if (Test-Path -LiteralPath $extractRoot) {
            Remove-Item -LiteralPath $extractRoot -Recurse -Force
        }
        New-Item -ItemType Directory -Path $extractRoot -Force | Out-Null
        Expand-Archive -LiteralPath $zipFile -DestinationPath $extractRoot -Force

        $extracted = Get-ChildItem -LiteralPath $extractRoot -Directory | Select-Object -First 1
        if (-not $extracted) {
            Write-Fail "Could not find extracted folder in archive."
            exit 1
        }

        if (Test-Path -LiteralPath $cloneDir) {
            Remove-Item -LiteralPath $cloneDir -Recurse -Force
        }
        Move-Item -LiteralPath $extracted.FullName -Destination $cloneDir
        Remove-Item -LiteralPath $extractRoot -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $zipFile -Force -ErrorAction SilentlyContinue
    }

    if (-not (Test-Path -LiteralPath $installer)) {
        Write-Fail "Installer script missing after download: $installer"
        exit 1
    }

    Write-Ok "Running installer from $cloneDir"
    # Re-enter via environment only — avoids PowerShell mis-binding (e.g. -AgentKey -> SocksPort).
    if ($ControllerUrl) { $env:CONTROLLER_URL = $ControllerUrl.Trim() }
    if ($AgentKey) { $env:TRINITY_AGENT_KEY = $AgentKey }
    if ($SocksPort) { $env:TRINITY_SOCKS_PORT = $SocksPort }
    if ($LocalBinary) { $env:TRINITY_LOCAL_BINARY = $LocalBinary }
    if ($DownloadUrl) { $env:TRINITY_DOWNLOAD_URL = $DownloadUrl }
    if ($InstallDir) { $env:TRINITY_INSTALL_DIR = $InstallDir }
    if ($UseScheduledTask) { $env:TRINITY_USE_SCHEDULED_TASK = "1" }

    & $installer
    exit $LASTEXITCODE
}

function Try-Build-WindowsBinary {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    if (-not (Test-Path -LiteralPath (Join-Path $repoRoot "go.mod"))) {
        return $null
    }

    $outDir = Join-Path $repoRoot "build"
    $out = Join-Path $outDir $BinaryName
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) {
        return $null
    }

    Write-Step "Building $BinaryName with Go (windows/amd64 — may take a few minutes)..."
    New-Item -ItemType Directory -Path $outDir -Force | Out-Null
    Push-Location $repoRoot
    try {
        $prevGOOS = $env:GOOS
        $prevGOARCH = $env:GOARCH
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        & go build -o $out .
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed (exit $LASTEXITCODE)"
        }
    }
    finally {
        if ($null -eq $prevGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $prevGOOS }
        if ($null -eq $prevGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $prevGOARCH }
        Pop-Location
    }

    if (Test-Path -LiteralPath $out) {
        Write-Ok "Built $BinaryName"
        return (Resolve-Path -LiteralPath $out).Path
    }
    return $null
}

function Assert-RepoScriptLocation {
    $scriptName = "install-agent-windows.ps1"
    if ((Split-Path -Leaf $PSScriptRoot) -ne "scripts") {
        Write-Fail "Run this installer from the TrinityProxy repo (scripts folder missing from path)."
        Write-Host ""
        Write-Host "Expected script path: ...\TrinityProxy\scripts\$scriptName"
        Write-Host "Actual script path:   $PSCommandPath"
        Write-Host "Current directory:    $(Get-Location)"
        Write-Host ""
        Write-Host "Use the one-liner from the dashboard Deploy Agent page (elevated PowerShell), or set TRINITY_DOWNLOAD_URL / TRINITY_LOCAL_BINARY when running a copy of this script."
        exit 1
    }

    $repoRoot = Split-Path -Parent $PSScriptRoot
    $expectedScript = Join-Path $PSScriptRoot $scriptName
    if (-not (Test-Path -LiteralPath $expectedScript)) {
        Write-Fail "Could not verify installer path."
        Write-Host "Expected: $expectedScript"
        exit 1
    }

    $looksLikeRepo = (Test-Path -LiteralPath (Join-Path $repoRoot "go.mod")) -or (Test-Path -LiteralPath (Join-Path $repoRoot "README.md"))
    $hasBinarySource = $env:TRINITY_DOWNLOAD_URL -or $env:TRINITY_LOCAL_BINARY -or (Test-Path -LiteralPath (Join-Path $repoRoot "build\$BinaryName"))
    if (-not $looksLikeRepo -and -not $hasBinarySource) {
        Write-Fail "Current folder does not look like the TrinityProxy repo root."
        Write-Host ""
        Write-Host "Repo root (parent of scripts): $repoRoot"
        Write-Host "Current directory:             $(Get-Location)"
        Write-Host ""
        Write-Host "cd into the cloned TrinityProxy folder, then run:"
        Write-Host "  .\scripts\$scriptName"
        exit 1
    }
}


function Resolve-SocksPort {
    $raw = if ($SocksPort) { $SocksPort } else { $env:TRINITY_SOCKS_PORT }
    if (-not $raw -or ($raw -match '^\s*-')) {
        return $DefaultSocksPort
    }
    $parsed = 0
    if ([int]::TryParse($raw.Trim(), [ref]$parsed) -and $parsed -ge 1 -and $parsed -le 65535) {
        return $parsed
    }
    Write-Warn "Ignoring invalid TRINITY_SOCKS_PORT (got: $raw); using $DefaultSocksPort"
    return $DefaultSocksPort
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

    if (-not $DownloadUrl) {
        $releaseUrl = Try-Get-GitHubReleaseBinaryUrl
        if ($releaseUrl) {
            $DownloadUrl = $releaseUrl
        }
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

    $built = Try-Build-WindowsBinary
    if ($built) {
        return $built
    }

    throw @"
Could not find trinityproxy.exe.

Options:
  - Install Go on this PC and re-run (the installer will build automatically), or
  - Build on another machine: make build-windows-agent, then set TRINITY_LOCAL_BINARY or TRINITY_DOWNLOAD_URL, or
  - Place build\trinityproxy.exe in the cloned repo under build\
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

if (-not (Test-InRepoScriptsDir)) {
    Invoke-BootstrapRepoAndReenter
}

Assert-RepoScriptLocation

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
