# Registro de validações

## Script de provisionamento de barbearia e usuário — 02/10/2026

Criado db/supabase/provision_shop_and_user.sql para execução manual no SQL Editor
do Supabase após as duas sequências de migrations. Usa uma conta Auth existente
para criar perfil, barbearia e vínculo numa única transação. Não cria credenciais,
não altera nomes existentes e não reativa acessos suspensos.

| Verificação desta etapa | Resultado |
| --- | --- |
| gofmt, go mod tidy, go vet ./... e go test ./... | PASS |
| go build ./cmd/api ./cmd/worker | PASS |
| Compilação dos testes com tag integration | PASS |
| TestIdentityMigrations em PostgreSQL real | PASS; suíte anterior e 22 novos cenários do script |
| Compose config e goose validate | PASS |
| Execução no Supabase real do usuário | NÃO EXECUTADA; o cadastro permanece manual |

Os novos cenários verificam criação e repetição, UUID do provedor preservado,
ID independente da barbearia, múltiplas unidades/usuários, preservação do perfil,
nomes com apóstrofo, validação de entradas, rejeição de acessos suspensos,
ausência de registros órfãos, colisão de slug e exigência de Auth/FK existentes.
A fixture mínima de auth.users valida integridade SQL, não login no provedor.

Foi utilizado exclusivamente o banco vazio e descartável
codex_provision_20261002_01a0c16e no PostgreSQL local do Compose. Sua ausência foi
conferida antes da criação; ele foi removido ao final e a remoção foi confirmada.
Nenhum dado de aplicação, projeto Supabase, conta real, .env ou volume foi
alterado. Não houve commit/push, dependência nova ou mudança de endpoint.

## Seleção de unidades e integração frontend/backend — 02/10/2026

Fluxo implementado: login no Supabase, validação da identidade local em /admin/me,
consulta das unidades ativas em /admin/shops, escolha explícita e painel por slug.
GET e POST administrativos de serviços revalidam usuário, unidade e membership.
A rota POST /admin/services e a configuração de slug fixo foram removidas.
Usuários, unidades e vínculos continuam provisionados manualmente.

| Verificação desta etapa | Resultado |
| --- | --- |
| gofmt, go mod tidy, go vet ./... e go test ./... | PASS |
| go build ./cmd/api ./cmd/worker | PASS |
| sqlc compile | PASS |
| docker compose --env-file .env.example config --quiet | PASS |
| goose -dir db/migrations validate | PASS |
| Integração PostgreSQL: migrations e repositório identity | PASS; inclui 15 cenários de leitura de unidades elegíveis, provisionamento e constraints |
| Integração PostgreSQL: catálogo, persistência e isolamento | PASS; leitura administrativa, criação, vínculo revogado e isolamento entre unidades |
| go test -tags=integration ./tests/integration -run '^$' | PASS somente de compilação |
| Frontend: npm run typecheck e npm run build | PASS; build normal de produção concluído |
| Frontend: Playwright | PASS; 26 testes em build de produção isolado, com HTTP do Supabase e da API simulado |
| Frontend: Prettier nos arquivos da tarefa | PASS; src, tests, README, API-INTEGRATION, configurações Next/Playwright e tsconfig |
| Frontend: npm run format:check global | FAIL; seis arquivos preexistentes fora desta tarefa, listados abaixo |
| git diff --check | PASS |
| Login real em um projeto Supabase | NÃO EXECUTADO; requer conta e configuração reais compatíveis |
| Integração RabbitMQ nesta etapa | NÃO EXECUTADA; autenticação não depende do broker e a porta local já era usada por outro projeto |
| Race e validação especializada do schema OpenAPI | NÃO EXECUTADOS |

O PostgreSQL 18.6 foi iniciado pelo Compose do projeto na porta local 55432 e
permanece disponível. Os testes reais usaram exclusivamente o banco temporário
codex_auth_20261001_01a0c16e, criado vazio depois de verificar sua ausência. As
migrations foram aplicadas somente nesse banco, removido ao final e cuja ausência
foi confirmada. Nenhum volume, banco de aplicação ou container de outro projeto
foi removido. O provisionamento de Auth nos testes de migrations é uma fixture
transacional local, não a criação de uma conta no Supabase real.

Os testes HTTP Go verificam JWT ES256 e JWKS com TLS local. Os testes Playwright
cobrem login, ordem /me → /shops, seleção/troca de unidade, restauração de sessão,
logout, lista vazia, token ausente/inválido, perfil sem acesso, slug desconhecido,
contratos inválidos, respostas atrasadas, timeout ao restaurar sessão, catálogo
público sem Bearer e layouts móveis. Não comprovam um login real no provedor.
As tentativas iniciais de navegador tiveram falhas de tempo de compilação e
seletores; após os ajustes, a execução completa final passou nos 26 testes.

