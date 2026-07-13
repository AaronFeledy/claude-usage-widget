$ErrorActionPreference = 'Stop'

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot '..' '..')
$installerPath = Join-Path $repositoryRoot 'install.ps1'
$installerSource = Get-Content $installerPath -Raw
$guardMatch = [regex]::Match(
    $installerSource,
    '(?m)^\s*if \([^\r\n]*\$PSCmdlet\.ShouldProcess\([^\r\n]*\)\) \{$'
)

if (-not $guardMatch.Success) {
    throw 'Could not find the installer ShouldProcess guard.'
}

$guard = $guardMatch.Value.Trim()
$harness = @"
[CmdletBinding(SupportsShouldProcess = `$true)]
param()
`$installDir = 'C:\Test'
$guard
    'entered'
}
"@

$result = $harness | Invoke-Expression
if ($result -notcontains 'entered') {
    throw 'The installer guard did not enter its install block under Invoke-Expression.'
}

Write-Host 'Install script Invoke-Expression test passed'
