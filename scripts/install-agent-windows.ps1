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
    TRINITY_LOG_LEVEL=info                Install verbosity: quiet | silent | info | debug (default: info)
    TRINITY_SOCKS_PORT=10855              Explicit SOCKS port (default: auto-pick free port in 10800-10999)
    TRINITY_SOCKS_USER / TRINITY_SOCKS_PASSWORD  Override auto-generated SOCKS credentials
    TRINITY_LOCAL_BINARY=C:\path\to\trinityproxy.exe   Use a pre-built binary
    TRINITY_DOWNLOAD_URL=https://.../trinityproxy.exe  Download instead of copy
    TRINITY_INSTALL_DIR=C:\Program Files\TrinityProxy  Override install folder

  Paste-and-run: use the one-liner from the dashboard Deploy Agent page (elevated PowerShell).
  Paste-and-run downloads this script from GitHub, pulls trinityproxy-windows-amd64.exe from the latest GitHub Release (no Go), or falls back to a fresh main.zip extract under %TEMP%\TrinityProxy for local Go build.

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
    [string]$LogLevel = $env:TRINITY_LOG_LEVEL,
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
$DefaultSocksPortStart = 10800
$DefaultSocksPortEnd = 10999
# Bump when bootstrap cache under %TEMP%\TrinityProxy must be refreshed.
$ScriptVersion = "5"
# Max seconds to wait for the Windows service to reach Running after async start.
$ServiceStartTimeoutSeconds = 45
$DefaultReleaseBinaryUrl = "https://github.com/Skillz147/TrinityProxy/releases/download/latest/trinityproxy-windows-amd64.exe"

function Resolve-LogLevel([string]$Raw) {
    if (-not $Raw -or -not $Raw.Trim()) { return "info" }
    switch ($Raw.Trim().ToLower()) {
        { $_ -in @("quiet", "total-silent", "totalsilent") } { return "quiet" }
        "silent" { return "silent" }
        "debug" { return "debug" }
        "info" { return "info" }
        default {
            Write-Host "   ERROR: Invalid TRINITY_LOG_LEVEL '$Raw' (use: quiet, silent, info, debug)" -ForegroundColor Red
            exit 1
        }
    }
}

$script:LogLevel = Resolve-LogLevel -Raw $LogLevel

function Test-LogAtLeast([string]$MinLevel) {
    $order = @{ quiet = 0; silent = 1; info = 2; debug = 3 }
    return $order[$script:LogLevel] -ge $order[$MinLevel]
}

function Write-Debug([string]$Message) {
    if ($script:LogLevel -eq "debug") {
        Write-Host "   DEBUG: $Message" -ForegroundColor DarkGray
    }
}

function Write-Step([string]$Message) {
    if (-not (Test-LogAtLeast "info")) { return }
    Write-Host ""
    Write-Host ">> $Message" -ForegroundColor Cyan
}

function Write-Ok([string]$Message) {
    if (-not (Test-LogAtLeast "info")) { return }
    Write-Host "   OK: $Message" -ForegroundColor Green
}

function Write-Warn([string]$Message) {
    if ($script:LogLevel -eq "quiet") { return }
    Write-Host "   Note: $Message" -ForegroundColor Yellow
}

function Write-Fail([string]$Message) {
    Write-Host "   ERROR: $Message" -ForegroundColor Red
}

