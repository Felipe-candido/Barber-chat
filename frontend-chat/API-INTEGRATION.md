# Integração do catálogo com a API Go

## Escopo e fonte dos contratos

Contratos conferidos antes da edição em:

- backend-chat/internal/modules/catalog/infra/http/routes.go: paths e métodos.
- backend-chat/internal/modules/catalog/infra/http/handler.go: request/response, limites, erros e acesso local.
- backend-chat/internal/modules/catalog/domain/service.go: validação.
- backend-chat/internal/modules/catalog/application/list_services.go e db/queries/catalog.sql: resolução do slug e filtro de ativos.
- backend-chat/internal/httpapi/server.go: health/readiness.

Não existem rotas de edição/exclusão, agendamento, disponibilidade, profissionais, perfil público de shop ou autenticação. Nenhuma delas foi inventada no cliente.

## Rotas de negócio integradas

### GET /api/v1/public/shops/{slug}/services

Slug obrigatório no path, codificado com encodeURIComponent. Não há paginação, filtros de categoria, autenticação ou parâmetros de query no contrato atual.

HTTP 200 retorna um ARRAY direto, não um envelope:

```json
[
  {
    "id": "11111111-1111-4111-8111-111111111111",
    "name": "Corte",
    "description": "Máquina e tesoura",
    "duration_minutes": 30,
    "price_cents": 4500,
    "currency": "BRL",
    "active": true
  }
]
```

Esses valores são apenas exemplo documental. O frontend não contém seed/fallback de serviços.

A API resolve a barbearia ativa pelo slug e retorna somente serviços ativos, ordenados por nome e ID. Uma barbearia ativa sem serviços retorna []. Barbearia ausente/inativa retorna 404. A listagem pública não substitui uma futura listagem administrativa com inativos.

Consumo:

- /admin/servicos: usa NEXT_PUBLIC_SHOP_SLUG.
- /b/{slug}: usa o slug da URL.
- /chat: atalho que usa NEXT_PUBLIC_SHOP_SLUG.

### POST /api/v1/admin/services

Content-Type: application/json. Corpo máximo de 64 KiB:

```json
{
  "name": "Corte",
  "description": "Máquina e tesoura",
  "duration_minutes": 30,
  "price_cents": 4500
}
```

Retorna HTTP 201 com UM objeto Service no mesmo formato da listagem. O servidor gera ID e define currency=BRL e active=true.

Regras reais:

- Nome após TrimSpace: 1 a 100 pontos de código Unicode.
- Duração inteira: 1 a 2147483647 minutos.
- Preço inteiro não negativo; campo obrigatório, inclusive quando o valor for zero.
- Descrição opcional no backend; frontend sempre envia uma string.
- Campos desconhecidos, múltiplos objetos JSON e corpo null são rejeitados.
- Não enviar shop_id, slug, category, active ou currency.
- Tenant é DEV_SHOP_SLUG, definido no servidor.

O frontend aceita vírgula ou ponto no preço e faz conversão decimal exata em centavos, sem multiplicação de float. Por utilizar números JavaScript/JSON, rejeita valores acima de Number.MAX_SAFE_INTEGER e respostas com preços fora desse limite. O backend usa int64, que possui limite maior.

A criação mostra estado de envio e bloqueia novo envio/fechamento durante a requisição. Só confirma sucesso depois da resposta da API e consulta novamente a lista. Se essa consulta falhar, o sucesso da criação continua informado, e o erro de atualização aparece separadamente.

Não há retry automático de POST. Falha de rede/timeout pode ocorrer depois de persistir: consultar a lista antes de repetir.

## Erros

Envelope: {"error":{"code":"...","message":"..."}}. O cliente preserva código/status e apresenta mensagens locais, sem imprimir conteúdo bruto do servidor.

| Status | Code                     | Significado                                        |
| ------ | ------------------------ | -------------------------------------------------- |
| 400    | invalid_body             | JSON inválido, desconhecido ou múltiplo            |
| 403    | admin_access_unavailable | Escrita local desabilitada ou acesso não permitido |
| 404    | shop_not_found           | Barbearia ausente/inativa                          |
| 413    | body_too_large           | Corpo acima de 64 KiB                              |
| 415    | unsupported_media_type   | Content-Type incorreto                             |
| 422    | invalid_service          | Campos inválidos ou preço ausente                  |
| 500    | internal_error           | Falha interna sanitizada                           |
| 503    | temporarily_unavailable  | Timeout/cancelamento do backend                    |

