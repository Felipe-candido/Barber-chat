# Notificações de agendamentos — execução e estado final

Atualizado em 2026-11-02. Entregas A–G implementadas. Backend, painel e worker
estão conectados; habilitação depende das flags e configuração do ambiente.
Testes automatizados usam dados locais descartáveis e sender simulado. O teste
de QR/callback usa Evolution local, sem parear telefone nem enviar a terceiros.
A homologação com telefone real e a implantação em nuvem pertencem ao operador.

## Produto e limites

Um número WhatsApp por barbearia, configurado pelo proprietário. A reserva
continua sem cadastro de cliente obrigatório. Confirmação e lembrete exigem
autorização explícita por agendamento; telefone cadastrado não equivale a consentimento.

O primeiro deploy pode usar somente a API no Render Free: confirmação após
commit e botão de lembrete na agenda. Worker fica pronto, porém não inicia na
API nem é necessário para o envio manual. Automatização exige processo contínuo
separado, RabbitMQ e heartbeat saudável. Não hospedar sessões da Evolution no
filesystem efêmero do serviço gratuito da API.

Não fazem parte desta entrega: campanhas, mensagens de cancelamento, respostas
SIM/NÃO, IA, gerenciamento de clientes, cortesia comercial ou provisionamento AWS.
Cancelar e alterar o lifecycle já invalidam envios pendentes.

## Entregas concluídas

| Etapa | Implementação e fronteira |
| --- | --- |
| A — regras | Política temporal, janelas UTC, antecedência 5min..7dias, código privado e contrato de payload |
| B — reserva transacional | Reserva + autorização + snapshot AES-GCM + jobs na mesma transação, triggers de invalidação |
| C — conexão | Owner conecta/consulta/desconecta; QR efêmero; lease/CAS; monitor e callback autenticado |
| D1 — núcleo de entrega | Claim, abrir conteúdo fora de TX, revalidar, iniciar tentativa, HTTP sem locks, persistir resultado |
| E — Evolution | Template versionado com código, endereço, preço e fuso; correlação por tentativa; incerteza sem retry cego |
| F — API/painel | Confirmação pós-commit, lembrete manual, opt-in, status e preferências da unidade |
| D2 — worker | Scheduler, outbox, relay, confirms, mandatory, quorum/DLQ, ACK manual, recovery, reconexão e heartbeat |
| G — operação | Recibos monotônicos, retenção, métricas, manutenção sem worker e guia de homologação |

## Arquitetura final

Monólito modular Go, um go.mod. API e worker são executáveis independentes que
reutilizam os mesmos casos de uso; não são microsserviços separados.

| Área | Arquivos centrais / responsabilidade |
| --- | --- |
| booking/application | Declara portas de planejamento e confirmação; não importa infra de notifications |
| booking/infra/http | Tenta confirmação somente após commit; preserva recibo e sucesso da reserva |
| customers/infra/postgres | Permissão de mensagens por reserva e telefone normalizado |
| notifications/domain | policy.go, connection.go, delivery.go: regras e estados |
| notifications/application | plan_booking.go, prepare_booking.go, deliver.go, message.go, booking_messages.go, settings.go, dispatch.go, receipts.go |
| notifications/infra/postgres | Adaptadores pequenos para planos, entrega, conexão, configuração, outbox e recibos |
| notifications/infra/evolution | Contratos do fornecedor, envio HTTP único e tradução de erros/evidências |
| notifications/infra/http | DTOs públicos e administrativos; comandos não recebem destino ou texto livre |
| notifications/infra/rabbitmq | Topologia, publicação confirmada, envelopes estritos e consumo com ACK |
| notifications/infra/worker.go | Supervisor, scheduler, monitor, manutenção e métricas |
| cmd/api | Monta HTTP, adaptadores e monitor; não conecta broker para notificações |
| cmd/worker | Monta processo contínuo somente com flag de automação |
| cmd/notifications-maintenance | Limpeza explícita para ambiente sem worker |
| frontend | Configurações WhatsApp, consentimento na reserva e botões/status na agenda |

Os adaptadores dos módulos donos dos dados implementam portas de notifications.
Queries da mesma transação são compartilhadas. Nenhuma chamada Evolution/AMQP
acontece segurando locks de calendário ou transação SQL.

## Fluxo inicial sem worker

1. Frontend consulta disponibilidade e apresenta WhatsApp marcado por padrão,
   permitindo desmarcar antes da confirmação. Omissão/false mantém fingerprint
   antigo de idempotência.
2. API autentica/autoriza quando administrativo; agendamento público usa as
   barreiras de publicação/assinatura e validação já existentes.
3. Booking salva reserva, contato, autorização e plano cifrado com jobs na mesma
   transação. Sem autorização/política ativa, salva reserva normalmente.