function Invoke-FileDownloadWithProgress {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Uri,
        [Parameter(Mandatory = $true)]
        [string]$OutFile
    )

    if (-not (Test-LogAtLeast "info")) {
        Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing
        return
    }

    $progressId = Get-Random -Minimum 1000 -Maximum 99999
    $sourceId = "TrinityProxyDownload$progressId"
    $wc = New-Object System.Net.WebClient
    $wc.Headers.Add("User-Agent", "TrinityProxy-Installer")
    $sub = Register-ObjectEvent -InputObject $wc -EventName DownloadProgressChanged -SourceIdentifier $sourceId -Action {
        $pct = $EventArgs.ProgressPercentage
        $receivedMB = [math]::Round($EventArgs.BytesReceived / 1MB, 2)
        $totalMB = if ($EventArgs.TotalBytesToReceive -gt 0) {
            [math]::Round($EventArgs.TotalBytesToReceive / 1MB, 2)
        } else {
            $null
        }
        $status = if ($null -ne $totalMB) { "$pct% ($receivedMB / $totalMB MB)" } else { "$pct% ($receivedMB MB received)" }
        Write-Progress -Activity "Downloading TrinityProxy agent" -Status $status -PercentComplete $pct -Id $progressId
    }

    try {
        $wc.DownloadFile($Uri, $OutFile)
    }
    finally {
        Write-Progress -Activity "Downloading TrinityProxy agent" -Completed -Id $progressId
        Unregister-Event -SourceIdentifier $sourceId -ErrorAction SilentlyContinue
        if ($sub) {
            Remove-Job -Job $sub -Force -ErrorAction SilentlyContinue
        }
        $wc.Dispose()
    }
}

function Get-AgentServiceEnvironment {
    param(
        [Parameter(Mandatory = $true)]
        [int]$Port,
        [Parameter(Mandatory = $true)]
        [string]$SocksUser,
        [Parameter(Mandatory = $true)]
        [string]$SocksPass
    )

    $envMap = [ordered]@{
        TRINITY_ROLE             = "agent"
        TRINITY_NONINTERACTIVE   = "1"
        TRINITY_SKIP_INSTALLER   = "1"
        TRINITY_SOCKS_PORT       = "$Port"
        TRINITY_SOCKS_USER       = $SocksUser
        TRINITY_SOCKS_PASSWORD   = $SocksPass
        CONTROLLER_URL           = $ControllerUrl
        TRINITY_AGENT_KEY        = $AgentKey
    }
    if ($env:TRINITY_DEVICE_CLASS) {
        $envMap.TRINITY_DEVICE_CLASS = $env:TRINITY_DEVICE_CLASS
    }
    return $envMap
}

function Set-ServiceEnvironment {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [hashtable]$Variables
    )

    $envPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$Name\Environment"
    if (Test-Path -LiteralPath $envPath) {
        Remove-Item -LiteralPath $envPath -Recurse -Force
    }
    New-Item -Path $envPath -Force | Out-Null
    foreach ($key in $Variables.Keys) {
        $value = $Variables[$key]
        if ($null -ne $value -and "$value".Length -gt 0) {
            New-ItemProperty -Path $envPath -Name $key -Value $value -PropertyType String -Force | Out-Null
        }
    }
}

function Wait-ServiceStatus {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [ValidateSet("Running", "Stopped")]
        [string]$DesiredStatus,
        [int]$TimeoutSeconds = 15,
        [string]$ProgressMessage = ""
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
        if ($svc -and $svc.Status -eq $DesiredStatus) {
            return $true
        }
        if ($ProgressMessage -and (Test-LogAtLeast "info")) {
            $remaining = [math]::Max(0, [int]($deadline - (Get-Date)).TotalSeconds)
            Write-Host "   $ProgressMessage (${remaining}s remaining)..." -ForegroundColor DarkGray
        }
        Start-Sleep -Milliseconds 500
    }
    return $false
}

function Wait-ServiceRemoved {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [int]$TimeoutSeconds = 15
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (-not (Get-Service -Name $Name -ErrorAction SilentlyContinue)) {
            return $true
        }
        Start-Sleep -Milliseconds 500
    }
    return $false
}

