#Requires -Version 5.1
<#
.SYNOPSIS
  Uninstall the TrinityProxy Windows agent service and remove local files.

.DESCRIPTION
  Stops the agent (allowing graceful deregister when supported), removes the
  Windows service or scheduled task, deletes firewall rules, and removes the
  install directory.

  Run in an elevated PowerShell (Run as administrator).
#>

[CmdletBinding()]
param(
    [string]$InstallDir = $(if ($env:TRINITY_INSTALL_DIR) { $env:TRINITY_INSTALL_DIR } else { Join-Path ${env:ProgramFiles} "TrinityProxy" })
)

$ErrorActionPreference = "Stop"

$ServiceName = "TrinityProxyAgent"
$ServiceDisplayName = "TrinityProxy Agent"

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

function Test-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Stop-AgentService {
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if (-not $svc) {
        return
    }

    Write-Step "Stopping TrinityProxy agent service..."
    if ($svc.Status -eq "Running") {
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 3
    }
    Write-Ok "Service stopped (graceful shutdown sends deregister when supported)"
}

function Remove-AgentService {
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Step "Removing Windows service..."
        & sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 2
        Write-Ok "Service removed"
        return
    }

    $task = Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
    if ($task) {
        Write-Step "Removing scheduled task..."
        Unregister-ScheduledTask -TaskName $ServiceName -Confirm:$false
        Write-Ok "Scheduled task removed"
    }
}

function Remove-FirewallRules {
    Write-Step "Removing TrinityProxy firewall rules..."
    $rules = Get-NetFirewallRule -DisplayName "TrinityProxy SOCKS5*" -ErrorAction SilentlyContinue
    foreach ($rule in $rules) {
        Remove-NetFirewallRule -Name $rule.Name -ErrorAction SilentlyContinue
    }
    if ($rules) {
        Write-Ok "Firewall rules removed"
    } else {
        Write-Warn "No TrinityProxy firewall rules found"
    }
}

function Remove-InstallDirectory {
    if (-not (Test-Path -LiteralPath $InstallDir)) {
        Write-Warn "Install directory not found: $InstallDir"
        return
    }

    Write-Step "Removing install directory..."
    Remove-Item -LiteralPath $InstallDir -Recurse -Force
    Write-Ok "Removed $InstallDir"
}

function Remove-UninstallRegistryEntry {
    $key = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\TrinityProxyAgent"
    if (Test-Path -LiteralPath $key) {
        Remove-Item -LiteralPath $key -Recurse -Force
    }
}

if (-not (Test-Admin)) {
    Write-Host "ERROR: Run this script as Administrator." -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "========================================" -ForegroundColor White
Write-Host "  TrinityProxy — Windows Agent Uninstall" -ForegroundColor White
Write-Host "========================================" -ForegroundColor White

Stop-AgentService
Remove-AgentService
Remove-FirewallRules
Remove-InstallDirectory
Remove-UninstallRegistryEntry

Write-Host ""
Write-Host "TrinityProxy agent uninstalled." -ForegroundColor Green
Write-Host "The controller will mark this node offline within ~2 minutes if deregister did not run." -ForegroundColor Yellow
Write-Host ""
