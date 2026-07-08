<#
.SYNOPSIS
    AgentUp installer for Windows.
.DESCRIPTION
    Downloads and installs the latest agentup binary from GitHub Releases.
    The binary is placed in a directory that's already in your PATH.
.PARAMETER Owner
    GitHub repository owner. Default: rockychang7
.PARAMETER Repo
    GitHub repository name. Default: agentup
.EXAMPLE
    irm https://raw.githubusercontent.com/rockychang7/agentup/main/install.ps1 | iex
#>

param(
    [string]$Owner = "rockychang7",
    [string]$Repo = "agentup"
)

$ErrorActionPreference = "Stop"

# --- Detect architecture ---
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "amd64" }

# --- Fetch latest release info ---
$apiUrl = "https://api.github.com/repos/$Owner/$Repo/releases/latest"
Write-Host "Fetching latest release from $Owner/$Repo..." -ForegroundColor Cyan
$release = Invoke-RestMethod -Uri $apiUrl -Headers @{ "User-Agent" = "agentup-installer" }

# --- Find matching asset ---
# Expected pattern: agentup_<version>_windows_amd64.zip
$pattern = "windows_amd64\.(zip|tar\.gz)$"
$asset = $release.assets | Where-Object { $_.name -match $pattern } | Select-Object -First 1

if (-not $asset) {
    Write-Host "ERROR: No Windows release asset found." -ForegroundColor Red
    Write-Host "Available assets:"
    $release.assets | ForEach-Object { Write-Host "  - $($_.name)" }
    exit 1
}

Write-Host "Found asset: $($asset.name)" -ForegroundColor Green

# --- Download ---
$tempDir = New-Item -ItemType Directory -Force -Path "$env:TEMP\agentup-install-$(Get-Random)"
$downloadPath = Join-Path $tempDir.FullName $asset.name

Write-Host "Downloading..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $downloadPath -UseBasicParsing

# --- Extract ---
$extractDir = Join-Path $tempDir.FullName "extracted"
New-Item -ItemType Directory -Force -Path $extractDir | Out-Null

if ($asset.name -match "\.zip$") {
    Expand-Archive -Path $downloadPath -DestinationPath $extractDir -Force
} else {
    tar -xzf $downloadPath -C $extractDir
}

# --- Find the binary ---
$binary = Get-ChildItem -Path $extractDir -Recurse -Filter "agentup.exe" | Select-Object -First 1
if (-not $binary) {
    Write-Host "ERROR: agentup.exe not found in archive." -ForegroundColor Red
    exit 1
}

# --- Install to a directory in PATH ---
$installDir = "$env:LOCALAPPDATA\agentup\bin"
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

Copy-Item $binary.FullName "$installDir\agentup.exe" -Force

# --- Add to user PATH if not already there ---
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    $env:Path += ";$installDir"
    Write-Host "Added $installDir to your PATH." -ForegroundColor Yellow
    Write-Host "Note: You may need to restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
}

# --- Cleanup ---
Remove-Item -Path $tempDir.FullName -Recurse -Force

# --- Verify ---
Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host "  Version: $($release.tag_name)" -ForegroundColor Gray
Write-Host "  Location: $installDir\agentup.exe" -ForegroundColor Gray
Write-Host ""
& "$installDir\agentup.exe" version
