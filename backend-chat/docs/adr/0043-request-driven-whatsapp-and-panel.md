# ADR 0043 — Confirmação pós-commit, lembrete manual e painel WhatsApp

Data: 2026-11-01. Estado: F1 implementado e validado localmente. Pareamento e
recebimento em telefone real pendentes; execução automática é a entrega D2.

## Modo inicial e persistência

Migration 00025 adiciona delivery_mode à política e dispatch_mode ao job. Políticas
existentes preservam automatic; novas políticas têm padrão request. O modo é
copiado na criação do plano. Alterações de modo/antecedência afetam novas reservas;
desabilitar continua invalidando todas as intenções ainda pendentes.

No modo request, confirmação vence imediatamente e expira na janela original;
lembrete vence created_at+1µs e expira starts_at−1min. Essa janela faz parte da
cifragem desde a criação. Um único job por tipo/geração continua evitando dois
lembretes para a mesma reserva. Não reabrir a janela de job automático antigo
nem estender ciphertext expirado ao clicar. Reservas próximas demais podem não
ter janela de lembrete, sem impedir confirmação ou agendamento.

Essa decisão substitui a proposta de intenção manual separada da seção 10.1:
o modo é escolhido antes do plano, sem criar dois trabalhos concorrentes para a
mesma finalidade. Jobs request nunca são selecionados pelo scheduler automático.

## Configuração e autorização

NOTIFICATION_DELIVERY_ENABLED=false por padrão. True exige planejamento,
chaveiro e Evolution configurados. NOTIFICATION_AUTOMATIC_ENABLED também começa
false e controla disponibilidade do modo automático; exige delivery habilitado.
Na Render inicial, automático permanece false e não se inicia cmd/worker.

ManageNotificationSettings usa transações owner da conexão e CAS por version.
Versão 0 representa ausência de política; primeira gravação cria versão 1, sem
overwrite concorrente. Corpo HTTP exige enabled, modo, antecedência e versão;
campos desconhecidos e configuração automática indisponível são recusados.

ManageBookingMessages só expõe metadados. Proprietário acessa a unidade toda;
barbeiro precisa de vínculo ativo com o profissional da reserva. DeliveryKey
carrega ator para comandos administrativos, e DeliverNotification repete essa
autorização nas duas transações antes do sender. Desativar vínculo durante a
abertura do payload bloqueia o envio. Chamadas internas do booking/worker usam
referência confiável sem fingir um ator administrativo.

## Reserva e contrato HTTP

CreateAppointmentOutput inclui shop_id interno separado do comprovante estável.
Transportes público/staff chamam BookingConfirmationDispatcher somente após o
retorno bem-sucedido do caso de uso, portanto após commit. Bridge em notifications
liga essa porta ao núcleo de entrega. Replay consulta estado sem iniciar envio.
Falha de WhatsApp não muda 201/200 nem refaz reserva: resposta acrescenta whatsapp
opcional, com state sanitizado, somente quando mensagens foram solicitadas.

Não usar goroutine descartável nem depender do browser para disparar confirmação.
Sem worker no modo request, pane entre commit e chamada pode deixar confirmação
pendente. A agenda oferece ação explícita para essa intenção comprovadamente
não iniciada e ainda válida. Resultado incerto permanece bloqueado para repetição.

Rotas e DTOs reais estão em notifications-openapi.yaml. Opções públicas mostram
somente whatsapp_enabled e passam pela regra de publicação/assinatura da unidade.
Opções staff funcionam na unidade autorizada, inclusive antes de publicação.
Todas as rotas privadas usam Bearer e escopo; preferências/conexão são owner-only.
Não devolver telefone, código, conteúdo cifrado, provider ID ou token de operação
nos status. Respostas são no-store; webhooks continuam autenticados separadamente.

## Interface

WhatsAppSettings integra a página de configurações preservando componentes e
tokens existentes: estado textual, última verificação, QR temporário, conectar,
desconectar, conferir estado e preferências. QR só fica na memória, é retirado na
expiração/ocultação da aba e não entra em localStorage. POST incerto exige consulta
antes de outro comando. Atualizações periódicas são GET, com cancelamento no unmount.

Opt-in público começa desmarcado e é opcional. No modo staff, a opção registra que
o funcionário coletou a autorização. O consentimento participa dos dados congelados
da tentativa idempotente. O comprovante separa reserva salva de envio aceito/incerto.
AppointmentNotifications mostra confirmação/lembrete na agenda e oferece ação só
quando o servidor informa can_send. Conferência usa GET; unknown não cria retry.

## Verificação

PostgreSQL comprova janela cifrada manual, CAS, escopo owner/barber, revogação
antes do sender e confirmação executada depois de reserva/plano/jobs visíveis
em outra conexão. Falha simulada do provedor mantém booking e replay corretos.
HTTP testa Bearer, proprietário, tenant, corpos e CORS. Passaram 26 testes de
navegador de WhatsApp, opt-in/comprovante, agenda e configurações; telas desktop
e celular foram inspecionadas. Testes utilizam mocks e banco local descartável.
Não houve envio real, mudança do .env do backend ou aplicação no Supabase.