function Start-AgentServiceWithProgress {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [int]$TimeoutSeconds = $ServiceStartTimeoutSeconds
    )

    Write-Step "Starting the agent..."

    $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if (-not $svc) {
        throw "Service '$Name' was not found after registration."
    }
    if ($svc.Status -eq "Running") {
        Write-Ok "Agent is already running"
        return
    }

    # sc.exe start blocks until SCM's ServicesPipeTimeout when the service never reports
    # SERVICE_RUNNING (common with .cmd wrappers). WMI/CIM StartService returns immediately.
    $wmiSvc = Get-CimInstance -ClassName Win32_Service -Filter "Name='$Name'" -ErrorAction Stop
    $startResult = Invoke-CimMethod -InputObject $wmiSvc -MethodName StartService
    $returnValue = [int]$startResult.ReturnValue
    if ($returnValue -eq 10) {
        Write-Ok "Agent is already running"
        return
    }
    if ($returnValue -ne 0) {
        throw "StartService failed (Win32 error $returnValue). Check Event Viewer > Windows Logs > Application for TrinityProxy errors."
    }

    $startedAt = Get-Date
    $deadline = $startedAt.AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
        if ($svc -and $svc.Status -eq "Running") {
            Write-Ok "Agent is running in the background"
            return
        }
        if ($svc -and $svc.Status -eq "Stopped") {
            throw "Service stopped immediately after start. Run manually: `"$InstallDir\$WrapperName`" — or check Event Viewer > Application."
        }
        if (Test-LogAtLeast "info") {
            $elapsed = [int]((Get-Date) - $startedAt).TotalSeconds
            Write-Host "   Waiting for agent service... ${elapsed}s" -ForegroundColor DarkGray
        }
        Start-Sleep -Seconds 1
    }

    $final = Get-Service -Name $Name -ErrorAction SilentlyContinue
    $status = if ($final) { $final.Status } else { "missing" }
    throw "Service did not reach Running within ${TimeoutSeconds}s (status: $status). Test manually: `"$InstallDir\$WrapperName`" or use -UseScheduledTask."
}

function Write-InstallStart {
    if ($script:LogLevel -eq "quiet") { return }
    if ($script:LogLevel -eq "silent") {
        Write-Host "TrinityProxy: Windows agent install started"
        return
    }
    Write-Host ""
    Write-Host "========================================" -ForegroundColor White
    Write-Host "  TrinityProxy — Windows Agent Setup" -ForegroundColor White
    Write-Host "========================================" -ForegroundColor White
}

function Write-InstallComplete([hashtable]$Summary) {
    if ($script:LogLevel -eq "quiet") { return }
    if ($script:LogLevel -eq "silent") {
        Write-Host "TrinityProxy: setup complete (controller: $($Summary.ControllerUrl), SOCKS port $($Summary.Port))"
        return
    }
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  Setup complete!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "Your Windows PC is now reporting to:"
    Write-Host "  $($Summary.ControllerUrl)"
    Write-Host ""
    Write-Host "Embedded SOCKS5 proxy listens on TCP port $($Summary.Port)."
    Write-Host "SOCKS credentials (also saved in install folder):"
    Write-Host "  Username: $($Summary.User)"
    Write-Host "  Password: $($Summary.Pass)"
    Write-Host "  Files:    $($Summary.InstallDir)\trinityproxy-username"
    Write-Host "            $($Summary.InstallDir)\trinityproxy-password"
    Write-Host "            $($Summary.InstallDir)\trinityproxy-port"
    Write-Host ""
    Write-Host "The agent runs in the background and starts automatically when Windows boots."
    Write-Host "Open your TrinityProxy dashboard Agents page — the node should appear within about a minute."
    Write-Host ""
    Write-Host "Useful commands (run PowerShell as Administrator):"
    if (-not $Summary.UseScheduledTask -and (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue)) {
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
    Write-Host "Test SOCKS locally:"
    Write-Host "  curl --proxy socks5://$($Summary.User):$($Summary.Pass)@127.0.0.1:$($Summary.Port) https://api.ipify.org"
    Write-Host ""
}

function Test-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Test-NonInteractive {
    if ($env:TRINITY_NONINTERACTIVE -eq "1") { return $true }
    if ($script:LogLevel -eq "quiet") { return $true }
    return $false
}


function Test-InRepoScriptsDir {
    if (-not $PSScriptRoot) { return $false }
    if ((Split-Path -Leaf $PSScriptRoot) -ne "scripts") { return $false }
    $repoRoot = Split-Path -Parent $PSScriptRoot
    return (Test-Path -LiteralPath (Join-Path $repoRoot "go.mod"))
}

function Get-DefaultDownloadUrl {
    if ($env:TRINITY_DOWNLOAD_URL) {
        return $env:TRINITY_DOWNLOAD_URL.Trim()
    }
    if ($DownloadUrl) {
        return $DownloadUrl.Trim()
    }
    return $DefaultReleaseBinaryUrl
}

function Try-Get-GitHubReleaseBinaryUrl {
    $repo = if ($env:TRINITY_GITHUB_REPO) { $env:TRINITY_GITHUB_REPO.Trim() } else { "Skillz147/TrinityProxy" }
    $preferred = @(
        "trinityproxy-windows-amd64.exe",
        "trinityproxy.exe"
    )
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ "User-Agent" = "TrinityProxy-Installer" } -UseBasicParsing
        foreach ($name in $preferred) {
            foreach ($asset in $release.assets) {
                if ($asset.name -ieq $name) {
                    return $asset.browser_download_url
                }
            }
        }
        foreach ($asset in $release.assets) {
            if ($asset.name -match '(?i)^trinityproxy.*\.exe$') {
                return $asset.browser_download_url
            }
        }
    }
    catch {
        return $null
    }
    return $null
}

