# Contrato com o frontend

## Estado atual

GET /health, GET /ready, POST /api/v1/admin/services (modo local explícito) e GET /api/v1/public/shops/{slug}/services estão implementados; ver [OpenAPI](openapi.yaml) e [guia de testes](catalog-testing.md). A primeira rota mede liveness, a segunda testa PostgreSQL com deadline. Readiness não valida migrations nem RabbitMQ: a futura API grava uma outbox e não depende do broker para aceitar agendamentos. O worker precisa de PostgreSQL e RabbitMQ e encerra com erro se perder uma dependência. As rotas de agendamento abaixo são propostas. Criação/listagem de serviços seguem o ADR 0007; escrita administrativa com membership ainda é futura.

## Fluxo público proposto

Frontend separado, provavelmente Next.js, com áreas pública e administrativa. Não existe chatbot com IA. O formulário em formato de conversa coleta nome, sobrenome, telefone, serviço, profissional, data, horário e revisão. Manter dados pessoais no estado de memória até confirmar; evitar armazenamento persistente no navegador por padrão. Não criar cliente ao avançar as etapas. Mostrar os detalhes imediatamente após o HTTP 201; entrega por mensagem ocorre depois e pode falhar sem desfazer a reserva.

| Método e rota proposta | Contrato |
| --- | --- |
| `GET /api/v1/public/shops/{slug}` | Identidade pública, timezone IANA, serviços ativos e profissionais habilitados; sem dados privados |
| `GET /api/v1/public/shops/{slug}/availability?service_id=...&professional_id=...&date=2026-10-01` | Data civil na barbearia; slots com `starts_at` e `ends_at` RFC 3339 com offset; seleção não reserva |
| `POST /api/v1/public/shops/{slug}/appointments` | Header `Idempotency-Key`; cliente, seleção e consentimentos; preço e duração calculados pelo servidor |

Exemplo de corpo futuro (identificadores ilustrativos):

```json
{
  "customer": { "first_name": "Ana", "last_name": "Silva", "phone": "+5511999999999" },
  "service_id": "service-uuid",
  "professional_id": "professional-uuid",
  "starts_at": "2026-10-01T14:00:00-03:00",
  "consent": { "booking_messages": true, "marketing": false, "notice_version": "v1" }
}
```

Resposta planejada: HTTP 201 com `appointment_id`, `status: scheduled`, snapshots do serviço/preço/duração, início, fim, moeda `BRL`, timezone, e estado da notificação `pending` ou `not_requested`. Nome/telefone não devem ser retornados em listagens públicas. A confirmação na tela não deve afirmar que a mensagem foi entregue. Se não houver consentimento para mensagens, a reserva pode continuar, sem jobs de envio.

Reutilizar uma chave com o mesmo corpo devolve o mesmo resultado; com corpo diferente, HTTP 409. A chave é aleatória, vinculada ao tenant e à operação, mantida pelo frontend durante retries. Retenção inicial proposta: 24 horas; expiração deve estar no contrato antes de implementar. Não expor dados de outros clientes por meio da chave.

## Validação, erros e acesso

Proposta de erros JSON: `{"error":{"code":"slot_unavailable","message":"Horário indisponível","request_id":"..."}}`. HTTP 400 para JSON/formato inválido, 422 para regra de entrada, 404 para recurso inexistente ou fora do tenant, 409 para concorrência/idempotência, 429 para limite de requisições e 503 para indisponibilidade transitória. Nunca incluir SQL, URLs com senha, stack traces ou respostas brutas do provedor. As rotas da fundação têm os corpos simples descritos no OpenAPI; 404/405 ainda são respostas padrão do Chi.

Aplicar limites de corpo, rejeitar campos desconhecidos, exigir exatamente um objeto JSON e validar tamanho de nomes, IDs, data e telefone no backend. A validação do frontend serve à experiência, não à integridade. Normalizar telefone internacionalmente para E.164 com país explícito; não basta remover pontuação ou prefixar +55. Selecionar biblioteca baseada em metadados de telefonia quando o fluxo for implementado. Telefone informado não prova identidade; números compartilhados/reciclados exigem cuidado ao reutilizar clientes. Nunca retornar histórico só porque alguém conhece o telefone.

Rotas administrativas propostas sob `/api/v1/admin`, com sessão e membership da barbearia verificados no backend. Avaliar OIDC com provedor maduro e sessão via cookie HttpOnly/Secure/SameSite; evitar criar autenticação própria neste scaffold. Autenticação pode ficar em um BFF Next.js, mas autorização por tenant deve permanecer na API. CSRF para autenticação por cookie; CORS com origens explícitas se as origens forem diferentes. Um proxy no mesmo domínio pode simplificar o MVP. O POST de serviços atual é uma facilidade de teste em loopback; não implementa autenticação nem membership.

Cancelamento/confirmacão pública futura: token opaco, aleatório, limitado ao agendamento, armazenado com hash e expiração; nunca apenas ID previsível ou telefone. Webhooks SIM/NÃO terão assinatura e deduplicação do ID do provedor. Se houver várias reservas para o mesmo telefone, correlacionar com uma mensagem/agendamento antes de alterar estado. Marketing precisa de opt-in separado, revogável e não pré-selecionado. Registrar versão do aviso e instante do consentimento sem acrescentar PII desnecessária aos logs.
