<#
.SYNOPSIS
  Runs the Barber-chat load test (k6) against the LOCAL stack.

.EXAMPLE
  # 100 customers browsing the public booking page
  .\loadtest\run-loadtest.ps1 -Slug minha-barbearia

  # Everything: public page, simultaneous bookings, admin panel and Next pages
  .\loadtest\run-loadtest.ps1 -Slug minha-barbearia -Write -Token $token -Front http://127.0.0.1:3000 -SpreadIPs

.NOTES
  Install k6 once:  winget install k6 --source winget   (then open a new terminal)
  Before running:   API with `go run ./cmd/api` (backend-chat) and, for -Front, the frontend in
                    production mode: `npm run build` and `npm start` (frontend-chat). `npm run dev`
                    is far slower and would distort the numbers.
#>
param(
  [Parameter(Mandatory = $true)][string]$Slug,
  [string]$Api = 'http://127.0.0.1:8080',
  [string]$Front = '',
  [string]$Token = '',
  [int]$Users = 100,
  [string]$Duration = '3m',
  # Creates real appointments. Allowed only against a local API (local database).
  [switch]$Write,
  # Gives each virtual user its own loopback address (127.0.0.10 ... ), so the API's per-IP
  # limiter sees 100 different customers instead of one.
  [switch]$SpreadIPs
)

$ErrorActionPreference = 'Continue'
Set-Location (Join-Path $PSScriptRoot '..')

if (-not (Get-Command k6 -ErrorAction SilentlyContinue)) {
  Write-Host 'k6 is not installed. Run: winget install k6 --source winget  (then open a new terminal)' -ForegroundColor Red
  exit 1
}

$apiUri = [Uri]$Api
if ($Write -and -not ($apiUri.IsLoopback)) {
  Write-Host 'Refusing -Write: it creates appointments. Use it only with a local API and a local database.' -ForegroundColor Red
  exit 1
}

$health = & curl.exe -s --max-time 5 "$Api/health"
if ($LASTEXITCODE -ne 0 -or "$health" -notmatch 'ok') {
  Write-Host "The API is not answering at $Api/health. Start it with: cd backend-chat; go run ./cmd/api" -ForegroundColor Red
  exit 1
}

New-Item -ItemType Directory -Force -Path 'loadtest/results' | Out-Null

$k6Args = @('run', '-e', "API=$Api", '-e', "SLUG=$Slug", '-e', "USERS=$Users", '-e', "DURATION=$Duration")
if ($Front) { $k6Args += @('-e', "FRONT=$Front") }
if ($Token) { $k6Args += @('-e', "TOKEN=$Token") }
if ($Write) { $k6Args += @('-e', 'WRITE=1') }
if ($SpreadIPs) {
  $last = 9 + [Math]::Min([Math]::Max($Users, 1), 240)
  $k6Args += @('-e', "LOCAL_IPS=127.0.0.10-127.0.0.$last")
}
$k6Args += 'loadtest/barber-load.js'

Write-Host "Running k6 with $Users users for $Duration (plus ramp-up and ramp-down)..." -ForegroundColor Cyan
& k6 @k6Args
$code = $LASTEXITCODE

Write-Host ''
if ($code -eq 0) {
  Write-Host 'All thresholds passed. Results saved in loadtest/results.' -ForegroundColor Green
} elseif ($code -eq 99) {
  Write-Host 'The test ran, but some thresholds failed (look for the crossed lines above). Results saved in loadtest/results.' -ForegroundColor Yellow
} else {
  Write-Host "k6 stopped with exit code $code." -ForegroundColor Red
  if ($SpreadIPs) { Write-Host 'If the error mentions local IPs, run again without -SpreadIPs.' -ForegroundColor Yellow }
}
exit $code
