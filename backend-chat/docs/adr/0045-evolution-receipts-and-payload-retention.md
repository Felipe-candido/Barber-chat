# 0045 — Evidência de entrega e retenção do conteúdo privado

Data: 2026-11-02. Status: implementado.

## Decisão

Configurar CONNECTION_UPDATE, SEND_MESSAGE e MESSAGES_UPDATE no webhook da
instância. Autenticar com header derivado por HMAC por instância, sobre HTTPS
fora do ambiente local. Esse token autentica a origem; não assina o corpo.
Nunca usar o apikey que veio no JSON como autenticação.

Somente eventos fromMe=true com ID de 32 hexadecimais que corresponda ao UUID
aleatório de uma tentativa persistida daquela instância atual são elegíveis.
Não consultar por telefone ou aceitar IDs arbitrários. Objetos do fornecedor
são traduzidos para ProviderReceipt pequeno, sem texto, telefone ou timestamp remoto.

Aceitação, entrega e leitura são estados monotônicos separados do lifecycle da
reserva. Recibo positivo pode resolver sending/unknown do token correspondente.
Resultado HTTP atrasado não regride accepted. Evidência histórica não sobrescreve
uma tentativa atual com outro token. ERROR não permite retry automático.

Remover ciphertext após payload_expires_at, com batch limitado/SKIP LOCKED,
preservando metadados, tentativas e agendamento. Payload expirado nunca é enviado,
mesmo que a limpeza ainda não tenha rodado. O worker faz manutenção periódica;
sem worker, operador executa cmd/notifications-maintenance em processo único.
Outbox consumida é removida após 30 dias; heartbeat após sete dias. Esses prazos
não são a política de retenção de todo o histórico comercial.

## Consequências

Recibos dependem da disponibilidade real do provedor e podem não chegar. accepted
não significa entregue; ausência de READ não significa que o cliente não leu.
O painel comunica esses estados sem prometer recebimento ou confirmar presença.
Não persistir eventos brutos. Payload purgado não pode ser restaurado por down
migration; a migration recusa esse rollback para impedir falsa recuperação.

Testes verificam autenticação/correlação, estados fora de ordem, HTTP perdido,
limpeza idempotente e preservação de histórico/conteúdo ainda válido. Testes
automatizados não substituem pareamento e recebimento num telefone real.