GET pode retornar 404/500/503. Falhas de rede, timeout do cliente (10 segundos), JSON inesperado e configuração inválida têm mensagens específicas. As leituras são canceladas ao sair da tela ou mudar o slug, evitando resposta antiga na unidade errada.

## Estrutura do frontend

- src/lib/api/config.ts: URL base e slug local.
- src/lib/api/client.ts: fetch central, timeout, cancelamento, parsing e ApiError.
- src/lib/api/services.ts: contratos tipados, validação de resposta, conversão monetária e operações reais.
- src/hooks/use-services.ts: ciclo de carregamento/erro/atualização compartilhado pelas telas.
- src/components/services-page.tsx: listagem, busca local e criação.
- src/components/booking-chat.tsx: catálogo público por slug e seleção de serviço.
- src/components/feedback-provider.tsx: notificações de interface, sem dados de negócio.

Nenhum componente chama fetch diretamente. Cache desativado para evitar catálogos obsoletos. Nenhum dado de negócio é gravado em localStorage; a antiga chave de demonstração é ignorada e não é apagada automaticamente.

## CORS e acesso local: único ajuste funcional no backend

O handler original proibia qualquer Origin; portanto um browser na porta 3000 não podia criar serviços na API da porta 8080. Não foi criado proxy para remover os headers e contornar essa proteção.

DEV_FRONTEND_ORIGIN permite UMA origem HTTP de loopback exata, sem barra final. Exige listener com IP de loopback. CORS foi adicionado somente às duas rotas do catálogo, com OPTIONS, Content-Type e GET/POST conforme o path. Sem wildcard e sem cookies/credenciais. POST continua exigindo DEV_SHOP_SLUG, peer local e Host local. Origens não autorizadas continuam rejeitadas.

Exemplo: http://127.0.0.1:3000 é diferente de http://localhost:3000 e da porta 3001. Abra exatamente a origem configurada. Variável vazia conserva o bloqueio de escrita pelo browser.

Isso é uma facilidade de desenvolvimento, não autenticação. Não publicar a API local por proxy/túnel. A integração de produção depende da autenticação/membership descrita na proposta do backend. NEXT_PUBLIC_SHOP_SLUG deve corresponder a DEV_SHOP_SLUG para que a tela administrativa leia a mesma barbearia onde o servidor cria. O POST atual não retorna contexto de tenant que permita essa verificação pelo navegador.

## Funcionalidades pendentes e contratos necessários

Os itens abaixo são requisitos para definição futura. URI e método ainda devem ser acordados/implementados no backend; não há chamadas ou URLs fictícias no código.

| Capacidade pendente         | Dados/contrato que o backend deve fornecer                                                     | Comportamento atual                                            |
| --------------------------- | ---------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| Listagem administrativa     | Serviços ativos e inativos, escopo autenticado, filtros/paginação definidos                    | Lista pública de ativos identificada como tal                  |
| Editar serviço              | ID, campos alteráveis, validação, objeto atualizado, erros de acesso/ausência/conflito         | Botão desabilitado                                             |
| Excluir/desativar           | Política para históricos/agendamentos, resposta definida, referências/conflitos                | Botão desabilitado; sem toggle falso                           |
| Categorias                  | Modelo, identificador e vínculo opcional no serviço                                            | Campo/filtro removidos                                         |
| Dados públicos da barbearia | Nome, slug, logo, timezone e estado, sem dados administrativos                                 | Slug real exibido; Palma identificada como marca de referência |
| Profissionais               | IDs, nomes, serviços habilitados e atividade, limitados ao tenant                              | Nenhuma equipe fictícia                                        |
| Disponibilidade             | Profissional/serviço/data ou intervalo; horários em UTC e timezone; duração                    | Seleção de horário indisponível                                |
| Criar agendamento           | Serviço, profissional, início, contato/consentimento; reserva confirmada e conflito de horário | Fluxo para após selecionar serviço; não coleta contato         |
| Consultar agenda            | Intervalo, profissional/status, timezone, paginação; agendamentos e snapshots                  | Calendário sem dados, sem métricas fictícias                   |
| Alterar/cancelar/concluir   | ID, transições permitidas, concorrência/versão, estado atualizado                              | Ações não disponíveis                                          |
| Login/sessão/membership     | Identidade verificada, unidades e permissões; sem confiar em shop_id do cliente                | /login continua prévia explícita sem autenticação              |

