# Testar criação e listagem de serviços

Endpoints implementados:

- GET /api/v1/admin/me: identidade local ativa; exige Bearer Supabase.
- GET /api/v1/admin/shops: unidades elegíveis vinculadas ao usuário; exige Bearer.
- GET /api/v1/admin/shops/{slug}/services: serviços ativos; exige o vínculo da unidade.
- POST /api/v1/admin/shops/{slug}/services: criação; exige usuário/unidade/membership ativos.
- GET /api/v1/public/shops/{slug}/services: lista pública de serviços ativos.

Editar e ativar/desativar serviços ainda não estão implementados.

## Preparar o banco

Use um banco de desenvolvimento. No Supabase, use conexão direta ou Session pooler na porta 5432 e SSL. O modo local de HTTP não impede escrever no Supabase se DATABASE_URL aponta para ele. Configure a exposição da Data API separadamente; estas migrations não criam políticas de acesso da Data API.

Goose continua sendo ferramenta externa; o carregamento automático da API não configura o terminal do Goose.

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned
. .\scripts\Load-Env.ps1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
$env:GOOSE_DRIVER = 'postgres'
$env:GOOSE_DBSTRING = $env:DATABASE_URL
& $goose -dir db/migrations validate
& $goose -dir db/migrations up
& $goose -dir db/migrations status
```

São quatro migrations comuns: btree_gist, shops/services, moeda BRL e users/memberships. No Supabase, aplicar também a sequência específica de Auth e provisionar o usuário/vínculo, seguindo [autenticação](authentication-proposal.md). Migrations antigas não foram alteradas.

Depois execute o conteúdo de db/seeds/development.sql no SQL Editor do Supabase, ou via psql no banco local. Ele cria uma barbearia de exemplo com slug barbearia-do-felipe. Se esse slug já existir, preserva o registro. Se estiver inativo, escolha uma barbearia ativa; o seed não reativa registros.

No PostgreSQL do Compose, o comando equivalente é:

```powershell
Get-Content -Raw db/seeds/development.sql | docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U barber -d barber
```

O comando acima é para as credenciais locais de .env.example, não para o Supabase. O schema é gerenciado pelas migrations; os antigos CREATE TABLE duplicados foram retirados de db/queries.

## Iniciar a API

Na pasta `backend-chat`, em um terminal novo:

```powershell
# Configure SUPABASE_URL no .env com o projeto que emitiu o access token.
go run ./cmd/api
```

O browser precisa de FRONTEND_ORIGIN exata, sem barra final, e SUPABASE_URL deve corresponder ao projeto do login. Não há slug fixo na configuração. Reinicie a API após mudanças.

Nenhum shop_id é recebido do cliente. O slug da URL solicita a unidade; o middleware consulta shops e exige membership do usuário autenticado antes de colocar ShopID no contexto. O handler passa esse UUID autorizado ao caso de uso. Slug, Origin ou IP local não concedem acesso.

## Fazer o primeiro POST

Em outro terminal:

```powershell
$tokenInput = Read-Host 'Access token da sessão Supabase (não use refresh token ou chave de API)' -AsSecureString
$accessToken = [System.Net.NetworkCredential]::new('', $tokenInput).Password
$headers = @{ Authorization = "Bearer $accessToken" }
Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/admin/me' -Headers $headers
$shops = @(Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/admin/shops' -Headers $headers)
$shops | Format-Table shop_id, name, slug
$chosenSlug = Read-Host 'Slug de uma unidade da lista'
if (!($shops | Where-Object slug -eq $chosenSlug)) { throw 'Escolha uma unidade vinculada.' }
$shopPath = [Uri]::EscapeDataString($chosenSlug)
$adminServicesURL = 'http://127.0.0.1:8080/api/v1/admin/shops/' + $shopPath + '/services'

$body = @{
    name = 'Corte masculino'
    description = 'Corte com máquina e tesoura'
    duration_minutes = 30
    price_cents = 3500
} | ConvertTo-Json

$service = Invoke-RestMethod -Method Post -Uri $adminServicesURL -Headers $headers -ContentType 'application/json; charset=utf-8' -Body ([Text.Encoding]::UTF8.GetBytes($body))
$service

Invoke-RestMethod -Uri $adminServicesURL -Headers $headers
Invoke-RestMethod -Uri ('http://127.0.0.1:8080/api/v1/public/shops/' + $shopPath + '/services')
Remove-Variable accessToken, tokenInput, headers
```

POST retorna 201 com id, nome normalizado, descrição, duração, price_cents=3500, currency=BRL e active=true. A listagem retorna um array. Repetir o POST cria outro serviço: deduplicação por nome ou chave de idempotência não faz parte deste incremento.

## Diagnóstico

| Resposta | Conferir |
| --- | --- |
| 401 | Token ausente, malformado, expirado ou de outro projeto |
| 403 | Usuário não provisionado/inativo; unidade/vínculo inelegível; Origin não permitida |
| 404 | Slug público inexistente/inativo, ou unidade desativada entre autorização e INSERT |
| 400 | JSON inválido, campo desconhecido, shop_id no corpo ou objetos extras |
| 413 / 415 | Corpo acima de 64 KiB / Content-Type diferente de application/json |
| 422 | Nome vazio/acima de 100 caracteres, duração inválida, preço ausente/negativo |
| 500 | Migrations pendentes ou outro erro interno; a resposta não expõe SQL |
| 503 | Falha JWKS, deadline ou cancelamento da operação |

/ready testa conectividade, não a presença das tabelas/coluna currency.

## Testes automatizados

Os testes dos módulos ficam em `internal/modules/<modulo>/tests`, organizados
por camada: `application`, `domain` e `infra/http` (quando há testes dessa camada).
Cada suíte importa a API pública do pacote que testa. Configuração, servidor e
worker seguem o mesmo padrão em `internal/config/tests`, `internal/httpapi/tests`
e `internal/worker/tests`. Os testes de integração ficam em `tests/integration`.

Execute os comandos abaixo na pasta `backend-chat`:

```powershell
go test ./...
go vet ./...
# Apenas os casos de uso:
go test ./internal/modules/catalog/tests/application ./internal/modules/identity/tests/application
# Todas as camadas dos módulos:
go test ./internal/modules/catalog/tests/... ./internal/modules/identity/tests/...
# Cobertura: incluir os pacotes de produção e de testes no filtro.
# As aspas preservam o argumento completo no PowerShell.
go test '-coverpkg=./internal/modules/catalog/...' ./internal/modules/catalog/tests/...
go test '-coverpkg=./internal/modules/identity/...' ./internal/modules/identity/tests/...
# Banco com migrations aplicadas; DATABASE_URL disponível na sessão:
go test -tags=integration -run TestCatalog -count=1 ./tests/integration/...
# A suíte completa também requer RABBITMQ_URL e RabbitMQ:
go test -tags=integration -count=1 ./tests/integration/...
```

Os testes de catálogo criam fixtures em transação e fazem rollback ao final. Exercitam POST, leitura persistida, listagem por tenant, barbearia/serviço inativos, validação sem INSERT, FK, checks de preço/duração/moeda e rejeição de criação após desativação. Não aplicam migrations automaticamente. Execute em banco de desenvolvimento; rollback de fixtures não substitui essa escolha.

Com a fixture de identity, TestCatalog exige PostgreSQL de desenvolvimento puro,
com as quatro migrations comuns. Não use o projeto Supabase com a FK de auth.users
para esse teste: os UUIDs temporários não são contas reais do provedor. O login
real é validado separadamente com uma sessão provisionada e as rotas da API.

## Onde ler a arquitetura

- internal/modules/catalog/infra/doc.go: fluxo ponta a ponta, composição, limites e tenant.
- internal/modules/catalog/domain/doc.go: entidade, invariantes e erros.
- internal/modules/catalog/application/doc.go: casos de uso, portas e DTO.
- internal/modules/catalog/infra/http/doc.go: validação, escopo autorizado e respostas.
- internal/modules/catalog/infra/postgres/doc.go: SQL, mapeamento e pool/transação.
- internal/modules/shops/infra/postgres/doc.go: resolução de slug sem assumir autorização.

Exemplo: go doc ./internal/modules/catalog/application. Os comentários de código ficam em inglês, conforme AGENTS.md; este guia explica o uso em português.
