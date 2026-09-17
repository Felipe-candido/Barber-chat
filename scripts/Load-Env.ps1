# Load simple KEY=VALUE settings into the current PowerShell process.
# This deliberately does not evaluate expressions or expand variables.
param([string]$Path = (Join-Path $PSScriptRoot '..\.env'))

$ErrorActionPreference = 'Stop'
foreach ($line in Get-Content -LiteralPath $Path) {
    if ($line -match '^\s*(#|$)') { continue }
    if ($line -notmatch '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
        throw 'Invalid .env line. Expected KEY=VALUE.'
    }
    $settingName = $Matches[1]
    $settingValue = $Matches[2].Trim()
    if ($settingValue.Length -ge 2 -and
        (($settingValue.StartsWith('"') -and $settingValue.EndsWith('"')) -or
         ($settingValue.StartsWith("'") -and $settingValue.EndsWith("'")))) {
        $settingValue = $settingValue.Substring(1, $settingValue.Length - 2)
    }
    [Environment]::SetEnvironmentVariable($settingName, $settingValue, 'Process')
}
