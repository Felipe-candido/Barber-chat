# Operação das notificações

Atualizado em 2026-11-02. Código implementado, habilitação explícita. Migrações
validadas em PostgreSQL local descartável; não aplicadas automaticamente no Supabase.
README e doc.go não fazem parte desta entrega.

## 1. Modo inicial: API e lembrete manual

Sem processo worker e sem RabbitMQ para este fluxo:

1. Cliente ou funcionário autoriza WhatsApp na criação do agendamento.
2. Booking grava reserva, permissão, snapshot cifrado, confirmação e lembrete
   na mesma transação. Sem autorização, a reserva continua funcionando.
3. Depois do commit, API tenta a confirmação por até 10s. A resposta mantém o
   comprovante e código mesmo se o envio falhar. Replay não envia novamente.
4. Proprietário ou barbeiro daquele atendimento abre os detalhes na agenda e
   aciona o lembrete. O servidor determina destinatário, conteúdo e janela.
5. Recibos autenticados atualizam aceitação/entrega/leitura quando disponíveis.

Confirmação expira em uma hora ou no início do atendimento, o que vier primeiro.
Lembrete em modo request expira um minuto antes do início. Cada propósito tem
uma intenção por geração da reserva. Não criar mensagens retroativas para
reservas anteriores ou sem autorização. Mudar política não altera planos existentes.

## 2. Configuração local

No PowerShell, a partir da raiz do projeto:

```powershell
cd C:\Users\felip\Barber-chat\backend-chat
. .\scripts\Load-Env.ps1
goose -dir .\db\migrations postgres "$env:DATABASE_URL" status
goose -dir .\db\migrations postgres "$env:DATABASE_URL" up
```

O último comando aplica até 00028 no banco escolhido pelo DATABASE_URL. Confira
o destino localmente; não publique a URL. Não executar essas migrations no banco
interno da Evolution. Não usar down como correção de um problema de configuração.

No `.env` privado do backend, configurar:

```dotenv
NOTIFICATION_PLANNING_ENABLED=true
NOTIFICATION_DELIVERY_ENABLED=true
NOTIFICATION_AUTOMATIC_ENABLED=false
NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID=local-2026-11
NOTIFICATION_PAYLOAD_KEYS={"local-2026-11":"SUA_CHAVE_BASE64_DE_32_BYTES"}
EVOLUTION_BASE_URL=http://127.0.0.1:8081
EVOLUTION_API_KEY=CHAVE_DA_SUA_INSTALACAO_EVOLUTION
EVOLUTION_WEBHOOK_SECRET=OUTRO_SEGREDO_ALEATORIO
EVOLUTION_WEBHOOK_URL=http://host.docker.internal:8082/api/v1/webhooks/evolution
EVOLUTION_WEBHOOK_ADDR=0.0.0.0:8082
```

As duas últimas linhas servem para Docker Desktop acessar o host Windows. API
administrativa pode continuar com HTTP_ADDR=127.0.0.1:8080. O listener 8082 aceita
somente o webhook, que exige autenticação. Restrinja-o ao ambiente local.

Gere dois segredos independentes localmente. Exemplo para gerar um valor base64:

```powershell
$notificationKeyBytes = New-Object byte[] 32
$notificationKeyGenerator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$notificationKeyGenerator.GetBytes($notificationKeyBytes)
[Convert]::ToBase64String($notificationKeyBytes)
$notificationKeyGenerator.Dispose()
[Array]::Clear($notificationKeyBytes, 0, $notificationKeyBytes.Length)
```

Execute novamente para o segredo do webhook; não reutilize o mesmo valor.
EVOLUTION_API_KEY deve corresponder à instalação local, em infra/evolution/.env;
esse arquivo usa EVOLUTION_API_KEY. Não enviar nenhum desses valores no chat,
logs, NEXT_PUBLIC_ ou Git. Preserve chaves antigas até vencerem todos os payloads
que dependem delas; reiniciar com outra chave torna esses conteúdos irrecuperáveis.

Inicie/preserve a instalação local conforme [Evolution local](evolution-local.md):

```powershell
docker compose --env-file .\infra\evolution\.env -f .\infra\evolution\compose.yaml up -d --wait
go run ./cmd/api
```

O backend carrega seu `.env`. Se já carregou variáveis no PowerShell antes de
editar o arquivo, recarregue Load-Env.ps1 ou abra um terminal novo para evitar
valores antigos no ambiente. Frontend, em outro terminal:

```powershell
cd C:\Users\felip\Barber-chat\frontend-chat
npm run dev
```

Use a origem exata configurada em FRONTEND_ORIGIN/NEXT_PUBLIC_API_BASE_URL.
Não misture localhost e 127.0.0.1 entre o navegador e a allowlist. Em banco remoto,
reserve tempo HTTP para o commit e a confirmação: por exemplo OPERATION_TIMEOUT=30s
e HTTP_TIMEOUT=60s, respeitando também os limites do proxy da hospedagem.

## 3. Teste prático com telefone

1. Entre como proprietário. Abra Configurações > WhatsApp e conecte pelo QR
   exibido no próprio Na Regua; escaneie em Aparelhos conectados no WhatsApp.
   Uma instância criada manualmente no gerenciador não se vincula à unidade.
2. Confirme estado conectado com verificação recente. Habilite notificações e
   escolha lembrete manual; salve. O modo automático fica indisponível sem worker.
