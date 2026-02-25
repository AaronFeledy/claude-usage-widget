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

function Get-FriendlySize {
    param($Bytes)
    $sizes = 'Bytes,KB,MB,GB' -split ','
    for ($i = 0; ($Bytes -ge 1kb) -and ($i -lt $sizes.Count); $i++) { $Bytes /= 1kb }
    if ($i -eq 0) { '{0:N0}{1}' -f $Bytes, $sizes[$i] } else { '{0:N2}{1}' -f $Bytes, $sizes[$i] }
}

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

# Download with streaming progress bar (same approach as Lando installer)
$tmpPath = Join-Path $env:TEMP "ClaudeUsageWidget-$(Get-Random).exe"
Write-Host "  Downloading $asset..."

# Customize progress bar colors
$Host.PrivateData.ProgressForegroundColor = 'White'
$Host.PrivateData.ProgressBackgroundColor = 'DarkCyan'

try {
    Add-Type -AssemblyName System.Net.Http
    $httpClient = New-Object System.Net.Http.HttpClient
    $httpClient.DefaultRequestHeaders.Add('User-Agent', 'ClaudeUsageWidget')
    $httpCompletionOption = [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead
    $response = $httpClient.GetAsync($url, $httpCompletionOption)

    Write-Progress -Activity "Downloading Claude Usage Widget $version" -Status 'Starting download...' -PercentComplete 0
    $response.Wait()
    $response.Result.EnsureSuccessStatusCode() | Out-Null

    $fileSize = $response.Result.Content.Headers.ContentLength
    $fileSizeString = Get-FriendlySize $fileSize

    $outputStream = [System.IO.FileStream]::new($tmpPath, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write)
    $downloadTask = $response.Result.Content.CopyToAsync($outputStream)

    $previousSize = 0
    $byteChange = @()
    $sleepTime = 500

    while (-not $downloadTask.IsCompleted) {
        Start-Sleep -Milliseconds $sleepTime

        # Calculate download speed
        $downloaded = $outputStream.Position
        $byteChange += ($downloaded - $previousSize)
        $previousSize = $downloaded
        if ($byteChange.Count -gt (1000 / $sleepTime)) {
            $byteChange = $byteChange | Select-Object -Last (1000 / $sleepTime)
        }
        $averageSpeed = $byteChange | Measure-Object -Average | Select-Object -ExpandProperty Average
        $speedString = Get-FriendlySize ($averageSpeed * (1000 / $sleepTime))

        $pct = [Math]::Min(100, [int]($downloaded / $fileSize * 100))
        Write-Progress -Activity "Downloading Claude Usage Widget $version" -Status "$(Get-FriendlySize $downloaded)/$fileSizeString (${speedString}/s)" -PercentComplete $pct
    }

    Write-Progress -Activity "Downloading Claude Usage Widget $version" -Status 'Download complete' -PercentComplete 100
    Start-Sleep -Milliseconds 200
    Write-Progress -Activity "Downloading Claude Usage Widget $version" -Completed
    $downloadTask.Dispose()
    $outputStream.Close()

    # Move to install location
    Move-Item -Path $tmpPath -Destination $exePath -Force

} catch {
    Write-Progress -Activity "Downloading Claude Usage Widget $version" -Completed
    if (Test-Path $tmpPath) { Remove-Item $tmpPath -Force }
    throw "Download failed: $_"
} finally {
    if ($httpClient) { $httpClient.Dispose() }
}

# Create Start Menu shortcut
$startMenuPath = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Claude Usage Widget.lnk'
Write-Host "  Creating Start Menu shortcut..." -NoNewline
try {
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($startMenuPath)
    $shortcut.TargetPath = $exePath
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Description = 'Claude Usage Widget - Monitor your Claude Max subscription usage'
    $shortcut.Save()
    [System.Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
    Write-Host " OK" -ForegroundColor Green
} catch {
    Write-Host " SKIP (non-critical)" -ForegroundColor Yellow
}

# Launch
Start-Process $exePath
Write-Host ""
Write-Host "  Installed $version to $exePath" -ForegroundColor Green
Write-Host "  Right-click the tray icon to configure." -ForegroundColor DarkGray