function Test-BootstrapCacheCurrent {
    param([string]$CloneDir)
    $installer = Join-Path $CloneDir "scripts\install-agent-windows.ps1"
    $versionFile = Join-Path $CloneDir ".install-script-version"
    if (-not (Test-Path -LiteralPath $installer)) { return $false }
    if (-not (Test-Path -LiteralPath $versionFile)) { return $false }
    try {
        $cached = (Get-Content -LiteralPath $versionFile -Raw -ErrorAction Stop).Trim()
        return ($cached -eq $ScriptVersion)
    }
    catch {
        return $false
    }
}

function Invoke-BootstrapRepoAndReenter {
    $branch = if ($env:TRINITY_REPO_BRANCH) { $env:TRINITY_REPO_BRANCH.Trim() } else { "main" }
    $zipURL = if ($env:TRINITY_REPO_ZIP_URL) { $env:TRINITY_REPO_ZIP_URL.Trim() } else { "https://github.com/Skillz147/TrinityProxy/archive/refs/heads/$branch.zip" }
    $cloneDir = Join-Path $env:TEMP "TrinityProxy"
    $installer = Join-Path $cloneDir "scripts\install-agent-windows.ps1"

    Write-Step "Preparing TrinityProxy installer (source download for Go build fallback)..."
    if (-not (Test-BootstrapCacheCurrent -CloneDir $cloneDir)) {
        if (Test-Path -LiteralPath $cloneDir) {
            Write-Warn "Removing stale %TEMP%\TrinityProxy (installer script version $ScriptVersion required)."
            Remove-Item -LiteralPath $cloneDir -Recurse -Force -ErrorAction SilentlyContinue
        }

        $zipFile = Join-Path $env:TEMP "TrinityProxy-src.zip"
        if (Test-Path -LiteralPath $zipFile) {
            Remove-Item -LiteralPath $zipFile -Force -ErrorAction SilentlyContinue
        }
        Write-Debug "Downloading source archive: $zipURL"
        if (Test-LogAtLeast "info") {
            Write-Host "   Downloading: $zipURL"
        }
        Invoke-FileDownloadWithProgress -Uri $zipURL -OutFile $zipFile
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

        New-Item -ItemType Directory -Path (Split-Path -Parent $cloneDir) -Force | Out-Null
        Move-Item -LiteralPath $extracted.FullName -Destination $cloneDir
        Set-Content -LiteralPath (Join-Path $cloneDir ".install-script-version") -Value $ScriptVersion -Encoding ASCII -NoNewline
        Remove-Item -LiteralPath $extractRoot -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $zipFile -Force -ErrorAction SilentlyContinue
    } else {
        Write-Ok "Using cached repo at $cloneDir (installer v$ScriptVersion)"
    }

    if (-not (Test-Path -LiteralPath $installer)) {
        Write-Fail "Installer script missing after download: $installer"
        exit 1
    }

    Write-Debug "Re-entering installer at $installer (log level: $script:LogLevel)"
    if ($env:TRINITY_LOG_LEVEL) { $env:TRINITY_LOG_LEVEL = $env:TRINITY_LOG_LEVEL.Trim() }
    elseif ($script:LogLevel) { $env:TRINITY_LOG_LEVEL = $script:LogLevel }
    if ($env:CONTROLLER_URL) { $env:CONTROLLER_URL = $env:CONTROLLER_URL.Trim() }
    if ($env:TRINITY_AGENT_KEY) { $env:TRINITY_AGENT_KEY = $env:TRINITY_AGENT_KEY.Trim() }
    if ($env:TRINITY_SOCKS_PORT) {
        $sp = $env:TRINITY_SOCKS_PORT.Trim()
        if ($sp -and $sp -notmatch '^\s*-') { $env:TRINITY_SOCKS_PORT = $sp }
        else { Remove-Item Env:TRINITY_SOCKS_PORT -ErrorAction SilentlyContinue }
    }
    if ($env:TRINITY_LOCAL_BINARY) { $env:TRINITY_LOCAL_BINARY = $env:TRINITY_LOCAL_BINARY.Trim() }
    if (-not $env:TRINITY_DOWNLOAD_URL) {
        $env:TRINITY_DOWNLOAD_URL = Get-DefaultDownloadUrl
    }
    if ($env:TRINITY_INSTALL_DIR) { $env:TRINITY_INSTALL_DIR = $env:TRINITY_INSTALL_DIR.Trim() }
    if ($env:TRINITY_USE_SCHEDULED_TASK -eq "1") { $env:TRINITY_USE_SCHEDULED_TASK = "1" }

    if (Test-LogAtLeast "info") {
        Write-Ok "Running installer from $cloneDir"
    }
    # Re-enter via environment only — never splat script parameters.
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


function Test-TcpPortAvailable([int]$Port) {
    $listener = $null
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Any, $Port)
        $listener.Start()
        return $true
    }
    catch {
        return $false
    }
    finally {
        if ($listener) { $listener.Stop() }
    }
}

