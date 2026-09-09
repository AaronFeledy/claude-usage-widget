#Requires -Version 5.1
[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$PackagePath,
    [string]$ReleaseManifestPath,
    [string]$InstallRoot,
    [string]$EntryPath,
    [switch]$NoLaunch
)

$ErrorActionPreference = 'Stop'
$repo = 'AaronFeledy/claude-usage-widget'
$localAppData = if ($env:LOCALAPPDATA) { $env:LOCALAPPDATA } else { throw 'LOCALAPPDATA is not defined.' }
if (-not $InstallRoot) { $InstallRoot = Join-Path $localAppData 'Headroom' }
if (-not $EntryPath) { $EntryPath = Join-Path $InstallRoot 'headroom.exe' }
$osArchitecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
$contractArchitecture = if ($osArchitecture -eq 'arm64') { 'arm64' } elseif ($osArchitecture -eq 'x64') { 'x86_64' } else { throw "Headroom does not support Windows $osArchitecture." }
$assetArchitecture = if ($contractArchitecture -eq 'arm64') { 'arm64' } else { 'x64' }
$allowedDownloadHosts = @('api.github.com', 'github.com', 'github-releases.githubusercontent.com', 'objects.githubusercontent.com', 'release-assets.githubusercontent.com')

function Save-HeadroomFile {
    param([Parameter(Mandatory)][uri]$Uri, [Parameter(Mandatory)][string]$Destination, [long]$MaxBytes = 8MB, [int]$TimeoutSeconds = 120)
    Add-Type -AssemblyName System.Net.Http
    $current = $Uri
    for ($redirects = 0; $redirects -le 5; $redirects++) {
        if ($current.Scheme -ne 'https' -or $current.Port -ne 443 -or $current.UserInfo -or $allowedDownloadHosts -notcontains $current.DnsSafeHost.ToLowerInvariant()) { throw "Refusing untrusted download URL: $current" }
        $handler = [System.Net.Http.HttpClientHandler]::new()
        $handler.AllowAutoRedirect = $false
        $client = [System.Net.Http.HttpClient]::new($handler)
        $client.DefaultRequestHeaders.Add('User-Agent', 'Headroom-Installer')
        $response = $null
        $cancel = $null
        try {
            $cancel = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds($TimeoutSeconds))
            $response = $client.GetAsync($current, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead, $cancel.Token).GetAwaiter().GetResult()
            if ([int]$response.StatusCode -ge 300 -and [int]$response.StatusCode -lt 400) {
                if (-not $response.Headers.Location) { throw "Download redirect has no location: $current" }
                $current = [uri]::new($current, $response.Headers.Location)
                continue
            }
            $response.EnsureSuccessStatusCode() | Out-Null
            if ($response.Content.Headers.ContentLength -and $response.Content.Headers.ContentLength -gt $MaxBytes) { throw "Download exceeds the $MaxBytes byte limit." }
            $input = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
            $output = [System.IO.File]::Open($Destination, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
            try {
                $buffer = New-Object byte[] 81920
                $written = 0L
                while (($read = $input.ReadAsync($buffer, 0, $buffer.Length, $cancel.Token).GetAwaiter().GetResult()) -gt 0) {
                    $written += $read
                    if ($written -gt $MaxBytes) { throw "Download exceeds the $MaxBytes byte limit." }
                    $output.Write($buffer, 0, $read)
                }
            } finally { $output.Dispose(); $input.Dispose() }
            return
        } finally {
            if ($response) { $response.Dispose() }
            if ($cancel) { $cancel.Dispose() }
            $client.Dispose()
            $handler.Dispose()
        }
    }
    throw 'Too many download redirects.'
}

function Read-ReleaseManifest {
    param([string]$Path)
    if ((Get-Item -LiteralPath $Path).Length -gt 4MB) { throw 'Release manifest is too large.' }
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    return ([System.Text.Encoding]::UTF8.GetString($bytes) | ConvertFrom-Json)
}

function Test-HeadroomVersion {
    param([string]$Version)
    if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$') { return $false }
    $withoutBuild = $Version.Split('+')[0]
    $dash = $withoutBuild.IndexOf('-')
    if ($dash -ge 0) {
        foreach ($identifier in $withoutBuild.Substring($dash + 1).Split('.')) {
            if ($identifier -match '^[0-9]+$' -and $identifier.Length -gt 1 -and $identifier[0] -eq '0') { return $false }
        }
    }
    return $true
}

Write-Host "Headroom installer ($assetArchitecture)" -ForegroundColor Cyan
if ($WhatIfPreference) {
    Write-Host "WhatIf: would validate and install a complete package at $InstallRoot with stable entry $EntryPath"
    return
}

$privateRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('.headroom-install-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $privateRoot | Out-Null
try {
    $localManifest = Join-Path $privateRoot 'release.json'
    if ($ReleaseManifestPath) {
        if ((Get-Item -LiteralPath $ReleaseManifestPath).Length -gt 4MB) { throw 'Release manifest is too large.' }
        Copy-Item -LiteralPath $ReleaseManifestPath -Destination $localManifest
    } elseif ($PackagePath) {
        throw '-ReleaseManifestPath is required with -PackagePath.'
    } else {
        $releasePath = Join-Path $privateRoot 'github-release.json'
        Save-HeadroomFile ([uri]"https://api.github.com/repos/$repo/releases/latest") $releasePath
        $release = Get-Content -LiteralPath $releasePath -Raw | ConvertFrom-Json
        if ([string]$release.tag_name -notmatch '^v(.+)$') { throw 'Latest release tag is not a Headroom version tag.' }
        $releaseVersion = $Matches[1]
        $releaseName = "Headroom-v$releaseVersion-release.json"
        $releaseAsset = @($release.assets | Where-Object { $_.name -ceq $releaseName })
        if ($releaseAsset.Count -ne 1) { throw "Release must contain exactly one $releaseName asset." }
        Save-HeadroomFile ([uri]$releaseAsset[0].browser_download_url) $localManifest 4MB
    }
    $manifest = Read-ReleaseManifest $localManifest
    if ($manifest.schema -ne 1 -or $manifest.product -cne 'Headroom' -or !(Test-HeadroomVersion ([string]$manifest.version))) { throw 'Unrecognized release manifest.' }
    if ($releaseVersion -and $manifest.version -cne $releaseVersion) { throw 'Release tag and release manifest versions do not match.' }
    $assetName = "Headroom-v$($manifest.version)-windows-$assetArchitecture.zip"
    $localArchive = Join-Path $privateRoot $assetName
    $packages = @($manifest.packages | Where-Object { $_.platform -ceq 'windows' -and $_.architecture -ceq $contractArchitecture -and $_.asset_name -ceq $assetName })
    if ($packages.Count -ne 1) { throw "Release manifest does not contain exactly one $assetName package." }
    $package = $packages[0]
    if ($package.size -isnot [int64] -and $package.size -isnot [int32]) { throw 'Package size must be an integer.' }
    if ([int64]$package.size -le 0 -or [int64]$package.size -gt 2GB) { throw 'Package size exceeds the installer limit.' }
    if ($PackagePath) {
        if ((Get-Item -LiteralPath $PackagePath).Length -ne [int64]$package.size) { throw 'Package size does not match the release manifest.' }
        Copy-Item -LiteralPath $PackagePath -Destination $localArchive
    } else {
        $releaseAsset = @($release.assets | Where-Object { $_.name -ceq $assetName })
        if ($releaseAsset.Count -ne 1) { throw "Release must contain exactly one $assetName asset." }
        Save-HeadroomFile ([uri]$releaseAsset[0].browser_download_url) $localArchive ([int64]$package.size) 600
    }
    $archiveInfo = Get-Item -LiteralPath $localArchive
    if ($archiveInfo.Length -ne [int64]$package.size) { throw 'Package size does not match the release manifest.' }
    $archiveHash = (Get-FileHash -LiteralPath $localArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($archiveHash -cne ([string]$package.sha256).ToLowerInvariant()) { throw 'Package hash does not match the release manifest.' }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [System.IO.Compression.ZipFile]::OpenRead($localArchive)
    try {
        $rootName = [System.IO.Path]::GetFileNameWithoutExtension($assetName)
        $managerEntryName = "$rootName/bootstrap/headroom-package.exe"
        $managerEntries = @($zip.Entries | Where-Object { $_.FullName -ceq $managerEntryName })
        if ($managerEntries.Count -ne 1 -or $managerEntries[0].Length -le 0 -or $managerEntries[0].Length -gt 64MB) { throw 'Package bootstrap entry is missing or oversized.' }
        $managerPath = Join-Path $privateRoot 'headroom-package.exe'
        $input = $managerEntries[0].Open()
        $output = [System.IO.File]::Open($managerPath, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
        try {
            $buffer = New-Object byte[] 81920
            $written = 0L
            while (($read = $input.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $written += $read
                if ($written -gt 64MB -or $written -gt [int64]$managerEntries[0].Length) { throw 'Package bootstrap entry exceeded its declared size.' }
                $output.Write($buffer, 0, $read)
            }
            if ($written -ne [int64]$managerEntries[0].Length) { throw 'Package bootstrap entry was truncated.' }
        } finally { $output.Dispose(); $input.Dispose() }
    } finally { $zip.Dispose() }

    if ($null -eq $PSCmdlet -or $PSCmdlet.ShouldProcess($InstallRoot, 'Install verified Headroom package')) {
        $legacyExecutable = Join-Path $localAppData 'ClaudeUsageWidget\ClaudeUsageWidget.exe'
        Get-Process -Name 'ClaudeUsageWidget' -ErrorAction SilentlyContinue | Where-Object {
            try { [System.IO.Path]::GetFullPath($_.Path) -eq [System.IO.Path]::GetFullPath($legacyExecutable) } catch { $false }
        } | Stop-Process -Force
        & $managerPath install --archive $localArchive --install-root $InstallRoot --entry-path $EntryPath --version $manifest.version --platform windows --arch $contractArchitecture --asset $assetName
        if ($LASTEXITCODE -ne 0) { throw "Headroom package manager failed with exit code $LASTEXITCODE." }
        $programs = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
        New-Item -ItemType Directory -Force -Path $programs | Out-Null
        $shell = New-Object -ComObject WScript.Shell
        try {
            $shortcut = $shell.CreateShortcut((Join-Path $programs 'Headroom.lnk'))
            $shortcut.TargetPath = $EntryPath
            $shortcut.WorkingDirectory = $InstallRoot
            $shortcut.Description = 'Headroom usage monitor'
            $shortcut.Save()
            $legacyShortcutPath = Join-Path $programs 'Claude Usage Widget.lnk'
            if (Test-Path -LiteralPath $legacyShortcutPath) {
                $legacy = $shell.CreateShortcut($legacyShortcutPath)
                $legacyTarget = Join-Path $localAppData 'ClaudeUsageWidget\ClaudeUsageWidget.exe'
                if ([System.IO.Path]::GetFullPath($legacy.TargetPath) -eq [System.IO.Path]::GetFullPath($legacyTarget)) { Remove-Item -LiteralPath $legacyShortcutPath }
            }
        } finally { [void][System.Runtime.InteropServices.Marshal]::ReleaseComObject($shell) }
        Write-Host "Installed Headroom $($manifest.version) at $InstallRoot"
        if ($NoLaunch) {
            Write-Host "Quit any running Headroom window and launch $EntryPath to use the installed generation."
        } else {
            $readyPath = Join-Path $privateRoot 'installed-ready.json'
            $nonce = ([guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')).Substring(0, 48)
            $priorNonce = [Environment]::GetEnvironmentVariable('HEADROOM_READY_NONCE', 'Process')
            try {
                [Environment]::SetEnvironmentVariable('HEADROOM_READY_NONCE', $nonce, 'Process')
                $quotedReady = '"' + $readyPath.Replace('"', '\"') + '"'
                Start-Process -FilePath $EntryPath -ArgumentList @('--headroom-installed-restart', '--headroom-ready-file', $quotedReady)
            } finally {
                [Environment]::SetEnvironmentVariable('HEADROOM_READY_NONCE', $priorNonce, 'Process')
            }
            $deadline = [DateTime]::UtcNow.AddSeconds(10)
            $ready = $null
            while ([DateTime]::UtcNow -lt $deadline) {
                if ((Test-Path -LiteralPath $readyPath) -and (Get-Item -LiteralPath $readyPath).Length -gt 0) {
                    try { $ready = Get-Content -LiteralPath $readyPath -Raw | ConvertFrom-Json } catch { $ready = $null }
                    if ($ready) { break }
                }
                Start-Sleep -Milliseconds 100
            }
            $state = Get-Content -LiteralPath (Join-Path $InstallRoot 'install-state.json') -Raw | ConvertFrom-Json
            $expectedExecutable = [System.IO.Path]::GetFullPath((Join-Path $InstallRoot (Join-Path $state.version_path 'bin\headroom.exe')))
            $readyMatches = $ready -and $ready.nonce -ceq $nonce -and $ready.version -ceq $state.active_version -and
                [int64]$ready.pid -gt 0 -and [System.IO.Path]::GetFullPath([string]$ready.executable) -ieq $expectedExecutable
            if ($readyMatches) { Write-Host 'Started the verified installed generation.' }
            else { Write-Warning "The installation is ready, but the new generation did not become active. Quit any running Headroom window and launch $EntryPath." }
        }
    }
} finally {
    if (Test-Path -LiteralPath $privateRoot) { Remove-Item -LiteralPath $privateRoot -Recurse -Force }
}
