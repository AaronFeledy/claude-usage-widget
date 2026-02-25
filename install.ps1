#Requires -Version 5.1
# Claude Usage Widget Installer
# Usage: irm https://raw.githubusercontent.com/AaronFeledy/claude-usage-widget/main/install.ps1 | iex

$ErrorActionPreference = 'Stop'
$repo = 'AaronFeledy/claude-usage-widget'
$installDir = Join-Path $env:LOCALAPPDATA 'ClaudeUsageWidget'
$exePath = Join-Path $installDir 'ClaudeUsageWidget.exe'

# Detect architecture
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -eq 'Arm64') { 'win-arm64' } else { 'win-x64' }
$asset = "ClaudeUsageWidget-$arch.exe"

Write-Host "Claude Usage Widget Installer" -ForegroundColor Cyan
Write-Host "Architecture: $arch"

# Get latest release URL
Write-Host "Fetching latest release..." -NoNewline
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'ClaudeUsageWidget-Installer' }
$url = ($release.assets | Where-Object { $_.name -eq $asset }).browser_download_url

if (-not $url) {
    Write-Host " FAILED" -ForegroundColor Red
    Write-Error "No $asset found in release $($release.tag_name)"
    exit 1
}
Write-Host " $($release.tag_name)" -ForegroundColor Green

# Create install directory
New-Item -ItemType Directory -Path $installDir -Force | Out-Null

# Kill running instance if any
$running = Get-Process -Name 'ClaudeUsageWidget' -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "Stopping running instance..."
    $running | Stop-Process -Force
    Start-Sleep -Seconds 1
}

# Download
Write-Host "Downloading $asset..." -NoNewline
Invoke-WebRequest $url -OutFile $exePath -UseBasicParsing
Write-Host " OK" -ForegroundColor Green

# Launch
Write-Host "Launching..." -ForegroundColor Cyan
Start-Process $exePath
Write-Host "Installed to $exePath" -ForegroundColor Green
Write-Host "Right-click the tray icon > Settings to enable Start with Windows." -ForegroundColor DarkGray
