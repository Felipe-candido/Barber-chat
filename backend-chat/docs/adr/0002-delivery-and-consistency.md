# ADR 0002 — PostgreSQL como fonte de verdade; RabbitMQ como transporte

Status: arquitetura adotada; fluxo de negócio ainda não implementado. Data: 2026-09-17.

## Decisão

Escolher RabbitMQ por seu valor educacional, apesar do custo de operar um broker adicional. Para um MVP exclusivamente orientado à simplicidade, River/PostgreSQL seria a primeira alternativa. Lembretes e retries ficam persistidos no PostgreSQL; não usar plugin de atraso nem timers em memória como fonte de verdade.

Criar cliente, agendamento, intenção de confirmação e job de lembrete na mesma transação. Outbox publicada depois do commit com publisher confirms, roteamento verificado e possibilidade de duplicação. ACK do consumidor somente após resultado ou retry persistido. PostgreSQL mantém a chave lógica de envio e tentativas; um provedor com idempotência é necessário para fechar a janela entre envio externo e commit local.

## Consequências

Entrega pelo menos uma vez; não prometer exactly-once ponta a ponta. API pode aceitar reservas com broker indisponível. São necessários scheduler com leases, relay de outbox, reconexão, DLQ e observabilidade antes de habilitar notificações. O scaffold apenas abre conexões; esses mecanismos estão planejados em [architecture.md](../architecture.md).