3. Faça uma **nova** reserva futura com telefone de teste seu e autorização
   marcada. Confira que a reserva e o código aparecem mesmo se WhatsApp falhar.
4. Confira recebimento, preço, fuso, endereço e código no telefone. Na agenda,
   abra detalhes: envio aceito é distinto de entregue/lido.
5. Clique em Enviar lembrete. Atualizar/repetir o comando não deve repetir um
   envio aceito. Em resultado incerto, consulte o estado e o telefone; não
   refaça o agendamento nem force tentativas pelo banco.
6. Crie outra reserva autorizada e cancele antes de enviar seu lembrete; ele
   deve ficar indisponível. Desativar notificações também invalida pendências.
7. Conta de barbeiro só pode consultar/enviar nos seus atendimentos. Proprietário
   pode gerenciar todos os atendimentos da unidade e sua conexão.

QR fica apenas em memória, é descartado em até 20s e ao ocultar/sair da tela.
Não é salvo em localStorage, banco ou logs. Um QR expirado exige comando novo.
Pareamento e recebimento em telefone real dependem da ação do operador; testes
automatizados desta entrega não enviam mensagens para contatos reais.

## 4. Sem worker: manutenção explícita

O prazo impede enviar conteúdo vencido mesmo antes de limpar o banco. Para
remover ciphertext expirado, execute periodicamente um processo único:

```powershell
cd C:\Users\felip\Barber-chat\backend-chat
. .\scripts\Load-Env.ps1
go run ./cmd/notifications-maintenance
```

Usa DATABASE_URL e a configuração normal do backend. Não envia mensagens nem
conecta RabbitMQ. Remove até mil payloads vencidos e mil itens antigos por
categoria a cada execução; repetir para drenar grandes atrasos. Preserva
agendamentos, tentativas e metadados. Outbox consumida vence após 30 dias,
heartbeat após sete dias. Backups têm retenção própria e precisam de política
separada. Down da migration 28 recusa restaurar conteúdo já purgado.

## 5. Render Free

Executar somente o processo API; manter NOTIFICATION_AUTOMATIC_ENABLED=false e
política da unidade em request. Confirmação e lembrete são tentados dentro de
requisições ativas, sem depender de goroutine sobrevivendo depois da resposta.
Dormência/reinício não executa lembretes. Não prometer envio automático nesse modo.

Evolution precisa estar em um host acessível pela Render, com banco/sessões
persistentes e HTTPS. 127.0.0.1/host.docker.internal na Render não alcançam seu
PC. O host gratuito da API não hospeda esta instalação local da Evolution com
suas sessões persistentes. Use a URL HTTPS pública da API para o webhook; deixe
EVOLUTION_WEBHOOK_ADDR vazio e use o listener HTTP normal no PORT da plataforma.
Exponha a API via TLS, use segredos próprios de produção e origine CORS exata.

Fonte da limitação: [serviços gratuitos da Render](https://render.com/docs/free).
Este guia prepara a operação; não provisiona hospedagem ou conta de fornecedor.

## 6. Ativar automação futuramente

Em host contínuo, com PostgreSQL e RabbitMQ acessíveis:

1. Aplicar migrations e compartilhar **o mesmo keyring** entre API e worker.
2. Configurar Evolution e RABBITMQ_URL. Usar TLS/rede privada e credenciais próprias.
3. Configurar NOTIFICATION_AUTOMATIC_ENABLED=true na API e no worker; manter
   as outras duas flags true. Iniciar explicitamente `go run ./cmd/worker`.
4. Aguardar heartbeat saudável e, como owner, selecionar automático em
   Configurações, com antecedência de 5 minutos a sete dias; padrão 24h.
5. Criar nova reserva para além da antecedência. Confirmar envio após o commit
   e esperar lembrete no due_at persistido. Reservas dentro da antecedência
   recebem confirmação, com lembrete automático dispensado.

O scheduler verifica vencimentos a cada 5s. Relay usa confirms e roteamento
obrigatório; consumidor usa ACK após decisão durável. Não há TTL para aguardar
data da reserva. Reinício pode recuperar trabalho ainda dentro da janela;
mensagens vencidas não são enviadas em massa.

Reentrega AMQP não implica reenvio WhatsApp. Só falha comprovadamente anterior ao
envio permite retry limitado com backoff. Timeout/400 genérico/queda ambígua vira
unknown; recibo positivo pode resolver o estado. Não há exactly-once distribuído.

Observe logs estáticos de reconexão, scheduler e manutenção, e métricas agregadas
outbox_pending, uncertain, sending e oldest_due_seconds. DLQ é somente para
envelopes inválidos/limites de transporte; não reenvie uma DLQ sem investigar.
O worker executa manutenção a cada cinco minutos, substituindo o comando manual.

## 7. Verificação de código

Go: `go test ./...`, `go vet ./...`; build de cmd/api, cmd/worker e
cmd/notifications-maintenance. Frontend: `npm run typecheck`, `npx playwright test`.
Testes integration exigem bancos locais descartáveis explicitamente indicados
pelos ambientes NOTIFICATIONS_TEST_DATABASE_URL e WHATSAPP_TEST_DATABASE_URL.
Broker real usa NOTIFICATIONS_TEST_RABBITMQ_URL em 127.0.0.1:55442. Sem essas
variáveis os testes opt-in são pulados, o que não significa validação real.

Decisões: ADRs 0038–0045 e contrato [OpenAPI de notificações](notifications-openapi.yaml).
