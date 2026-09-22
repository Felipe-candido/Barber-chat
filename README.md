# Barber-chat

Fundação de um SaaS de agendamentos para barbearias e projeto de estudo de Go, Chi e mensageria. Uma aplicação modular, um módulo Go e dois executáveis: API e worker. PostgreSQL guarda os dados; RabbitMQ será o transporte das notificações.

**Implementado:** configuração por ambiente/.env, HTTP com Chi, health/readiness, logs JSON, graceful shutdown, pool PostgreSQL, conexão AMQP, ciclo de vida do worker, Compose, migrations e testes. O catálogo agora cria serviços em modo local explícito e lista serviços ativos por slug. **Ainda não existe:** edição/ativação de serviços, autenticação administrativa, agendamento, scheduler/outbox, consumer de negócio, envio de mensagens, frontend ou relatórios. O worker verifica dependências e aguarda encerramento; não envia notificações.

**Estrutura de negócio:** sete módulos em `internal/modules`, cada um com `domain`, `application` e `infra`. Catálogo implementa criar/listar serviços; shops fornece resolução de slug ativo. Os demais fluxos continuam planejados. Os `doc.go` do catálogo explicam o fluxo completo, cada camada, os adaptadores e a fronteira de acesso local. Veja [como testar serviços](docs/catalog-testing.md) e o [ADR 0007](docs/adr/0007-catalog-services-and-local-tenant.md).

## Requisitos