4. Após commit, handler tenta confirmação com prazo de 10s. Falha de mensagem
   não desfaz a reserva. Replay consulta estado, sem novo envio.
5. Lembrete é acionado nos detalhes da agenda, por owner ou barbeiro responsável.
6. Webhook autenticado registra aceitação/entrega/leitura da tentativa conhecida.
   A tela de comprovante mantém o código independente do resultado WhatsApp.

Modo request planeja lembrete desde a criação até um minuto antes do atendimento.
Confirmação expira em min(criação + 1h, início). O conteúdo é cifrado com a
expiração já autenticada; não ampliar depois uma janela por edição SQL.
Reservas antigas/sem plano não recebem backfill de código.

## Automação pronta para ativação posterior

Modo automatic calcula due_at = início - antecedência (padrão 24h). Se a reserva
for criada dentro dessa antecedência, lembrete é dispensado, sem duas mensagens
simultâneas. Janela do lembrete: até min(due_at + 1h, início - 1min).
“24h antes” significa tempo decorrido; não é “dia civil anterior às 18h”.

Scheduler consulta jobs automáticos vencidos, incrementa dispatch_seq e grava
outbox atomicamente. Relay publica apenas IDs/versão/sequência com confirmação
do broker e mandatory. Falha de confirmação conserva trabalho para republicação
do mesmo envelope. Consumidor revalida o banco e só dá ACK após decisão durável.
Enviar duplicata AMQP não significa repetir uma tentativa WhatsApp.

Heartbeat exige sessão AMQP ativa e scheduler válido. A opção automática aparece
com flag global e heartbeat saudável nos últimos dois minutos. API e worker
precisam compartilhar keyring. Políticas editadas só valem para novas reservas.

## Integridade, incerteza e privacidade

- Uma intenção por shop/appointment/kind/schedule_generation.
- Tenant nas FKs, autorização owner/barbeiro do atendimento e RLS server-only.
- Consentimento, reserva ativa, telefone, política, janela e conexão são
  revalidados antes de iniciar I/O; cancelamento/desativação invalidam pendências.
- Lease com token por tentativa; lease durável serializa sender por barbearia.
- Sending abandonado vira unknown. Lease anterior ao início é recuperável.
- Somente not_sent/provider_unavailable comprovado permite retry automático
  com backoff de 30s exponencial, teto 15min e máximo cinco tentativas na janela.
- HTTP 400 genérico da Evolution pode ocorrer depois do efeito; timeout/reset
  também não prova ausência de envio. Unknown nunca entra em retry cego.
- Recibos exigem header HMAC por instância, fromMe e ID aleatório conhecido.
  Aceitação, entrega e leitura avançam sem regredir. Não alteram presença.
- Sem exactly-once distribuído. “Aceito” não garante entrega ao aparelho.
- Telefone/texto/código/QR não aparecem em logs ou envelopes. Código recuperável
  só no snapshot AES-GCM; QR só em memória do proprietário por até 20s.
- Payload vencido é removido por manutenção, preservando histórico. Outbox
  consumida é retida 30 dias, heartbeat sete dias. Backups têm política própria.

## Banco e compatibilidade

| Migration | Mudança |
| --- | --- |
| 00021 | Política, consentimento, planos cifrados e jobs |
| 00022 | Conexão WhatsApp por unidade |
| 00023 | Tentativas, estados/leases de entrega |
| 00024 | Versão do template persistida antes do envio |
| 00025 | Modo request/automatic; novas políticas começam em request |
| 00026 | Outbox, sequência, sender lease e heartbeat |
| 00027 | Estado e instante do recibo monotônico |
| 00028 | Purga de payload com metadados preservados |

Migrations são explícitas, não executadas pelo bootstrap. A validação é local;
não houve aplicação automática desta etapa no Supabase. Down não inventa conteúdo
já purgado e recusa rollback destrutivo de história de novas estruturas.
Planos antigos mantêm seu modo/janela; replay não os recria.

## Validação e encerramento

Cobertura: regras e template; contratos HTTP e webhook; política/CAS; limites de
permissões/tenancy; privacidade; commit antes do envio; reserva válida sob falha;
PostgreSQL real com concorrência e fencing; RabbitMQ real com ACK/confirm perdidos,
sem rota, DLQ e reconexão TCP; retenção e reconciliação. Frontend exercita
configuração/QR, reserva com e sem autorização e estado incerto sem repetir ações.

Guia com comandos, configuração local, Render, manutenção e ativação futura:
[Operação das notificações](notifications-operations.md).
Contratos: [OpenAPI](openapi.yaml), [notificações](notifications-openapi.yaml).
Decisões: ADRs 0038–0045.

A pendência prática é parear um telefone de teste e conferir mensagens reais.
Não declarar esse recebimento validado apenas por testar sender falso, QR,
callback ou contrato HTTP. AWS e o próximo módulo serão planejados separadamente.