function Find-FreeTcpPort {
    param(
        [int]$StartPort = $DefaultSocksPortStart,
        [int]$EndPort = $DefaultSocksPortEnd
    )

    for ($port = $StartPort; $port -le $EndPort; $port++) {
        if (Test-TcpPortAvailable -Port $port) {
            return $port
        }
    }

    throw "No free TCP port found in range ${StartPort}-${EndPort}"
}

function New-RandomHex([int]$ByteCount) {
    $bytes = New-Object byte[] $ByteCount
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return ([BitConverter]::ToString($bytes) -replace '-', '').ToLower()
}

function Resolve-SocksPort {
    param([string]$TargetDir)

    $raw = if ($SocksPort) { $SocksPort } else { $env:TRINITY_SOCKS_PORT }
    if ($raw -and ($raw -notmatch '^\s*-')) {
        $parsed = 0
        if ([int]::TryParse($raw.Trim(), [ref]$parsed) -and $parsed -ge 1 -and $parsed -le 65535) {
            return $parsed
        }
        Write-Warn "Ignoring invalid TRINITY_SOCKS_PORT (got: $raw)"
    }

    $portFile = Join-Path $TargetDir "trinityproxy-port"
    if (Test-Path -LiteralPath $portFile) {
        $saved = (Get-Content -LiteralPath $portFile -Raw -ErrorAction SilentlyContinue).Trim()
        $parsed = 0
        if ([int]::TryParse($saved, [ref]$parsed) -and $parsed -ge 1 -and $parsed -le 65535) {
            if (Test-TcpPortAvailable -Port $parsed) {
                return $parsed
            }
            Write-Warn "Previously assigned port $parsed is in use; selecting a new port"
        }
    }

    return Find-FreeTcpPort
}

