#Requires -Version 5.1
<#
.SYNOPSIS
Claude Usage Widget Installer

.DESCRIPTION
Downloads and installs the Claude Usage Widget for Windows.
Auto-detects system architecture (x64/ARM64).

.EXAMPLE
irm https://raw.githubusercontent.com/AaronFeledy/claude-usage-widget/main/install.ps1 | iex
#>

$ErrorActionPreference = 'Stop'

$repo = 'AaronFeledy/claude-usage-widget'
$installDir = Join-Path $env:LOCALAPPDATA 'ClaudeUsageWidget'
$exePath = Join-Path $installDir 'ClaudeUsageWidget.exe'

# Detect architecture
$arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$runtime = if ($arch -eq 'ARM64') { 'win-arm64' } else { 'win-x64' }
$asset = "ClaudeUsageWidget-$runtime.exe"

Write-Host "Claude Usage Widget Installer" -ForegroundColor Cyan
Write-Host "  Architecture: $runtime"

# Get latest release
Write-Host "  Fetching latest release..." -NoNewline
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'ClaudeUsageWidget' }
$url = ($release.assets | Where-Object { $_.name -eq $asset }).browser_download_url

if (-not $url) {
    Write-Host " FAILED" -ForegroundColor Red
    throw "No $asset found in release $($release.tag_name)"
}
$version = $release.tag_name
Write-Host " $version" -ForegroundColor Green

# Create install directory
New-Item -ItemType Directory -Path $installDir -Force | Out-Null

# Kill running instance
$running = Get-Process -Name 'ClaudeUsageWidget' -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "  Stopping running instance..." -NoNewline
    $running | Stop-Process -Force
    Start-Sleep -Seconds 1
    Write-Host " OK" -ForegroundColor Green
}

# Download using HttpClient (no progress bar overhead)
Write-Host "  Downloading $asset..." -NoNewline
try {
    Add-Type -AssemblyName System.Net.Http
    $httpClient = New-Object System.Net.Http.HttpClient
    $httpClient.DefaultRequestHeaders.Add('User-Agent', 'ClaudeUsageWidget')
    $response = $httpClient.GetAsync($url).Result
    $response.EnsureSuccessStatusCode() | Out-Null
    $bytes = $response.Content.ReadAsByteArrayAsync().Result
    [System.IO.File]::WriteAllBytes($exePath, $bytes)
    $size = '{0:N1} MB' -f ($bytes.Length / 1MB)
    Write-Host " $size" -ForegroundColor Green
} catch {
    Write-Host " FAILED" -ForegroundColor Red
    throw "Download failed: $_"
} finally {
    if ($httpClient) { $httpClient.Dispose() }
}

# Launch
Start-Process $exePath
Write-Host ""
Write-Host "  Installed $version to $exePath" -ForegroundColor Green
Write-Host "  Right-click the tray icon to configure." -ForegroundColor DarkGray
