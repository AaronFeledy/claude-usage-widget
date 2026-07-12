#Requires -Version 5.1
[CmdletBinding(SupportsShouldProcess = $true)]
param()

$ErrorActionPreference = 'Stop'

$repo = 'AaronFeledy/claude-usage-widget'
$localAppData = if ($env:LOCALAPPDATA) { $env:LOCALAPPDATA } else { Join-Path ([System.IO.Path]::GetTempPath()) 'LocalAppData' }
$installDir = Join-Path $localAppData 'ClaudeUsageWidget'
$appExePath = Join-Path $installDir 'ClaudeUsageWidget.exe'
$serverExePath = Join-Path $installDir 'usage-server.exe'

$arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$runtime = if ($arch -eq 'ARM64') { 'win-arm64' } else { 'win-x64' }
$appAsset = "ClaudeUsageWidget-$runtime.exe"
$serverAsset = "usage-server-$runtime.exe"

function Get-FriendlySize {
    param([long]$Bytes)
    $sizes = 'Bytes,KB,MB,GB' -split ','
    for ($i = 0; ($Bytes -ge 1kb) -and ($i -lt $sizes.Count); $i++) { $Bytes /= 1kb }
    if ($i -eq 0) { '{0:N0}{1}' -f $Bytes, $sizes[$i] } else { '{0:N2}{1}' -f $Bytes, $sizes[$i] }
}

function Get-ReleaseAssetUrl {
    param($Release, [string]$Name)
    ($Release.assets | Where-Object { $_.name -eq $Name } | Select-Object -First 1).browser_download_url
}

function Save-Asset {
    param([string]$Url, [string]$Path, [string]$Name, [string]$Version)

    Write-Host "  Downloading $Name..."
    Add-Type -AssemblyName System.Net.Http
    $httpClient = [System.Net.Http.HttpClient]::new()
    $httpClient.DefaultRequestHeaders.Add('User-Agent', 'ClaudeUsageWidget')
    $tmpPath = Join-Path ([System.IO.Path]::GetTempPath()) "$Name-$(Get-Random).tmp"
    try {
        $response = $httpClient.GetAsync($Url, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        $response.EnsureSuccessStatusCode() | Out-Null
        $fileSize = $response.Content.Headers.ContentLength
        $fileSizeString = Get-FriendlySize $fileSize
        $inputStream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $outputStream = [System.IO.FileStream]::new($tmpPath, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write)
        try {
            $buffer = New-Object byte[] 81920
            $downloaded = 0L
            while (($read = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $outputStream.Write($buffer, 0, $read)
                $downloaded += $read
                if ($fileSize -gt 0) {
                    $pct = [Math]::Min(100, [int]($downloaded / $fileSize * 100))
                    Write-Progress -Activity "Downloading $Name $Version" -Status "$(Get-FriendlySize $downloaded)/$fileSizeString" -PercentComplete $pct
                }
            }
        } finally {
            $outputStream.Dispose()
            $inputStream.Dispose()
            Write-Progress -Activity "Downloading $Name $Version" -Completed
        }
        Move-Item -Path $tmpPath -Destination $Path -Force
    } catch {
        if (Test-Path $tmpPath) { Remove-Item $tmpPath -Force }
        throw "Download failed for ${Name}: $_"
    } finally {
        $httpClient.Dispose()
    }
}

Write-Host 'Claude Usage Widget Installer' -ForegroundColor Cyan
Write-Host "  Architecture: $runtime"
Write-Host "  App asset: $appAsset"
Write-Host "  Server asset: $serverAsset"

if ($WhatIfPreference) {
    Write-Host "  WhatIf: would install both binaries to $installDir" -ForegroundColor Yellow
    return
}

Write-Host '  Fetching latest release...' -NoNewline
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'ClaudeUsageWidget' }
$appUrl = Get-ReleaseAssetUrl $release $appAsset
$serverUrl = Get-ReleaseAssetUrl $release $serverAsset

if (-not $appUrl) {
    Write-Host ' FAILED' -ForegroundColor Red
    throw "No $appAsset found in release $($release.tag_name)"
}
if (-not $serverUrl) {
    Write-Host ' FAILED' -ForegroundColor Red
    throw "No $serverAsset found in release $($release.tag_name)"
}
$version = $release.tag_name
Write-Host " $version" -ForegroundColor Green

if ($PSCmdlet.ShouldProcess($installDir, 'Install Claude Usage Widget and usage server')) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null

    $running = Get-Process -Name 'ClaudeUsageWidget', 'usage-server' -ErrorAction SilentlyContinue
    if ($running) {
        Write-Host '  Stopping running processes...' -NoNewline
        $running | Stop-Process -Force
        Start-Sleep -Seconds 1
        Write-Host ' OK' -ForegroundColor Green
    }

    Save-Asset $appUrl $appExePath $appAsset $version
    Save-Asset $serverUrl $serverExePath $serverAsset $version
}

$startMenuPath = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Claude Usage Widget.lnk'
Write-Host '  Creating Start Menu shortcut...' -NoNewline
try {
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($startMenuPath)
    $shortcut.TargetPath = $appExePath
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Description = 'Claude Usage Widget - Monitor your Claude Max subscription usage'
    $shortcut.Save()
    [System.Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
    Write-Host ' OK' -ForegroundColor Green
} catch {
    Write-Host ' SKIP (non-critical)' -ForegroundColor Yellow
}

Start-Process $appExePath
Write-Host ''
Write-Host "  Installed $version to $installDir" -ForegroundColor Green
Write-Host '  Right-click the tray icon to configure.' -ForegroundColor DarkGray