function Ensure-SocksCredentials {
    param(
        [Parameter(Mandatory = $true)]
        [string]$TargetDir,
        [Parameter(Mandatory = $true)]
        [int]$Port
    )

    $userFile = Join-Path $TargetDir "trinityproxy-username"
    $passFile = Join-Path $TargetDir "trinityproxy-password"
    $portFile = Join-Path $TargetDir "trinityproxy-port"

    $user = $null
    $pass = $null

    if ($env:TRINITY_SOCKS_USER -and $env:TRINITY_SOCKS_PASSWORD) {
        $user = $env:TRINITY_SOCKS_USER.Trim()
        $pass = $env:TRINITY_SOCKS_PASSWORD.Trim()
    }
    elseif ((Test-Path -LiteralPath $userFile) -and (Test-Path -LiteralPath $passFile)) {
        $user = (Get-Content -LiteralPath $userFile -Raw -ErrorAction Stop).Trim()
        $pass = (Get-Content -LiteralPath $passFile -Raw -ErrorAction Stop).Trim()
    }
    else {
        $user = New-RandomHex -ByteCount 8
        $pass = New-RandomHex -ByteCount 16
    }

    if (-not $user -or -not $pass) {
        throw "Failed to resolve SOCKS credentials"
    }

    Set-Content -Path $userFile -Value $user -Encoding ASCII -NoNewline
    Set-Content -Path $passFile -Value $pass -Encoding ASCII -NoNewline
    Set-Content -Path $portFile -Value $Port -Encoding ASCII -NoNewline

    $script:SocksUser = $user
    $script:SocksPass = $pass
    return @{ User = $user; Pass = $pass }
}

function Resolve-SourceBinary {
    param([switch]$DownloadOnly)

    if ($LocalBinary -and (Test-Path -LiteralPath $LocalBinary)) {
        Write-Debug "TRINITY_LOCAL_BINARY=$LocalBinary"
        Write-Ok "Using binary from TRINITY_LOCAL_BINARY"
        return (Resolve-Path -LiteralPath $LocalBinary).Path
    }

    $repoBinary = Join-Path (Split-Path -Parent $PSScriptRoot) "build\$BinaryName"
    if (Test-Path -LiteralPath $repoBinary) {
        Write-Debug "Using repo binary at $repoBinary"
        Write-Ok "Using local build: build\$BinaryName"
        return (Resolve-Path -LiteralPath $repoBinary).Path
    }

    if (-not $DownloadUrl) {
        $DownloadUrl = Get-DefaultDownloadUrl
    }
    if ($DownloadUrl -eq $DefaultReleaseBinaryUrl) {
        $releaseUrl = Try-Get-GitHubReleaseBinaryUrl
        if ($releaseUrl) {
            $DownloadUrl = $releaseUrl
        }
    }

    if ($DownloadUrl) {
        $tempFile = Join-Path $env:TEMP "trinityproxy-download.exe"
        Write-Step "Downloading TrinityProxy agent..."
        Write-Debug "TRINITY_DOWNLOAD_URL=$DownloadUrl"
        Write-Debug "Saving to $tempFile"
        if (Test-LogAtLeast "info") {
            Write-Host "   From: $DownloadUrl"
        }
        Invoke-FileDownloadWithProgress -Uri $DownloadUrl -OutFile $tempFile
        if (-not (Test-Path -LiteralPath $tempFile)) {
            throw "Download failed — file not found after download."
        }
        Write-Ok "Download complete"
        return $tempFile
    }

    if ($DownloadOnly) {
        return $null
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
    Write-Debug "Checking firewall rule: $ruleName"
    $existing = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Debug "Existing firewall rule found (Enabled=$($existing.Enabled))"
        Write-Ok "Firewall rule already exists: $ruleName"
        return
    }

    Write-Step "Opening Windows Firewall for SOCKS port $Port..."
    Write-Debug "New-NetFirewallRule -DisplayName '$ruleName' -Direction Inbound -Protocol TCP -LocalPort $Port -Action Allow -Profile Any"
    New-NetFirewallRule `
        -DisplayName $ruleName `
        -Direction Inbound `
        -Protocol TCP `
        -LocalPort $Port `
        -Action Allow `
        -Profile Any | Out-Null
    Write-Ok "Firewall rule added for inbound TCP port $Port"
}

function Write-WrapperScript([string]$TargetDir, [int]$Port, [string]$SocksUser, [string]$SocksPass) {
    $wrapperPath = Join-Path $TargetDir $WrapperName
    $exePath = Join-Path $TargetDir $BinaryName
    $envMap = Get-AgentServiceEnvironment -Port $Port -SocksUser $SocksUser -SocksPass $SocksPass

    $lines = @(
        "@echo off",
        "rem TrinityProxy agent launcher — do not edit; re-run install-agent-windows.ps1 to update"
    )
    foreach ($key in $envMap.Keys) {
        $lines += ("set " + $key + "=" + $envMap[$key])
    }
    $lines += 'cd /d "%~dp0"'
    $lines += ('"' + $exePath + '"')

    Set-Content -Path $wrapperPath -Value ($lines -join "`r`n") -Encoding ASCII
    return $wrapperPath
}