O format:check global ainda aponta .prettierrc.json, AGENTS.md, BRANDING.md,
CLAUDE.md, package.json e postcss.config.mjs. Esses arquivos não foram
reformatados por serem alterações preexistentes fora do escopo. O contrato
OpenAPI foi revisado junto às rotas, mas não foi instalado um validador adicional.

Não foram alterados .env/.env.local reais, credenciais, contas ou vínculos de
aplicação. Não houve commit/push nem dependência nova. Agendamentos, papéis de
permissão e revogação imediata de JWT pelo provedor continuam fora desta etapa.
Os registros abaixo são históricos e não descrevem o estado atual das rotas.

## Autenticação e autorização HTTP — 30/09/2026

Bootstrap com verifier compartilhado, middleware Bearer, identidade em /admin/me
e criação de serviços com membership por unidade. A rota legada continua exigindo
os mesmos checks. O .env real, contas, credenciais, migrations e banco configurado
não foram alterados. Sem commit/push ou dependência nova.

| Verificação desta etapa | Resultado |
| --- | --- |
| gofmt nos arquivos alterados e go mod tidy | PASS |
| go vet ./... | PASS |
| go test ./... | PASS; JWT/JWKS via TLS local, identidade esperada, headers malformados, isolamento de unidades, inativos, CORS, timeout e guards de contexto |
| go build ./cmd/api ./cmd/worker | PASS |
| go test -tags=integration -run '^$' ./tests/integration/... | PASS somente de compilação; nenhum teste de integração executado |
| docker compose --env-file .env.example config --quiet | PASS |
| goose -dir db/migrations validate | PASS somente da estrutura dos arquivos; nenhuma migration aplicada |
| git diff --check | PASS |
| Docker/integração PostgreSQL e RabbitMQ reais | INDISPONÍVEL; pipe dockerDesktopLinuxEngine ausente |
| Sessão/login real no projeto Supabase | NÃO EXECUTADO; testes usam tokens ES256 e JWKS locais |

A suíte HTTP usa o verifier, os casos de uso e os handlers reais, substituindo
somente a persistência. Não comprova integridade PostgreSQL nem configura o Auth
real. Os testes de catálogo com tag integration foram adaptados para verificar
membership com repositório real; executar quando o banco de desenvolvimento
estiver disponível e com migrations aplicadas. OpenAPI/contrato foram atualizados;
não houve validação completa do schema OpenAPI com ferramenta especializada.
Race continua recomendado em CI com toolchain C compatível; não foi executado
nesta etapa. Os registros abaixo são históricos.

## Catálogo de serviços — 22/09/2026

Implementados criação local explícita, listagem pública por slug, resolução de tenant via shops, validação e moeda BRL, com documentação nos doc.go. Nenhum commit/push foi feito. O .env e o Supabase não foram alterados.

| Verificação desta etapa | Resultado |
| --- | --- |
| gofmt e go mod tidy | PASS |
| go vet ./... e go test ./... | PASS; domínio, aplicação, HTTP, configuração e testes anteriores |
| Builds cmd/api e cmd/worker | PASS |
| sqlc generate e sqlc compile | PASS |
| goose -dir db/migrations validate | PASS; estrutura dos arquivos, não execução no banco |
| docker compose --env-file .env.example config --quiet | PASS |
| go doc ./internal/modules/catalog/infra | PASS; documentação do fluxo renderizada |
| git diff --check | PASS |
| go test -tags=integration -count=1 ./tests/integration/... | FAIL por dependências locais indisponíveis: transação/ping PostgreSQL e conexão RabbitMQ |
| Aplicação das migrations em PostgreSQL real | NÃO EXECUTADA nesta etapa; engine Docker indisponível |

O Docker Desktop foi iniciado, mas o pipe dockerDesktopLinuxEngine continuou ausente. A integração foi tentada explicitamente contra as URLs locais de .env.example, sem acessar o Supabase. A suíte compilou, mas nenhum comportamento de persistência do catálogo é declarado aprovado por esse resultado. Rodar migrations e integração novamente com PostgreSQL/RabbitMQ disponíveis; instruções em [Testar catálogo](catalog-testing.md).

Os registros abaixo são históricos e não substituem essa validação mais recente.

## Fundação — 17/09/2026

Execução em 17/09/2026 UTC (16/09 à noite em America/Sao_Paulo), Windows amd64, Go 1.27.1 instalado pelo usuário, Docker Desktop com engine Linux. O repositório inicialmente continha somente README.md, sem alterações pendentes. Remote e branch foram preservados; nenhum commit/push foi feito.

