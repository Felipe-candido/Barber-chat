#!/usr/bin/env bash
# Linux counterpart of Initialize-Environment.ps1: creates infra/aws/.env with new
# private infrastructure secrets. Never overwrites an existing file or prints values.
set -euo pipefail
cd "$(dirname "$0")"
if [ -e .env ]; then
  echo "infra/aws/.env already exists. Edit it explicitly; no values were changed." >&2
  exit 1
fi
command -v openssl >/dev/null || { echo "openssl is required." >&2; exit 1; }
umask 077
cp .env.example .env
for name in RABBITMQ_PASSWORD EVOLUTION_DB_PASSWORD EVOLUTION_API_KEY EVOLUTION_WEBHOOK_SECRET; do
  sed -i "s/^${name}=\$/${name}=$(openssl rand -hex 32)/" .env
done
chmod 600 .env
echo "Created infra/aws/.env with new private infrastructure secrets. Values were not printed."
echo "Fill the domain, site origin, Supabase and Stripe values, then run bash deploy.sh check."
echo "Reuse the complete notification encryption keyring if the database already contains jobs."