function Install-WindowsService {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ExePath,
        [Parameter(Mandatory = $true)]
        [hashtable]$ServiceEnv
    )

    $existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Step "Updating existing Windows service..."
        if ($existing.Status -eq "Running") {
            Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
            if (-not (Wait-ServiceStatus -Name $ServiceName -DesiredStatus "Stopped" -TimeoutSeconds 15 -ProgressMessage "Stopping existing service")) {
                Write-Warn "Existing service did not stop cleanly; continuing with reinstall"
            }
        }
        & sc.exe delete $ServiceName | Out-Null
        if (-not (Wait-ServiceRemoved -Name $ServiceName -TimeoutSeconds 15)) {
            Write-Warn "Previous service entry still present; sc.exe delete may have failed"
        }
    }

    Write-Step "Registering Windows service..."
    $binArg = "`"$ExePath`""
    Write-Debug "sc.exe create $ServiceName binPath= $binArg start= auto DisplayName= $ServiceDisplayName"
    & sc.exe create $ServiceName binPath= $binArg start= auto DisplayName= $ServiceDisplayName | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "sc.exe create failed (exit $LASTEXITCODE). Try running this script as Administrator."
    }

    Set-ServiceEnvironment -Name $ServiceName -Variables $ServiceEnv
    & sc.exe description $ServiceName "TrinityProxy agent — embedded SOCKS5 proxy and controller heartbeats" | Out-Null
    & sc.exe failure $ServiceName reset= 86400 actions= restart/60000/restart/60000/restart/60000 | Out-Null
    Write-Ok "Service registered: $ServiceName (runs $BinaryName directly with registry environment)"
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

function Install-UninstallSupport([string]$TargetDir) {
    $uninstallName = "uninstall-agent-windows.ps1"
    $source = Join-Path $PSScriptRoot $uninstallName
    $dest = Join-Path $TargetDir $uninstallName
    if (Test-Path -LiteralPath $source) {
        Copy-Item -LiteralPath $source -Destination $dest -Force
    }

    $uninstallCmd = "powershell.exe -NoProfile -ExecutionPolicy Bypass -File `"$dest`""
    $key = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\TrinityProxyAgent"
    New-Item -Path $key -Force | Out-Null
    Set-ItemProperty -Path $key -Name DisplayName -Value $ServiceDisplayName
    Set-ItemProperty -Path $key -Name Publisher -Value "TrinityProxy"
    Set-ItemProperty -Path $key -Name InstallLocation -Value $TargetDir
    Set-ItemProperty -Path $key -Name UninstallString -Value $uninstallCmd
    Set-ItemProperty -Path $key -Name QuietUninstallString -Value $uninstallCmd
    Set-ItemProperty -Path $key -Name NoModify -Value 1 -Type DWord
    Set-ItemProperty -Path $key -Name NoRepair -Value 1 -Type DWord
}

function Start-AgentBackground {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WrapperPath,
        [Parameter(Mandatory = $true)]
        [string]$ExePath,
        [Parameter(Mandatory = $true)]
        [hashtable]$ServiceEnv
    )

    if ($UseScheduledTask) {
        Install-ScheduledTaskFallback -WrapperPath $WrapperPath
        return
    }

    try {
        Install-WindowsService -ExePath $ExePath -ServiceEnv $ServiceEnv
        Start-AgentServiceWithProgress -Name $ServiceName
    }
    catch {
        Write-Warn "Windows service setup failed: $($_.Exception.Message)"
        Write-Warn "Falling back to a scheduled task..."
        Install-ScheduledTaskFallback -WrapperPath $WrapperPath
    }
}

# --- Main ---

