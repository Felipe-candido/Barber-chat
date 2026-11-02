# 0044 — Outbox, RabbitMQ e processo opcional de notificações

Data: 2026-11-02. Status: implementado.

## Decisão

Manter um módulo Go e executáveis separados. A API não conecta RabbitMQ para
notificações e pode operar em modo request no Render Free. O worker só inicia
o runtime real com NOTIFICATION_AUTOMATIC_ENABLED=true e dependências válidas.

Scheduler PostgreSQL busca trabalho devido com SKIP LOCKED, incrementa sequência
e cria uma outbox na mesma transação. Modo request nunca é materializado para
envio automático. Publish usa lease/token, mensagem persistente, mandatory e
confirmação; consumidor usa prefetch 1 e ACK somente após decisão persistida.
Publicação incerta pode repetir o mesmo envelope. Consumo repetido consulta o
estado da tentativa antes de qualquer I/O. Envelope antigo não inicia geração nova.

Uma sessão de sender por barbearia é serializada com lease durável; nenhum lock
SQL fica aberto durante a chamada HTTP. Lock order no término é conexão, job,
tentativa, compatível com o início. Tokens antigos não liberam outro sender.

Job sending cujo lease expirou vira unknown. Lease antes de I/O pode ser
recuperado. Somente not_sent/provider_unavailable comprovado permite retry
automático: até cinco tentativas, backoff 30s exponencial com teto 15min,
sempre dentro da janela e com toda a elegibilidade revalidada. Timeout, conexão
perdida e HTTP 400 genérico da Evolution não comprovam ausência do efeito.

Fila principal e DLQ são quorum/duráveis. Envelopes inválidos vão para DLQ;
indisponibilidade SQL fecha a sessão sem ACK. Supervisor reconecta com backoff
e jitter, e encerra loops/canais antes do pool. API e worker podem reconciliar
conexões em paralelo, coordenados pelo PostgreSQL.

## Operação e consequências

Heartbeat saudável exige sessão AMQP ativa e scheduler válido. Configuração
automática só é oferecida com flag global e heartbeat saudável em até dois minutos.
Isso indica capacidade recente, sem garantir disponibilidade futura do telefone.
Logs agregados mostram outbox pendente, unknown, sending e atraso dos jobs,
sem telefone, texto, código ou rótulos por cliente.

Testes com PostgreSQL e RabbitMQ reais cobrem concorrência, fencing, ACK perdido,
confirm perdido, publicação sem rota, DLQ, desconexão TCP, reconexão e shutdown.
Não existe transação distribuída nem garantia de exactly-once no WhatsApp.

Não usar RabbitMQ TTL para aguardar a data do lembrete: due_at continua no banco.
O TTL da DLQ limita retenção operacional de envelopes inválidos.
