# Teste de carga (k6)

Simula clientes e equipe usando o Na Regua ao mesmo tempo, **contra o ambiente local**.

## Preparar (uma vez)

```powershell
winget install k6 --source winget   # depois abra um terminal novo
```

## Rodar

1. Banco e fila: `cd backend-chat; docker compose up -d --wait`
2. API: `cd backend-chat; go run ./cmd/api`
3. (Opcional, para testar páginas) Frontend em modo produção: `cd frontend-chat; npm run build; npm start`
4. Uma barbearia **publicada**, com pelo menos um serviço e um barbeiro com expediente. Use o slug dela.

Na raiz do repositório:

```powershell
# 100 clientes navegando na página pública de agendamento
.\loadtest\run-loadtest.ps1 -Slug minha-barbearia

# Cada cliente com um IP próprio (o limitador da API trata cada IP separadamente)
.\loadtest\run-loadtest.ps1 -Slug minha-barbearia -SpreadIPs

# Completo: + agendamentos simultâneos, painel e páginas do Next
.\loadtest\run-loadtest.ps1 -Slug minha-barbearia -SpreadIPs -Write -Token $token -Front http://127.0.0.1:3000
```

- `-Write` cria agendamentos de verdade (clientes "Carga Teste N"). Só é aceito com API local. Para limpar depois, apague esses agendamentos no banco local ou recrie o volume do Docker.
- `-Token`: entre no painel, abra o DevTools (F12) → Application → Local Storage → a chave `sb-...-auth-token` → copie o valor de `access_token`. Ele vale cerca de 1 hora.
- `-Users` e `-Duration` mudam a carga (padrão: 100 usuários por 3 minutos, mais rampas de subida e descida).

## Como ler o resultado

O final da saída traz um resumo e grava `loadtest/results/summary-*.txt|json`.

| Métrica | Bom sinal | O que indica se estiver ruim |
| --- | --- | --- |
| `http_req_duration` p95 | público < 500 ms, painel < 800 ms | fila no pool do Postgres, consultas lentas |
| `rate_limited_429` | perto de 0 | o limitador de requisições da API cortando clientes legítimos |
| `slot_conflict_409` | aparece com `-Write` | **esperado**: dois clientes disputando o mesmo horário; o banco impede reserva dupla |
| `unavailable_503` / `server_errors_5xx` | 0 | timeouts por falta de conexões livres, erros internos |
| `bookings_created` | > 0 com `-Write` | — |

Linhas marcadas com ✗ são limites (thresholds) que falharam.