function Try-Install-StandaloneAgent {
    if (Test-InRepoScriptsDir) { return $false }

    Write-Step "Trying pre-built GitHub release binary (no Go required)..."
    $savedDownload = $DownloadUrl
    try {
        $candidate = Resolve-SourceBinary -DownloadOnly
    }
    catch {
        Write-Warn "Pre-built download not available yet ($($_.Exception.Message))"
        return $false
    }
    finally {
        $DownloadUrl = $savedDownload
    }

    if (-not $candidate -or -not (Test-Path -LiteralPath $candidate)) {
        return $false
    }

    Invoke-AgentInstallCore -SourceBinary $candidate
    return $true
}

function Invoke-AgentInstallCore {
    param(
        [Parameter(Mandatory = $true)]
        [string]$SourceBinary
    )

    Write-InstallStart

    if (-not (Test-Admin)) {
        Write-Fail "This installer must run as Administrator."
        Write-Host ""
        Write-Host "Right-click PowerShell and choose 'Run as administrator', then run this script again."
        exit 1
    }

    $nonInteractive = Test-NonInteractive
    Write-Debug "Log level=$script:LogLevel nonInteractive=$nonInteractive installDir=$InstallDir"
    Write-Debug "CONTROLLER_URL=$ControllerUrl TRINITY_SOCKS_PORT env=$($env:TRINITY_SOCKS_PORT)"
    $script:socksPort = Resolve-SocksPort -TargetDir $InstallDir
    Write-Debug "Resolved SOCKS port=$socksPort"

    if (-not $ControllerUrl) {
        if ($nonInteractive) {
            Write-Fail "CONTROLLER_URL is required when TRINITY_NONINTERACTIVE=1"
            exit 1
        }
        $script:ControllerUrl = Read-Host "Enter your controller URL (example: https://api.example.com)"
    }

    if (-not $AgentKey) {
        if ($nonInteractive) {
            Write-Fail "TRINITY_AGENT_KEY is required when TRINITY_NONINTERACTIVE=1"
            exit 1
        }
        $secure = Read-Host "Enter your agent key (from the dashboard Deploy Agent page)" -AsSecureString
        $script:AgentKey = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
            [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        )
    }

    $script:ControllerUrl = $ControllerUrl.Trim().TrimEnd("/")
    if (-not ($ControllerUrl -match "^https?://")) {
        Write-Fail "CONTROLLER_URL must start with http:// or https://"
        exit 1
    }

    Write-Step "Preparing install folder..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Write-Ok "Install folder: $InstallDir"

    Write-Step "Locating TrinityProxy binary..."
    $targetBinary = Join-Path $InstallDir $BinaryName
    Copy-Item -LiteralPath $SourceBinary -Destination $targetBinary -Force
    Write-Ok "Installed $BinaryName"

    Ensure-FirewallRule -Port $socksPort

    Write-Step "Generating SOCKS credentials and configuration..."
    $creds = Ensure-SocksCredentials -TargetDir $InstallDir -Port $socksPort
    Write-Ok "SOCKS port $socksPort (unique per agent; firewall opened)"

    Write-Step "Writing agent configuration..."
    $wrapperPath = Write-WrapperScript -TargetDir $InstallDir -Port $socksPort -SocksUser $creds.User -SocksPass $creds.Pass
    Write-Ok "Launcher script ready (TRINITY_SKIP_INSTALLER=1, TRINITY_SOCKS_PORT=$socksPort)"

    Install-UninstallSupport -TargetDir $InstallDir
    Write-Ok "Registered uninstall entry in Windows Apps & features"

    Start-AgentBackground -WrapperPath $wrapperPath -ExePath $targetBinary -ServiceEnv (Get-AgentServiceEnvironment -Port $socksPort -SocksUser $creds.User -SocksPass $creds.Pass)

    Write-InstallComplete @{
        ControllerUrl     = $ControllerUrl
        Port              = $socksPort
        User              = $creds.User
        Pass              = $creds.Pass
        InstallDir        = $InstallDir
        UseScheduledTask  = [bool]$UseScheduledTask
    }
}


if (-not (Test-InRepoScriptsDir)) {
    if (Try-Install-StandaloneAgent) { exit 0 }
    Invoke-BootstrapRepoAndReenter
}

Assert-RepoScriptLocation

$sourceBinary = Resolve-SourceBinary
Invoke-AgentInstallCore -SourceBinary $sourceBinary
