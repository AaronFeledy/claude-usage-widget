[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][string]$Architecture,
    [Parameter(Mandatory)][string]$BuildDir,
    [Parameter(Mandatory)][string]$WorkDir,
    [Parameter(Mandatory)][string]$OutputDir,
    [Parameter(Mandatory)][string]$Server,
    [Parameter(Mandatory)][string]$CredentialHelper,
    [Parameter(Mandatory)][string]$Launcher,
    [Parameter(Mandatory)][string]$Manager,
    [Parameter(Mandatory)][string]$QtRoot
)
$ErrorActionPreference = 'Stop'
$assetArch = if ($Architecture -eq 'x86_64') { 'x64' } elseif ($Architecture -eq 'arm64') { 'arm64' } else { throw "Unsupported architecture: $Architecture" }
& $Manager asset-name --version $Version --platform windows --arch $Architecture | Out-Null
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$asset = "Headroom-v$Version-windows-$assetArch.zip"
$packageRoot = Join-Path $WorkDir "Headroom-v$Version-windows-$assetArch"
if (Test-Path $packageRoot) { Remove-Item -Recurse -Force $packageRoot }
New-Item -ItemType Directory -Force (Join-Path $packageRoot 'bundle'), (Join-Path $packageRoot 'bootstrap'), $OutputDir | Out-Null
cmake --install $BuildDir --prefix (Join-Path $packageRoot 'bundle')
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$platformDestination = Join-Path $packageRoot 'bundle/plugins/platforms'
New-Item -ItemType Directory -Force $platformDestination | Out-Null
foreach ($pluginName in @('qwindows.dll', 'qoffscreen.dll')) {
    $plugin = @(Get-ChildItem $QtRoot -Recurse -File -Filter $pluginName | Where-Object { $_.Directory.Name -eq 'platforms' })
    if ($plugin.Count -ne 1) { throw "Expected one Qt platform plugin $pluginName, found $($plugin.Count)" }
    Copy-Item $plugin[0].FullName (Join-Path $platformDestination $pluginName) -Force
}
Copy-Item $Server (Join-Path $packageRoot 'bundle/bin/usage-server.exe')
Copy-Item $CredentialHelper (Join-Path $packageRoot 'bundle/bin/headroom-credential-helper.exe')
Copy-Item $Launcher (Join-Path $packageRoot 'bootstrap/headroom.exe')
Copy-Item $Manager (Join-Path $packageRoot 'bootstrap/headroom-package.exe')
Copy-Item $Manager (Join-Path $packageRoot 'bundle/bin/headroom-package.exe')
Copy-Item packaging/THIRD_PARTY_NOTICES.txt (Join-Path $packageRoot 'bundle/share/headroom/THIRD_PARTY_NOTICES.txt')
New-Item -ItemType Directory -Force (Join-Path $packageRoot 'bundle/share/licenses/headroom'), (Join-Path $packageRoot 'bundle/share/licenses/qt'), (Join-Path $packageRoot 'bundle/share/licenses/dotnet'), (Join-Path $packageRoot 'bundle/share/licenses/sqlite'), (Join-Path $packageRoot 'bundle/share/licenses/msvc'), (Join-Path $packageRoot 'bundle/share/licenses/go') | Out-Null
Copy-Item LICENSE (Join-Path $packageRoot 'bundle/share/licenses/headroom/LICENSE')
$qtLicenses = @(Get-ChildItem $QtRoot -Recurse -Depth 3 -File -Include 'LICENSE*','*NOTICE*')
if ($qtLicenses.Count -eq 0) { throw "Qt license inventory not found under $QtRoot" }
$qtLicenses | Copy-Item -Destination (Join-Path $packageRoot 'bundle/share/licenses/qt')
$dotnetRoot = Split-Path (Get-Command dotnet).Source
$dotnetLicenses = @(Get-ChildItem $dotnetRoot -Recurse -Depth 3 -File -Include 'LICENSE*','*NOTICE*')
if ($dotnetLicenses.Count -eq 0) { throw "dotnet license inventory not found under $dotnetRoot" }
$dotnetLicenses | Copy-Item -Destination (Join-Path $packageRoot 'bundle/share/licenses/dotnet')
Copy-Item (Join-Path (go env GOROOT) 'LICENSE') (Join-Path $packageRoot 'bundle/share/licenses/go/LICENSE')
'SQLite is in the public domain. https://sqlite.org/copyright.html' | Set-Content -Encoding UTF8 (Join-Path $packageRoot 'bundle/share/licenses/sqlite/NOTICE.txt')
'Microsoft Visual C++ runtime files are redistributed under the Visual Studio license.' | Set-Content -Encoding UTF8 (Join-Path $packageRoot 'bundle/share/licenses/msvc/NOTICE.txt')
& $Manager create-package --root $packageRoot --output (Join-Path $OutputDir $asset) --version $Version --platform windows --arch $Architecture --qt-version 6.8.3 --baseline $(if ($Architecture -eq 'arm64') { 'windows-11-arm64-msvc2022' } else { 'windows-10-1809-x64-msvc2022' })
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& $Manager verify --archive (Join-Path $OutputDir $asset) --version $Version --platform windows --arch $Architecture --asset $asset
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
