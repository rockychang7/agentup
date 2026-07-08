<#
.SYNOPSIS
    AgentUp uninstaller for Windows.
.DESCRIPTION
    Removes the agentup binary and cleans up PATH.
.EXAMPLE
    irm https://raw.githubusercontent.com/rockychang7/agentup/main/uninstall.ps1 | iex
#>

$ErrorActionPreference = "Stop"

$installDir = "$env:LOCALAPPDATA\agentup\bin"
$binary = "$installDir\agentup.exe"

Write-Host "Uninstalling agentup..." -ForegroundColor Cyan

# --- Remove from user PATH ---
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -and ($userPath -like "*$installDir*")) {
    $newPath = ($userPath -split ';' | Where-Object { $_ -and $_ -ne $installDir }) -join ';'
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = ($env:Path -split ';' | Where-Object { $_ -and $_ -ne $installDir }) -join ';'
    Write-Host "  Removed $installDir from PATH." -ForegroundColor Yellow
}

# --- Delete binary ---
if (Test-Path $binary) {
    Remove-Item $binary -Force
    Write-Host "  Removed: $binary" -ForegroundColor Green
} else {
    Write-Host "  agentup.exe not found at $binary" -ForegroundColor DarkGray
}

# --- Clean up empty directories ---
if (Test-Path $installDir) {
    $remaining = Get-ChildItem $installDir -ErrorAction SilentlyContinue
    if (-not $remaining) {
        Remove-Item $installDir -Force
        $parentDir = Split-Path $installDir -Parent
        $remaining = Get-ChildItem $parentDir -ErrorAction SilentlyContinue
        if (-not $remaining) {
            Remove-Item $parentDir -Force
        }
    }
}

Write-Host ""
Write-Host "agentup has been uninstalled." -ForegroundColor Green
Write-Host "You may need to restart your terminal for PATH changes to take effect." -ForegroundColor DarkGray