| Validação executada | Resultado |
| --- | --- |
| `gofmt -w cmd internal tests` e `gofmt -l cmd internal tests` | PASS; nenhum arquivo restante sem formatação |
| `go mod tidy` | PASS; dependências diretas fixadas e go.sum gerado |
| `go mod verify` | PASS; todos os módulos verificados |
| `go vet ./...` | PASS |
| `go test ./...` | PASS; configuração, HTTP/readiness, métodos/rotas, drain e ciclo de vida do worker |
| `go build -o bin/api.exe ./cmd/api` | PASS |
| `go build -o bin/worker.exe ./cmd/worker` | PASS |
| `docker compose --env-file .env.example config --quiet` | PASS |
| `docker compose up -d --wait` | PASS usando PostgreSQL na porta externa 55432; PostgreSQL e RabbitMQ saudáveis |
| Goose v3.28.0, `-dir db/migrations validate` | PASS; executável oficial com SHA-256 verificado |
| Goose `up` em PostgreSQL 18.6 | PASS; extensão btree_gist habilitada, versão 1 |
| Segundo Goose `up` e `status` | PASS; nenhuma migration pendente, versão preservada |
| `go test -tags=integration -count=1 -v ./tests/integration/...` | PASS; conexão PostgreSQL, timezone UTC, extensão e AMQP roundtrip com publisher confirm/ACK |
| Smoke dos binários compilados | PASS; `/health` = ok, `/ready` = ready, worker conectado e verificando dependências |
| Parser YAML de `docs/openapi.yaml` e conferência de rotas | PASS; não equivale à validação completa do schema OpenAPI por ferramenta especializada |
| `git diff --check`, inspeção de arquivos novos e whitespace | PASS; `.env`, `.tools` e `bin` ignorados; alterações locais conferidas |
| `go test -race ./...` | BLOQUEADO; Go informou que race requer cgo, indisponível nesta configuração. Nenhum resultado race é reivindicado |

A tentativa inicial de expor PostgreSQL em 5432 foi recusada pelo Windows; a configuração final usa 55432, sem alterar outros serviços. Erros iniciais de acesso ao Docker foram resolvidos executando a validação com acesso ao Docker Desktop. A verificação inicial de checksum do Goose parou por leitura incorreta da resposta binária pelo PowerShell; o checksum foi depois lido como arquivo de texto, comparado com o hash local e aprovado antes da execução.

O smoke HTTP usou temporariamente a porta 18080 para não disputar a porta padrão. Seus processos foram encerrados após as chamadas; graceful shutdown foi validado pelo teste que cancela o contexto com uma requisição em andamento e verifica seu término. Não foi testado o evento Ctrl+C do console Windows de ponta a ponta.

Os containers locais permanecem disponíveis. `docker compose stop` para interromper preservando os volumes. Os binários são artefatos locais ignorados pelo Git. Na etapa seguinte, a pedido do usuário, `.tools` foi removida integralmente, incluindo seus caches, Goose e logs; esses arquivos não são necessários para executar a aplicação. O registro acima preserva os resultados obtidos antes da remoção.

Não estão implementados, portanto não foram validados como funcionalidades: criação de agendamento, integridade concorrente das futuras tabelas, outbox, scheduler persistido, retries/DLQ, cancelamento de lembretes, envio externo, autenticação e isolamento multi-tenant. A arquitetura define essas exigências para os próximos incrementos; testes do scaffold não substituem seus testes de aceitação.

## Segunda etapa — limpeza e estrutura modular

A pedido do usuário, `.tools` foi inspecionada e removida depois de verificar seu caminho absoluto e a ausência de links e conteúdo inesperado. Nenhum arquivo de código, instalação Go, container ou volume foi removido. O Goose foi usado para validar as migrations antes da exclusão; não houve mudança no SQL nesta etapa.

Foram criados sete diretórios de negócio em `internal/modules`, cada um com `doc.go` e `application/doc.go` (14 arquivos). São contratos documentais de pacotes, sem handlers, persistência ou casos de uso executáveis. Não foram criados testes artificiais para declarações de pacotes; os testes existentes foram executados novamente.

Validações desta etapa, todas aprovadas: gofmt (incluindo checagem final sem diferenças), go mod tidy, go vet, go test ./..., compilação de API e worker, go doc de booking/application, Docker Compose config, integração PostgreSQL/RabbitMQ, git diff --check e conferência dos 14 arquivos. Confirmado ao final que `.tools` não existe.

Os comandos Go usaram a instalação em `C:\Program Files\Go` e os caches padrão `C:\Users\felip\AppData\Local\go-build` e `C:\Users\felip\go\pkg\mod`. As dependências foram baixadas novamente nesses locais. Não há instalação portátil nem dependência da pasta removida.

## Terceira etapa — camadas explícitas

A pedido do usuário, os sete doc.go de domínio foram movidos da raiz de seus módulos para domain/doc.go e sete infra/doc.go foram acrescentados. Application foi preservada. Agora há 21 pacotes de camada nos sete módulos. Os quatro arquivos executáveis em estudo (main da API, main do worker, config e server) não tiveram seu comportamento alterado.

Validações aprovadas: gofmt, go mod tidy, go vet, go test ./..., build de API/worker, go list dos 21 pacotes, Docker Compose config, integração real PostgreSQL/RabbitMQ e git diff --check. `.tools` continua ausente. Não houve nova migration, mudança de schema ou dependência; não foi necessário reaplicar migrations. Nenhum caso de uso/adaptador de negócio é declarado implementado por esses contratos de pacote.
