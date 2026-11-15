#!/usr/bin/env bash
# One entry point for the EC2 host, so the right Compose files and profile are
# always used together. Forgetting compose.notifications.yaml on a plain
# "docker compose up -d" would silently detach WhatsApp from the API.
#
#   bash deploy.sh check     validate .env without printing any value
#   bash deploy.sh up        start/refresh every service enabled in .env
#   bash deploy.sh update    git pull, rebuild, show pending migrations, then up
#   bash deploy.sh status    container state
#   bash deploy.sh logs [service]
#   bash deploy.sh migrate [status|up]
set -euo pipefail
cd "$(dirname "$0")"

value() { sed -n "s/^$1=//p" .env | tail -n 1 | tr -d '\r'; }
enabled() { [ "$(value "$1")" = "true" ]; }

compose() {
  local args=(-f compose.yaml)
  if enabled NOTIFICATION_PLANNING_ENABLED || enabled NOTIFICATION_DELIVERY_ENABLED || enabled NOTIFICATION_AUTOMATIC_ENABLED; then
    args+=(-f compose.notifications.yaml)
    if enabled NOTIFICATION_AUTOMATIC_ENABLED; then args+=(--profile worker); fi
  fi
  docker compose "${args[@]}" "$@"
}

check() {
  local failed=0
  fail() { echo "  ERRO  $1"; failed=1; }
  [ -f .env ] || { echo "infra/aws/.env not found. Run bash init-env.sh first." >&2; exit 1; }
  if [ "$(stat -c %a .env 2>/dev/null || echo 600)" != "600" ]; then fail ".env deve ter permissão 600 (chmod 600 .env)"; fi
  if grep -q $'\r' .env; then fail ".env tem quebras de linha do Windows (rode: sed -i 's/\\r\$//' .env)"; fi
  if grep -qE '^[A-Z0-9_]+=.*[[:space:]]$' .env; then fail "há linhas com espaço no final do valor"; fi

  for name in API_DOMAIN TLS_EMAIL FRONTEND_ORIGIN DATABASE_URL SUPABASE_URL SUPABASE_PUBLISHABLE_KEY \
    SUPABASE_SERVICE_ROLE_KEY STRIPE_MODE STRIPE_SECRET_KEY STRIPE_MONTHLY_PRICE_ID \
    RABBITMQ_PASSWORD EVOLUTION_DB_PASSWORD EVOLUTION_API_KEY EVOLUTION_WEBHOOK_SECRET; do
    [ -n "$(value "$name")" ] || fail "$name está vazio"
  done

  local domain origin db mode key
  domain=$(value API_DOMAIN); origin=$(value FRONTEND_ORIGIN); db=$(value DATABASE_URL)
  mode=$(value STRIPE_MODE); key=$(value STRIPE_SECRET_KEY)
  case "$domain" in *example.com | *://* | */*) fail "API_DOMAIN deve ser só o domínio da API, sem https:// e sem barra" ;; esac
  case "$origin" in
    https://*example.com) fail "FRONTEND_ORIGIN ainda é o valor de exemplo" ;;
    https://*/) fail "FRONTEND_ORIGIN não pode terminar com /" ;;
    https://*) ;;
    *) fail "FRONTEND_ORIGIN deve começar com https://" ;;
  esac
  case "$db" in
    postgres://* | postgresql://*) ;;
    *) fail "DATABASE_URL deve começar com postgresql:// (sem aspas nem barra invertida)" ;;
  esac
  case "$db" in *sslmode=disable*) fail "DATABASE_URL usa sslmode=disable; use sslmode=require" ;; esac
  case "$db" in *'$'*) fail "DATABASE_URL contém \$; escreva \$\$ no lugar de cada \$" ;; esac
  case "$mode" in
    test | live) case "$key" in sk_"$mode"_*) ;; *) [ -z "$key" ] || fail "STRIPE_SECRET_KEY não é uma chave sk_${mode}_" ;; esac ;;
    *) fail "STRIPE_MODE deve ser test ou live" ;;
  esac
  case "$(value STRIPE_MONTHLY_PRICE_ID)" in price_* | "") ;; *) fail "STRIPE_MONTHLY_PRICE_ID deve começar com price_" ;; esac
  case "$(value STRIPE_WEBHOOK_SECRET)" in
    whsec_*) ;;
    "") echo "  AVISO STRIPE_WEBHOOK_SECRET vazio: a rota do webhook responde 404 e assinaturas não são confirmadas" ;;
    *) fail "STRIPE_WEBHOOK_SECRET deve começar com whsec_" ;;
  esac
  if enabled NOTIFICATION_DELIVERY_ENABLED || enabled NOTIFICATION_AUTOMATIC_ENABLED; then
    enabled NOTIFICATION_PLANNING_ENABLED || fail "envio de mensagens exige NOTIFICATION_PLANNING_ENABLED=true"
    [ -n "$(value NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID)" ] && [ -n "$(value NOTIFICATION_PAYLOAD_KEYS)" ] ||
      fail "envio de mensagens exige NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID e NOTIFICATION_PAYLOAD_KEYS"
  fi
  if enabled NOTIFICATION_AUTOMATIC_ENABLED && ! enabled NOTIFICATION_DELIVERY_ENABLED; then
    fail "lembrete automático exige NOTIFICATION_DELIVERY_ENABLED=true"
  fi
  # Total memory below ~3.5 GB froze this host while building Evolution.
  local memory
  memory=$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo 2>/dev/null || echo 0)
  if [ "$memory" -gt 0 ] && [ "$memory" -lt 3500 ] && compose config --services 2>/dev/null | grep -q '^evolution$'; then
    echo "  AVISO ${memory} MB de memória: o WhatsApp (Evolution) precisa de uma instância com 4 GB"
  fi
  if [ "$failed" -ne 0 ]; then echo "Corrija os itens acima em infra/aws/.env." >&2; return 1; fi
  compose config --quiet
  echo "  OK    .env válido. Serviços: $(compose config --services | sort | tr '\n' ' ')"
}

command=${1:-}
[ $# -gt 0 ] && shift
case "$command" in
  check) check ;;
  up)
    check
    compose up -d
    compose ps
    ;;
  update)
    check
    git -C ../.. pull --ff-only
    compose build api
    compose --profile tools build migrate
    status=$(compose --profile tools run --rm migrate status 2>&1) || { echo "$status" >&2; exit 1; }
    echo "$status"
    if echo "$status" | grep -q "Pending"; then
      echo "Há migrations pendentes. Faça backup, rode bash deploy.sh migrate up e repita bash deploy.sh update." >&2
      echo "A API não foi reiniciada." >&2
      exit 1
    fi
    compose up -d
    compose ps
    ;;
  status) compose ps ;;
  logs) compose logs --tail "${TAIL:-100}" -f "$@" ;;
  migrate)
    action=${1:-status}
    case "$action" in status | up) ;; *) echo "Use: bash deploy.sh migrate [status|up]" >&2; exit 1 ;; esac
    compose --profile tools build migrate
    compose --profile tools run --rm migrate "$action"
    ;;
  *)
    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
    exit 1
    ;;
esac
