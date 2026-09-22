# Testar criação e listagem de serviços

Os dois endpoints implementados são:

- POST /api/v1/admin/services: criação, disponível somente com modo local explícito.
- GET /api/v1/public/shops/{slug}/services: lista pública de serviços ativos.

Editar, ativar/desativar e acesso administrativo com usuário/membership ainda não estão implementados.

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

São três migrations: extensão btree_gist, tabelas shops/services, e currency BRL. Migrations antigas não foram alteradas. O status deve mostrar todas aplicadas.

Depois execute o conteúdo de db/seeds/development.sql no SQL Editor do Supabase, ou via psql no banco local. Ele cria uma barbearia de exemplo com slug barbearia-do-felipe. Se esse slug já existir, preserva o registro. Se estiver inativo, escolha uma barbearia ativa; o seed não reativa registros.

No PostgreSQL do Compose, o comando equivalente é:

```powershell
Get-Content -Raw db/seeds/development.sql | docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U barber -d barber
```

O comando acima é para as credenciais locais de .env.example, não para o Supabase. O schema é gerenciado pelas migrations; os antigos CREATE TABLE duplicados foram retirados de db/queries.

## Iniciar a API

Na raiz do projeto, em um terminal novo:

```powershell
$env:DEV_SHOP_SLUG = 'barbearia-do-felipe'
go run ./cmd/api
```

Outra opção é adicionar DEV_SHOP_SLUG=barbearia-do-felipe ao seu .env. HTTP_ADDR precisa ser um IP loopback, por exemplo 127.0.0.1:8080. Se houver uma API antiga nessa porta, encerre-a e inicie a nova versão.

Nenhum shop_id é recebido do cliente. A API usa o slug configurado no servidor, consulta shops e passa o UUID resolvido para o domínio. O slug identifica a barbearia, mas não autentica pessoas. Esse modo não deve ser exposto por túnel/proxy; produção precisa de autorização por membership.

## Fazer o primeiro POST

Em outro terminal:

```powershell
$body = @{
    name = 'Corte masculino'
    description = 'Corte com máquina e tesoura'
    duration_minutes = 30
    price_cents = 3500
} | ConvertTo-Json

$service = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:8080/api/v1/admin/services' -ContentType 'application/json; charset=utf-8' -Body ([Text.Encoding]::UTF8.GetBytes($body))
$service

Invoke-RestMethod -Uri 'http://127.0.0.1:8080/api/v1/public/shops/barbearia-do-felipe/services'
```

POST retorna 201 com id, nome normalizado, descrição, duração, price_cents=3500, currency=BRL e active=true. A listagem retorna um array. Repetir o POST cria outro serviço: deduplicação por nome ou chave de idempotência não faz parte deste incremento.

## Diagnóstico

| Resposta | Conferir |
| --- | --- |
| 403 | DEV_SHOP_SLUG não configurado ou requisição fora da fronteira local |
| 404 | Slug inexistente ou barbearia inativa |
| 400 | JSON inválido, campo desconhecido, shop_id no corpo ou objetos extras |
| 413 / 415 | Corpo acima de 64 KiB / Content-Type diferente de application/json |
| 422 | Nome vazio/acima de 100 caracteres, duração inválida, preço ausente/negativo |
| 500 | Migrations pendentes ou outro erro interno; a resposta não expõe SQL |
| 503 | Deadline ou cancelamento da operação |

/ready testa conectividade, não a presença das tabelas/coluna currency.

## Testes automatizados

```powershell
go test ./...
go vet ./...
# Banco com migrations aplicadas; DATABASE_URL disponível na sessão:
go test -tags=integration -run TestCatalog -count=1 ./tests/integration/...
# A suíte completa também requer RABBITMQ_URL e RabbitMQ:
go test -tags=integration -count=1 ./tests/integration/...
```

Os testes de catálogo criam fixtures em transação e fazem rollback ao final. Exercitam POST, leitura persistida, listagem por tenant, barbearia/serviço inativos, validação sem INSERT, FK, checks de preço/duração/moeda e rejeição de criação após desativação. Não aplicam migrations automaticamente. Execute em banco de desenvolvimento; rollback de fixtures não substitui essa escolha.

## Onde ler a arquitetura

- internal/modules/catalog/infra/doc.go: fluxo ponta a ponta, composição, limites e tenant.
- internal/modules/catalog/domain/doc.go: entidade, invariantes e erros.
- internal/modules/catalog/application/doc.go: casos de uso, portas e DTO.
- internal/modules/catalog/infra/http/doc.go: rotas, validação, modo local e respostas.
- internal/modules/catalog/infra/postgres/doc.go: SQL, mapeamento e pool/transação.
- internal/modules/shops/infra/postgres/doc.go: resolução de slug sem assumir autorização.

Exemplo: go doc ./internal/modules/catalog/application. Os comentários de código ficam em inglês, conforme AGENTS.md; este guia explica o uso em português.
