# 0039 — Plano de notificações na transação do agendamento

Data: 2026-11-01. Estado: entrega B implementada e testada em PostgreSQL local.
Complementa o [ADR 0038](0038-shop-whatsapp-notification-worker.md).

## Decisão

Booking declara BookingNotificationPlanner em application/notifications.go.
CreateAppointment invoca essa porta depois de inserir uma reserva nova e antes
de completar o recibo idempotente. O replay sai antes desse ponto. Ausência de
planner na composição de criação é erro, evitando ignorar o planejamento.

Notifications/infra/booking.go traduz os contratos entre módulos. Seu caso de
uso PlanBookingNotifications recebe policy/store, recorder de customers e reader
de shops, todos ligados às mesmas Queries da transação de booking. Nenhum deles
abre outro pool, faz commit próprio, publica AMQP ou envia HTTP.

A preparação da entrega A produz as janelas e o snapshot cifrado. Nesta entrega,
o adaptador PostgreSQL grava permissão, plano e os jobs elegíveis. Falha em
qualquer ponto, inclusive depois de gravar todos os jobs, desfaz reserva, contato
novo, permissão, plano e comando idempotente. Erro interno inesperado retorna
500 pelo contrato atual; falhas reconhecidas de dependência/deadline mantêm sua
classificação existente, sem expor SQL, código ou contato.

## Persistência mínima

A migration 00021 adiciona quatro tabelas:

- shop_notification_settings: política por unidade; ausência equivale à política
  padrão desligada. Leitura de uma política existente usa FOR SHARE para impedir
  que uma desativação passe despercebida durante a criação do plano.
- customer_booking_message_permissions: autorização daquela reserva, finalidade
  transacional, notice_version=1, origem pública/administrativa, ator quando
  administrativo, instante e eventual revogação. Pertence a customers, sem CRM.
- appointment_notification_plans: versão da política, geração, razões de dispensa
  e envelope cifrado quando há mensagens elegíveis. Consolida conteúdo e plano
  para evitar duas tabelas sem necessidade. O contexto autenticado usa a expiração
  persistida com precisão de microssegundos, sem truncar dados depois da cifragem.
- notification_jobs: confirmação/lembrete, prazos e estado pending/invalidated.
  Unicidade por unidade/reserva/tipo/geração e referências compostas protegem o
  isolamento e a duplicação. Outbox, attempts e estados de transporte chegam na
  entrega D; esta migration não antecipa seus adaptadores.

FK composta liga a autorização ao cliente da própria reserva, incluindo tenant.
Os novos dados têm RLS habilitada e privilégios de PUBLIC/anon/authenticated
revogados. O backend usa papel servidor autorizado; nenhuma tabela é consumida
diretamente pelo navegador.

## Invalidação também no banco

Em vez de repetir chamadas em cancelamento público, lifecycle administrativo e
conclusão automática, a migration conecta uma trigger AFTER UPDATE à reserva.
Ela só altera notification_jobs e invalida pending quando a reserva deixa de
estar scheduled/confirmed ou quando seus dados de agenda deixam de corresponder
ao snapshot. Não altera a reserva nem substitui as regras/auditoria de booking.

Triggers de revogação e desativação da política também invalidam jobs pendentes.
Tudo participa da transação que fez a alteração: um rollback de cancelamento
restaura simultaneamente o atendimento e seus jobs pending. Desligar o planejamento
no ambiente não desliga essas proteções já instaladas no banco.

O guard de INSERT de job adquire primeiro o lock do profissional e depois relê
reserva, plano e autorização. Isso impede um INSERT concorrente, inclusive por
SQL direto, de usar uma situação anterior ao cancelamento. Seguir essa ordem de
locks nos futuros consumers. O teste espera uma espera real de lock observada
com pg_blocking_pids, confirma o cancelamento e verifica a rejeição do job.

O futuro consumidor ainda precisa revalidar reserva, permissão e configuração
imediatamente antes da tentativa. Invalidação no banco não retira uma mensagem
externa já iniciada; mantém-se a fronteira descrita no ADR 0038.

## Contrato e compatibilidade

O campo opcional whatsapp_notifications tem padrão false no POST público e no
POST administrativo. No segundo caso, o ator autenticado declara que coletou a
autorização. O campo não permite enviar sem política habilitada, não comprova
posse do telefone e não autoriza marketing. A interface de coleta será criada
na entrega F; o frontend atual continua omitindo o campo.

A opção participa do fingerprint idempotente. False/omissão mantém exatamente o
JSON canônico anterior, inclusive campo vazio da chave, para recuperar recibos
criados antes do deploy. True acrescenta o campo. Alterar a autorização usando
a mesma chave gera conflito; não muda silenciosamente o pedido original.

NOTIFICATION_PLANNING_ENABLED=false mantém o fluxo anterior e não consulta as
tabelas novas. True exige migration 00021 e chaveiro servidor válido em
NOTIFICATION_PAYLOAD_KEYS, JSON com chaves base64 de 32 bytes, e identificação
ativa em NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID. O chaveiro é construído uma vez no
bootstrap; adaptadores transacionais reutilizam a proteção e recebem a Query
da transação corrente. Erros de configuração não imprimem valores secretos.

As configurações de exemplo ficam desligadas, sem alterar o .env do usuário.
Não aplicar backfill nem gerar mensagens para reservas antigas/replays.
Essas variáveis controlam planejamento; não habilitam uma entrega inexistente.

## Validação

Migration aplicada, revertida enquanto vazia e reaplicada num banco descartável;
schema copiado para bancos separados de notificações/booking/lifecycle, sem auth
e sem conexão com Supabase. Testes reais passaram para rollback após Save,
decifragem depois do round trip PostgreSQL, origem/ator da autorização, replay,
fingerprint anterior, concorrência, cancelamento público/owner, revogação,
desativação, conclusão manual/automática, ausência e isolamento de tenant.

As regressões de booking (reserva, disponibilidade, privacidade, cancelamento e
concorrência) e de lifecycle (permissões, auditoria, rollback e corridas) também
passaram. As demais suítes de integração não foram executadas nesta entrega.
O worker de transporte e a Evolution continuam pendentes; nenhum envio real foi feito.

Referência: [triggers executam na transação da alteração](https://www.postgresql.org/docs/current/trigger-definition.html).
