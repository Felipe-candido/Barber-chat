$ErrorActionPreference = 'Stop'
$evolutionEnvPath = Join-Path $PSScriptRoot '.env'
if (Test-Path -LiteralPath $evolutionEnvPath) {
    Write-Output 'Local Evolution configuration already exists; values were preserved.'
    return
}
$evolutionDbBytes = New-Object byte[] 32
$evolutionApiBytes = New-Object byte[] 32
$evolutionRng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $evolutionRng.GetBytes($evolutionDbBytes)
    $evolutionRng.GetBytes($evolutionApiBytes)
} finally {
    $evolutionRng.Dispose()
}
$evolutionDbPassword = [BitConverter]::ToString($evolutionDbBytes).Replace('-', '').ToLowerInvariant()
$evolutionApiKey = [BitConverter]::ToString($evolutionApiBytes).Replace('-', '').ToLowerInvariant()
$evolutionContents = "EVOLUTION_PORT=8081`nEVOLUTION_DB_PASSWORD=$evolutionDbPassword`nEVOLUTION_API_KEY=$evolutionApiKey`n"
$evolutionFile = [System.IO.File]::Open($evolutionEnvPath, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write)
try {
    $evolutionBytes = [System.Text.Encoding]::UTF8.GetBytes($evolutionContents)
    $evolutionFile.Write($evolutionBytes, 0, $evolutionBytes.Length)
} finally {
    $evolutionFile.Dispose()
    [Array]::Clear($evolutionDbBytes, 0, $evolutionDbBytes.Length)
    [Array]::Clear($evolutionApiBytes, 0, $evolutionApiBytes.Length)
}
Write-Output 'Created ignored infra/evolution/.env. No backend .env values were changed.'
