# Creates deployment-only secrets. Never overwrite an existing environment.
param()
$ErrorActionPreference = 'Stop'
$target = Join-Path $PSScriptRoot '.env'
if (Test-Path -LiteralPath $target) {
    throw 'infra/aws/.env already exists. Edit it explicitly; no values were changed.'
}
$content = [System.IO.File]::ReadAllText((Join-Path $PSScriptRoot '.env.example'))
$random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try {
    foreach ($name in @('RABBITMQ_PASSWORD', 'EVOLUTION_DB_PASSWORD', 'EVOLUTION_API_KEY', 'EVOLUTION_WEBHOOK_SECRET')) {
        $bytes = New-Object byte[] 32
        $random.GetBytes($bytes)
        $value = [BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()
        $content = [regex]::Replace($content, '(?m)^' + $name + '=\r?$', $name + '=' + $value)
    }
} finally { $random.Dispose() }
# CreateNew protects against overwriting a file created concurrently.
$stream = [System.IO.File]::Open($target, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write)
try {
    $bytes = (New-Object System.Text.UTF8Encoding($false)).GetBytes($content)
    $stream.Write($bytes, 0, $bytes.Length)
} finally { $stream.Dispose() }
Write-Output 'Created infra/aws/.env with new private infrastructure secrets. Values were not printed.'
Write-Output 'Fill the domain, Supabase connection and project keys before deployment.'
Write-Output 'Reuse the complete notification encryption keyring if the database already contains jobs.'
