# Exercises only a unique local Compose project with an empty PostgreSQL fixture.
param([switch]$SkipBuild)
$ErrorActionPreference = 'Stop'
$taskProject = 'codex-aws-' + [guid]::NewGuid().ToString('N').Substring(0, 10)
$taskDirectory = Join-Path ([System.IO.Path]::GetTempPath()) $taskProject
[System.IO.Directory]::CreateDirectory($taskDirectory) | Out-Null
$taskEnvFile = Join-Path $taskDirectory '.env'
$taskPrevious = @{}
$taskStarted = $false

function Compose {
    & docker @script:taskCompose @args
    if ($LASTEXITCODE -ne 0) { throw 'Local container test command failed; production was not accessed.' }
}
function Sql([string]$query) {
    $result = & docker @script:taskCompose exec -T test-db psql -U deployment_test -d deployment_test -v ON_ERROR_STOP=1 -At -c $query
    if ($LASTEXITCODE -ne 0) { throw 'Local fixture SQL failed.' }
    return ($result -join "`n").Trim()
}
function Assert-Status([string]$path, [int]$expected, [string[]]$headers = @()) {
    $arguments = @('--insecure', '--silent', '--show-error', '--max-time', '15', '--output', (Join-Path $taskDirectory 'response'), '--write-out', '%{http_code}')
    $status = & $script:taskCurl @arguments @headers ('https://localhost:18443' + $path)
    if ($LASTEXITCODE -ne 0 -or [int]$status -ne $expected) { throw "Unexpected HTTP status for $path; expected $expected." }
}
function Wait-Heartbeat([string]$after = '0') {
    $deadline = [DateTime]::UtcNow.AddSeconds(90)
    do {
        if ((Sql "SELECT EXISTS(SELECT 1 FROM notification_worker_heartbeats WHERE healthy AND checked_at>clock_timestamp()-interval '15 seconds' AND checked_at>to_timestamp($after))") -eq 't') { return }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'The automatic worker did not persist a healthy scheduler/AMQP heartbeat.'
}
try {
    if ($env:DOCKER_HOST -and $env:DOCKER_HOST -notmatch '^(npipe|unix)://') { throw 'Use a local Docker engine for this test.' }
    $context = (& docker context inspect | ConvertFrom-Json)[0]
    if ($LASTEXITCODE -ne 0 -or $context.Endpoints.docker.Host -notmatch '^(npipe|unix)://') { throw 'Use a local Docker context, never an EC2 context.' }
    $curlCommand = Get-Command curl.exe -ErrorAction SilentlyContinue
    if (-not $curlCommand) { $curlCommand = Get-Command curl -CommandType Application }
    $script:taskCurl = $curlCommand.Source

    # Override inherited application settings as well as --env-file. A prior
    # Load-Env.ps1 must never redirect this test to the user's Supabase or Stripe.
    $settings = @{}
    foreach ($line in [System.IO.File]::ReadAllLines((Join-Path $PSScriptRoot '.env.example'))) {
        if ($line -match '^([A-Z][A-Z0-9_]*)=(.*)$') { $settings[$Matches[1]] = $Matches[2] }
    }
    $settings['DATABASE_URL'] = 'postgres://deployment_test:local_container_test_only@test-db:5432/deployment_test?sslmode=disable'
    $settings['SUPABASE_URL'] = 'https://deployment-fixture.supabase.co'
    $settings['API_DOMAIN'] = 'localhost'
    $settings['TLS_EMAIL'] = 'deployment@example.com'
    $settings['FRONTEND_ORIGIN'] = 'https://app.example.com'
    $settings['PUBLIC_BIND_IP'] = '127.0.0.1'
    $settings['HTTP_PORT'] = '18080'
    $settings['HTTPS_PORT'] = '18443'
    $settings['EVOLUTION_MANAGER_PORT'] = '18081'
    $settings['PROXY_SUBNET'] = '172.30.88.0/29'
    $settings['CADDY_IPV4_ADDRESS'] = '172.30.88.2'
    $settings['API_IPV4_ADDRESS'] = '172.30.88.3'
    $settings['NOTIFICATION_PLANNING_ENABLED'] = 'true'
    $settings['COMPOSE_PROFILES'] = ''
    $settings['NOTIFICATION_DELIVERY_ENABLED'] = 'true'
    $settings['NOTIFICATION_AUTOMATIC_ENABLED'] = 'true'
    $settings['NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID'] = 'deployment-test'
    $settings['NOTIFICATION_PAYLOAD_KEYS'] = '{"deployment-test":"' + [Convert]::ToBase64String((New-Object byte[] 32)) + '"}'
    foreach ($name in @('RABBITMQ_PASSWORD', 'EVOLUTION_DB_PASSWORD', 'EVOLUTION_API_KEY', 'EVOLUTION_WEBHOOK_SECRET')) {
        $settings[$name] = 'local_test_' + [guid]::NewGuid().ToString('N')
    }
    $text = foreach ($name in $settings.Keys) {
        $taskPrevious[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $settings[$name], 'Process')
        $name + '=' + $settings[$name]
    }
    [System.IO.File]::WriteAllLines($taskEnvFile, [string[]]$text, (New-Object System.Text.UTF8Encoding($false)))
    $script:taskCompose = @('compose', '--project-name', $taskProject, '--env-file', $taskEnvFile, '-f', (Join-Path $PSScriptRoot 'compose.yaml'), '-f', (Join-Path $PSScriptRoot 'compose.notifications.yaml'), '-f', (Join-Path $PSScriptRoot 'tests/compose.yaml'), '--profile', 'worker')

    # config is captured, never printed: the interpolated form contains secrets.
    $config = (& docker @script:taskCompose config --format json | ConvertFrom-Json)
    if ($LASTEXITCODE -ne 0) { throw 'Invalid Compose configuration.' }
    foreach ($name in @('api', 'worker', 'rabbitmq', 'evolution-db', 'evolution-redis', 'test-db')) {
        if ($config.services.$name.ports) { throw "Private service $name exposes a host port." }
    }
    if ($config.services.api.environment.TRUSTED_PROXY_CIDRS -ne '172.30.88.2/32') { throw 'API must trust only Caddy.' }
    if (-not $SkipBuild) { Compose build api migrate evolution }
    Write-Output ('Starting isolated local test project ' + $taskProject)
    $taskStarted = $true
    Compose up -d --wait --wait-timeout 120 test-db
    Compose run --rm --no-deps migrate up
    Compose up -d --wait --wait-timeout 300
    Compose exec -T api /app/healthcheck
    Assert-Status '/api/v1/admin/me' 401
    Assert-Status '/api/v1/admin/me' 204 @('--request', 'OPTIONS', '--header', 'Origin: https://app.example.com', '--header', 'Access-Control-Request-Method: GET')
    Assert-Status '/api/v1/admin/me' 403 @('--request', 'OPTIONS', '--header', 'Origin: https://untrusted.example.com', '--header', 'Access-Control-Request-Method: GET')
    Assert-Status '/api/v1/webhooks/evolution' 404
    Assert-Status '/ready' 404
    Wait-Heartbeat
    Write-Output 'HTTPS, CORS, private routes, API readiness and automatic worker heartbeat passed.'

    # Recreate the provider database container and verify volume persistence.
    Compose exec -T evolution-db psql -U evolution -d evolution -v ON_ERROR_STOP=1 -c 'CREATE TABLE codex_deployment_probe(id INT PRIMARY KEY); INSERT INTO codex_deployment_probe VALUES(1);'
    Compose up -d --force-recreate --wait --wait-timeout 120 evolution-db
    $persisted = & docker @script:taskCompose exec -T evolution-db psql -U evolution -d evolution -At -c 'SELECT count(*) FROM codex_deployment_probe'
    if ($LASTEXITCODE -ne 0 -or $persisted.Trim() -ne '1') { throw 'Evolution database volume was not preserved.' }
    Compose restart rabbitmq
    # Require a later heartbeat, not a cached healthy row from before reconnect.
    $afterRestart = Sql 'SELECT extract(epoch FROM clock_timestamp())'
    Wait-Heartbeat $afterRestart
    Write-Output 'Provider volume persistence and broker reconnection passed. No phone was paired or messaged.'
} finally {
    if ($taskStarted) {
        # No -v: retain all test volumes; never touch projects already in use.
        & docker @script:taskCompose down --timeout 40
        if ($LASTEXITCODE -ne 0) { Write-Warning ('Test containers need manual inspection: ' + $taskProject) }
    }
    foreach ($name in $taskPrevious.Keys) { [Environment]::SetEnvironmentVariable($name, $taskPrevious[$name], 'Process') }
    Write-Output ('Disposable test volume prefix retained: ' + $taskProject)
}