- Go 1.27.1 ou patch posterior compatível ([download oficial](https://go.dev/dl/)). Após instalar no Windows, abrir um novo terminal para atualizar o PATH.
- Docker Desktop com engine Linux em execução e Docker Compose v2.
- PowerShell. Comandos abaixo executados na raiz do projeto.
- Goose v3.28.0 para migrations. É ferramenta externa; não integra os binários da aplicação.

As versões e alternativas estão em [docs/architecture.md](docs/architecture.md). O module path `github.com/Felipe-candido/Barber-chat` foi derivado do remote existente.

Não há dependência de `.tools`: a pasta temporária da primeira validação foi removida. Use a instalação normal de Go e seus caches padrão (`go env GOCACHE GOMODCACHE`). Goose pode ser instalado separadamente pelo comando abaixo quando precisar executar migrations.

Se você vem de Python, Django ou FastAPI, leia o [guia Go para Python/Django/FastAPI](docs/go-from-python-django-fastapi.md). Ele explica as linhas do código atual, `context`, `defer`, canais, goroutines, pools e a organização dos módulos usando equivalências práticas.

## Módulos de negócio

| Pacote em `internal/modules` | Responsabilidade |
| --- | --- |
| `shops` | Barbearia/tenant, slug e timezone |
| `catalog` | Profissionais, serviços e associações |
| `customers` | Contatos e consentimentos, sem conta de acesso |
| `booking` | Agendamentos, disponibilidade, expediente, folgas e bloqueios |
| `notifications` | Intenções, lembretes, tentativas e resultados de envio |
| `identity` | Usuários administrativos, membership e permissões |
| `reporting` | Relatórios derivados dos agendamentos e seus snapshots |

Cada módulo organiza três pacotes: `domain` para regras/tipos de negócio, `application` para casos de uso/portas e `infra` para adaptadores concretos como repositórios PostgreSQL, handlers HTTP e clientes externos. A raiz do módulo é apenas um diretório organizador. Não são sete módulos Go nem sete serviços: continua existindo um go.mod e dois executáveis.

`internal/platform` continua responsável por conexões e recursos genéricos compartilhados. A futura implementação em `booking/infra` receberá o pool ou a transação e executará SQL de agendamentos; não abrirá um novo pool próprio. A montagem do servidor continua em `internal/httpapi` e a composição das dependências em `cmd`. A infraestrutura de cada módulo não pode ser importada pelo seu domínio ou pelos seus casos de uso.

```powershell
go doc ./internal/modules/booking/domain
go doc ./internal/modules/booking/application
go doc ./internal/modules/booking/infra
```

O próximo fluxo real deve criar cliente/agendamento/intenção de notificação atomicamente, apesar da separação de responsabilidades. Veja o [ADR das camadas por módulo](docs/adr/0005-explicit-module-layers.md).

## Preparar o ambiente

```powershell
go version
docker version
docker compose version
if (!(Test-Path .env)) { Copy-Item .env.example .env }
docker compose config --quiet
docker compose up -d --wait
go mod download
```

A API e o worker leem automaticamente o `.env` do diretório de execução por meio de `config.Load()`. Execute os comandos na raiz do projeto. Variáveis já definidas no terminal têm prioridade, inclusive valores vazios; o arquivo é opcional quando o ambiente já fornece a configuração. Reinicie o processo após editar o arquivo. Se você carregou valores antigos com o script, abra um novo terminal antes de iniciar a aplicação.

Compose também lê `.env`. O script `scripts/Load-Env.ps1` continua disponível para migrations e testes de integração: importa `KEY=VALUE` para a sessão sem executar/interpolar conteúdo. Credenciais do exemplo são exclusivas para desenvolvimento local. Não usar em produção. Alterações em senhas de containers com volumes existentes não reconfiguram automaticamente usuários já criados.

Portas locais: PostgreSQL `127.0.0.1:55432` (porta interna 5432), AMQP `127.0.0.1:5672`, painel RabbitMQ [localhost:15672](http://localhost:15672), API [localhost:8080](http://localhost:8080). A porta externa 55432 evita o bloqueio encontrado na 5432 deste Windows. Se alterar portas/credenciais em `.env`, ajustar também `DATABASE_URL`/`RABBITMQ_URL`; credenciais em URLs precisam de percent-encoding quando tiverem caracteres especiais.

## Migrations

Instalar a CLI fixada com Go; alternativamente, obter o executável e validar SHA-256 na [release oficial do Goose](https://github.com/pressly/goose/releases/tag/v3.28.0).

```powershell
go install github.com/pressly/goose/v3/cmd/goose@v3.28.0
. .\scripts\Load-Env.ps1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
# Se GOBIN estiver configurado, usar o goose.exe desse diretório.
& $goose -dir db/migrations validate
& $goose -dir db/migrations postgres $env:DATABASE_URL up
& $goose -dir db/migrations postgres $env:DATABASE_URL status
```

As migrations habilitam `btree_gist`, criam shops/services e adicionam a moeda BRL. Goose cria sua tabela de versões. O Down da primeira mantém a extensão para não apagar objetos compartilhados. O seed de exemplo fica em db/seeds/development.sql e é executado separadamente. Ver [db/migrations/README.md](db/migrations/README.md). Não executar migrations automaticamente no boot dos processos.

## Executar API e worker

Terminal 1:

```powershell
go run ./cmd/api
```

Terminal 2:

```powershell
go run ./cmd/worker
```

Terminal 3:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health
Invoke-RestMethod http://127.0.0.1:8080/ready
```

`/health` retorna `{"status":"ok"}` sem consultar dependências. `/ready` retorna `{"status":"ready"}` ou HTTP 503 com `{"status":"unavailable"}` conforme PostgreSQL; não valida schema nem RabbitMQ. Assim a API pode iniciar com o banco indisponível e reportar readiness negativa. A API não exige `RABBITMQ_URL`; o worker exige e encerra com erro quando não consegue conectar ou perde conexão. Neste scaffold a recuperação do worker é por reinício manual. Não há dados de negócio para perder.

Ctrl+C encerra os processos. Para verificar sinais diretamente no Windows, preferir os binários compilados em vez do wrapper `go run`:

```powershell
New-Item -ItemType Directory -Force bin | Out-Null
go build -o bin/api.exe ./cmd/api
go build -o bin/worker.exe ./cmd/worker
.\bin\api.exe
# Em outro terminal, na raiz do projeto:
.\bin\worker.exe
```

## Configuração

| Variável | Padrão / obrigação | Uso |
| --- | --- | --- |
| `HTTP_ADDR` | `127.0.0.1:8080` | Interface/porta HTTP |
| `DEV_SHOP_SLUG` | Vazio (escrita desabilitada) | Slug para testes de criação; exige listener loopback; não é autenticação |
| `DATABASE_URL` | Obrigatória | PostgreSQL; pool de até 5 conexões por processo, sessão UTC |
| `RABBITMQ_URL` | Obrigatória só para worker | AMQP/AMQPS |
| `LOG_LEVEL` | `INFO` | Nível slog (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `DB_TIMEOUT` | `3s` | Timeout de probes PostgreSQL e conexão inicial PostgreSQL/AMQP |
| `SHUTDOWN_TIMEOUT` | `10s` | Drain HTTP e limite de fechamento AMQP |
| `WORKER_INTERVAL` | `30s` | Intervalo da verificação PostgreSQL do worker |

Valores inválidos falham cedo sem imprimir credenciais. Um `.env` existente com erro de leitura ou sintaxe também impede a inicialização, com erro sanitizado. O parser `godotenv` v1.5.1 aceita comentários e aspas; use aspas simples para valores com `$` literal, pois valores sem aspas ou com aspas duplas permitem interpolação. O arquivo é lido como configuração, sem alterar o ambiente do terminal. Em produção, forneça variáveis pela plataforma e dispense o arquivo. Veja o [ADR do carregamento automático](docs/adr/0006-automatic-dotenv.md).

Timezone da barbearia será dado de negócio (`America/Sao_Paulo` inicialmente), e não timezone global do processo. Não há configuração de provedor de mensagens nesta etapa.

## Validação

```powershell
gofmt -w cmd internal tests
go mod tidy
go vet ./...
go test ./...
go build -o bin/api.exe ./cmd/api
go build -o bin/worker.exe ./cmd/worker
docker compose --env-file .env.example config --quiet
. .\scripts\Load-Env.ps1
& $goose -dir db/migrations validate
& $goose -dir db/migrations postgres $env:DATABASE_URL up
go test -tags=integration -count=1 ./tests/integration/...
git diff --check
git status --short
```

Integração completa requer PostgreSQL/RabbitMQ disponíveis, env carregada e migrations aplicadas. Falha se faltar configuração; não pula silenciosamente. Testa pool/UTC/extensão e roundtrip AMQP com fila temporária. Os testes TestCatalog usam transações revertidas para validar POST, persistência, isolamento de duas barbearias, serviços/barbearias inativos e constraints. Eles podem rodar separadamente com `-run TestCatalog` sem RabbitMQ. Não comprova ainda outbox, retry, cancelamento ou double booking.

`go test -race ./...` é recomendado em CI Linux ou Windows com toolchain C compatível. Hot reload com Air e golangci-lint/Staticcheck são opções futuras; não são pré-requisitos. O mínimo atual usa as ferramentas oficiais Go. Nenhum Makefile é necessário.

Para inspecionar/parar dependências preservando dados:

```powershell
docker compose ps
docker compose logs --tail 50
docker compose stop
```

Não remover volumes para resolver erros de configuração. O resultado das validações desta entrega está em [docs/validation.md](docs/validation.md).

## Leitura e próximo incremento

- [Arquitetura e comparações](docs/architecture.md): limites, concorrência, tenant, timezones, outbox e notificações.
- [ADRs](docs/adr/): decisões e trade-offs registrados.
- [Contrato proposto para frontend](docs/api-contract.md) e [OpenAPI atual](docs/openapi.yaml).
- [AGENTS.md](AGENTS.md): instruções persistentes para próximas tarefas.

Próximo passo do catálogo: edição e ativação/desativação, listagem administrativa e autorização por membership antes de publicação. Depois profissionais/associações e a fatia de agendamento com proteção de sobreposição, seguida de outbox/worker. Definir regras de lembrete, cancelamento, seleção de profissional e canal/provedor antes das funcionalidades correspondentes.