Health/readiness já existem: GET /health -> 200 {"status":"ok"}; GET /ready -> 200 {"status":"ready"} ou 503 {"status":"unavailable"}. São probes operacionais, não fontes de dados das telas; não receberam CORS nem consumo no catálogo.

## Testar localmente

Backend, em novo PowerShell:

```powershell
cd C:\Users\felip\Barber-chat\backend-chat
$env:HTTP_ADDR = '127.0.0.1:8080'
$env:DEV_SHOP_SLUG = 'barbearia-do-felipe'
$env:DEV_FRONTEND_ORIGIN = 'http://127.0.0.1:3000'
go run ./cmd/api
```

A API lê DATABASE_URL do seu .env. Não copiar a connection string PostgreSQL para o frontend. O banco precisa ter as migrations existentes aplicadas e uma barbearia ativa com esse slug. Não se cria barbearia/migration automaticamente ao abrir o frontend. Consulte backend-chat/docs/catalog-testing.md. Para dados de exemplo, o seed existente é uma operação separada; não executar contra uma base real sem revisar.

Frontend, em outro terminal:

```powershell
cd C:\Users\felip\Barber-chat\frontend-chat
if (!(Test-Path .env.local)) { Copy-Item .env.example .env.local }
npm.cmd install
npm.cmd run dev
```

Em .env.local:

```dotenv
NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8080
NEXT_PUBLIC_SHOP_SLUG=barbearia-do-felipe
```

Variáveis NEXT_PUBLIC_* são públicas e incorporadas ao build. Não colocar segredos. Reiniciar o servidor de desenvolvimento após mudar; em produção, gerar novo build.

Abrir http://127.0.0.1:3000/admin/servicos. Criar um serviço, confirmar o sucesso e recarregar: ele deve permanecer porque vem do banco. Conferir no catálogo público http://127.0.0.1:3000/b/barbearia-do-felipe. Se o POST der 403, conferir as variáveis locais e reiniciar a API. Se GET der 404, conferir o slug e a atividade da barbearia. Falha de rede pode significar API desligada, URL errada ou CORS.

## Verificações

```powershell
# Em frontend-chat
npm.cmd run typecheck
npm.cmd run format:check
npm.cmd run build
npm.cmd test

# Em backend-chat
go test ./...
go vet ./...
go build ./cmd/api ./cmd/worker
```

Playwright inicia um Next de teste isolado na porta 3100 com configuração de catálogo fixa. As respostas HTTP são interceptadas APENAS nos testes, sem acessar o seu banco ou cadastrar serviços reais. Cobrem os formatos exatos, erros, lista vazia, ausência de fallback local, bloqueio de envio duplicado, refresh, chat por slug e layouts móveis. Os testes Go exercitam os handlers reais e as regras CORS.

Testes com PostgreSQL real são separados: go test -tags=integration ./tests/integration/... exige banco preparado e variáveis de ambiente. Testes de browser com fixtures e unitários Go não comprovam persistência no Supabase.

### Pendência encontrada no banco configurado

Na validação de 23/09/2026, os testes `TestCatalog` acessaram o PostgreSQL, mas falharam porque `services.currency` não existe (`SQLSTATE 42703`). A migration existente `backend-chat/db/migrations/00003_service_currency.sql` adiciona essa coluna. Nenhuma migration foi alterada ou aplicada automaticamente. `/ready` pode retornar sucesso mesmo com essa pendência: ele verifica conectividade, não o schema.

Antes do teste manual, confira o banco de destino e o status das migrations seguindo `backend-chat/docs/catalog-testing.md`. Aplique as migrations pendentes no seu banco de desenvolvimento e rode novamente `go test -tags=integration -run TestCatalog -count=1 ./tests/integration/...`. Se o Goose marcar a terceira migration como aplicada e a coluna continuar ausente, investigue o schema/search_path e o histórico do banco antes de reaplicar SQL.
