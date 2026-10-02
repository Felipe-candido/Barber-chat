# Autenticação: Supabase Auth e acesso por barbearia

Backend/frontend atualizados em 01/10/2026. **Implementados: domínio de identity, Authenticate/AuthorizeShopAction/ListMyShops, queries/repositório, verifier ES256/JWKS, middleware, `/admin/me`, `/admin/shops`, seleção de unidade e leitura/criação administrativa de serviços.** Pendentes: CRUD de contas e outros fluxos administrativos. As migrations não são aplicadas no boot; contas e vínculos continuam manuais.

## Decisão para o primeiro incremento

A proposta de contas criadas manualmente é adequada para as primeiras barbearias. Cada pessoa deve ter uma conta própria, mesmo sem diferenciação de papéis. O Supabase cuida de credenciais e sessão; nossa aplicação guarda o perfil e as unidades que a pessoa pode administrar. Não há conta/senha compartilhada por empresa.

Esta versão substitui a proposta anterior de owner/staff e de um ID local separado com issuer/subject. Teremos um único projeto Supabase confiável, o mesmo UUID no Auth e em users, e vínculos sem role. Todos os membros ativos terão as mesmas permissões de negócio dentro da unidade. Criar contas/vínculos permanece uma operação do operador, fora do painel.

```mermaid
erDiagram
    AUTH_USERS ||--o| USERS : "mesmo UUID"
    USERS ||--o{ SHOP_MEMBERSHIPS : possui
    SHOPS ||--o{ SHOP_MEMBERSHIPS : possui
    USERS {
        uuid id PK,FK
        string display_name
        boolean active
        timestamptz created_at
    }
    SHOP_MEMBERSHIPS {
        uuid user_id PK,FK
        uuid shop_id PK,FK
        boolean active
        timestamptz created_at
    }
```

- `auth.users`: identidade gerenciada pelo provedor; login, senha, confirmação e recuperação pertencem ao Supabase.
- `public.users`: perfil administrativo. `id` copia o UUID do Auth, sem default que gere outro ID. Não duplicamos senha, hash ou e-mail.
- `public.shop_memberships`: ligação com `public.shops`, única por (user_id, shop_id). Sem papel. Um usuário pode ter várias unidades; uma unidade pode ter vários usuários.
- Profissionais e clientes continuam conceitos separados. Clientes do chat público não precisam de conta.

Membership evita duplicar uma pessoa ou mover seu shop_id quando ela administrar uma segunda unidade. Limitar uma unidade por pessoa poderá ser uma regra futura explícita; não é uma restrição deste modelo.

## Arquivos e ordem de aplicação

1. `db/migrations/00004_create_identity_tables.sql`: cria users/memberships, constraints, índice por shop_id e RLS sem policies de browser. Segue as migrations 00001–00003.
2. `db/supabase/migrations/00001_link_auth_users.sql`: adiciona a FK `public.users.id -> auth.users.id ON DELETE CASCADE` e revoga privilégios diretos de anon/authenticated nas duas tabelas.
3. `db/supabase/provision_shop_and_user.sql`: procedimento manual para criar barbearia, perfil e vínculo a partir de um UUID existente no Auth. Não é migration nem seed automático.
4. `db/supabase/provision_membership.sql`: procedimento manual para vincular uma conta a uma barbearia já existente. Insere perfil/vínculo sem duplicação e recusa reativar acessos suspensos.

O PostgreSQL do Compose não possui auth.users. Por isso a ligação específica do provedor tem uma sequência separada e uma tabela de versões própria. **No Supabase, as duas sequências são obrigatórias.** Se auth.users não existir, a segunda falha em vez de deixar a FK ausente silenciosamente. Não criar auth.users falsa no banco da aplicação.

Em PowerShell, no diretório backend-chat, confira antes se DATABASE_URL aponta para o projeto pretendido. Para migrations no Supabase use conexão direta ou Session pooler, conforme o guia de banco existente. Goose já está instalado neste ambiente:

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned
. .\scripts\Load-Env.ps1
$env:GOOSE_DRIVER = 'postgres'
$env:GOOSE_DBSTRING = $env:DATABASE_URL

goose -dir db/migrations validate
goose -dir db/migrations status
goose -dir db/migrations up
if ($LASTEXITCODE -ne 0) { throw 'Falha nas migrations comuns' }

# Histórico SEPARADO: não omitir -table nem misturar as pastas.
goose -dir db/supabase/migrations -table public.goose_supabase_version validate
goose -dir db/supabase/migrations -table public.goose_supabase_version up
if ($LASTEXITCODE -ne 0) { throw 'Falha na ligação com Supabase Auth' }

goose -dir db/migrations status
goose -dir db/supabase/migrations -table public.goose_supabase_version status
```

Rollback, somente em banco descartável ou após revisão: desfazer primeiro a sequência Supabase e depois a comum. Não remover a FK de um ambiente ativo para resolver erro de provisionamento. Um Down da sequência comum remove as tabelas e seus registros; não executá-lo para corrigir cadastro.

## Criar uma conta e ligá-la à barbearia

1. No Dashboard Supabase, configure login por e-mail/senha e desabilite cadastro público na configuração do Auth. Esconder o botão de signup não desabilita o provedor.
   O verifier atual aceita somente JWTs ES256. Confira a signing key do projeto e use um access token emitido por essa chave; não há fallback para HS256 nem uso de JWT secret no Go. Configure SUPABASE_URL com a origem HTTPS desse mesmo projeto.
2. Em Authentication → Users, crie a conta pela interface do provedor e copie seu UUID. Não faça INSERT em auth.users pelo SQL Editor nem invente hashes. Convites/recuperação exigem URLs permitidas e telas de retorno; essas telas ainda precisam ser integradas. Para uso real com envio de e-mail, configurar SMTP é outra etapa.
3. Confira a barbearia no SQL Editor:

   ```sql
   SELECT id, name, slug, active FROM public.shops ORDER BY name;
   ```

   Ela deve existir e estar ativa. O seed de desenvolvimento existente cria barbearia-do-felipe; não executar esse seed para cadastrar outras empresas reais.
4. Abra `db/supabase/provision_membership.sql`. Substitua `target_user_id`, `target_shop_slug` e `target_display_name` e execute no SQL Editor. Use o UUID do passo 2 e o slug do passo 3. Se a transação abortar, execute ROLLBACK antes de tentar de novo na mesma sessão SQL.
5. Confira a associação:

   ```sql
   SELECT u.id, u.display_name, u.active AS user_active,
          s.slug, s.active AS shop_active, m.active AS membership_active
   FROM public.users u
   JOIN public.shop_memberships m ON m.user_id = u.id
   JOIN public.shops s ON s.id = m.shop_id
   ORDER BY s.slug, u.display_name;
   ```

Não há trigger de criação automática. Criar a conta no Auth, sozinho, não cria acesso ao negócio. Se o SQL falhar, corrija e repita: o script reutiliza usuário/vínculo ativos sem sobrescrever perfil nem duplicar associação. Não reativa acessos suspensos. Cada unidade adicional usa o mesmo UUID com outro slug.

Suspender users.active afeta todas as unidades; suspender shop_memberships.active afeta somente o vínculo. Excluir no Auth elimina perfil/vínculos por cascata, sem excluir barbearia/serviços. As rotas administrativas verificam essas flags no banco por requisição, sem cache de permissões. /me identifica o usuário ativo, mesmo sem vínculo; criar serviços exige o vínculo da unidade solicitada.

### Criar a barbearia e o perfil em uma execução

Depois de aplicar as duas sequências de migrations, abra
`db/supabase/provision_shop_and_user.sql` e copie seu conteúdo inteiro para o SQL
Editor **do mesmo projeto usado por SUPABASE_URL e DATABASE_URL no backend**.
Execute como operador administrativo, nunca pelo frontend ou por uma policy
que permita ao usuário criar o próprio vínculo.

Altere somente os cinco valores no início de DECLARE:

```sql
target_user_id UUID := '00000000-0000-0000-0000-000000000000'; -- Troque pelo UUID real.
target_display_name TEXT := 'Felipe';
target_shop_name TEXT := 'Barbearia do Felipe';
target_shop_slug TEXT := 'barbearia-do-felipe';
target_shop_timezone TEXT := 'America/Sao_Paulo';
```

O UUID precisa existir em Authentication → Users nesse projeto. O script não cria
conta, senha nem e-mail no Auth: cria somente public.users, public.shops e
public.shop_memberships. O ID da barbearia é gerado separadamente. Não execute
esse script no PostgreSQL puro do Compose, que não possui auth.users.

O slug aceita letras minúsculas sem acentos, números e hífens simples entre
palavras, até 100 caracteres. O nome da barbearia aceita até 150 caracteres e o
nome do usuário até 100. O fuso deve ser reconhecido pelo PostgreSQL. Em textos
SQL com apóstrofo, duplique-o: `'Barbearia O''Neil'`.

A transação cria todos os registros ou nenhum deles. Repetir os mesmos dados
ativos reutiliza a barbearia e o vínculo sem duplicar. Um perfil ativo existente
é reutilizado sem alterar seu nome; um slug existente só é aceito se nome e fuso
forem iguais aos informados. Usuário, barbearia ou vínculo suspenso geram erro,
sem reativação automática. Para ligar alguém a uma unidade já existente sem
repetir seus dados, use provision_membership.sql.

Ao concluir, a consulta do script retorna o perfil, a barbearia e as três flags
active. Faça login no frontend e a unidade aparecerá na seleção. Se já estiver
logado, use a opção de atualizar a lista de barbearias. Se ocorrer erro, execute
`ROLLBACK;` antes de corrigir e repetir na mesma sessão SQL.

## Fluxo implementado

1. Next.js autentica no Supabase, que entrega sessão e access token. Login e renovação pertencem ao frontend/provedor, não ao catálogo Go.
2. O frontend envia `Authorization: Bearer <access_token>` ao Go. Nunca envia senha ao catálogo.
3. Uma única instância de Verifier no bootstrap valida ES256, issuer de SUPABASE_URL/auth/v1, audience authenticated, expiração, assinatura e subject UUID não vazio. JWKS vem de uma URL fixa do projeto, com cache, controle de atualização e timeout. JWT usa golang-jwt; JWK usa go-jose. Tokens anônimos, service_role e chaves de API não são sessões administrativas válidas.
4. Authenticate usa o sub verificado para consultar users.id e exige usuário local ativo. O middleware armazena StaffIdentityOutput no contexto; GET /api/v1/admin/me retorna somente user_id/display_name. O UUID é o do Auth, não outro ID gerado.
5. Depois de /me, GET /api/v1/admin/shops chama ListMyShops com o usuário do contexto: relê o perfil e lista somente vínculos/unidades ativos, retornando shop_id/name/slug (ou []). O frontend mostra essas unidades em /admin, sem escolher automaticamente, e abre /admin/{slug}/servicos após a escolha.
6. GET e POST /api/v1/admin/shops/{slug}/services usam ShopAccess: AuthorizeShopAction relê o usuário, resolve a unidade ativa e verifica exatamente (user_id, shop_id), com membership ativo. A URL solicita uma unidade; não concede acesso. Outra unidade só é aceita se houver seu próprio vínculo. Não se escolhe o primeiro vínculo, nem se autoriza por metadata.
7. O middleware armazena AuthorizedShopScope no contexto derivado. O handler exige identidade/escopo consistentes, valida JSON no POST e passa ShopID explicitamente ao CreateService ou ListAuthorizedServices. Application/domain não leem valores HTTP do contexto. O INSERT ainda revalida que a barbearia está ativa; não há operação de agendamento nesta etapa.

Token ausente/inválido retorna 401 com WWW-Authenticate; usuário não provisionado/inativo ou unidade/vínculo inelegível retorna 403 genérico. Falha JWKS/deadline retorna 503; inconsistência de identidade/erro inesperado de repositório retorna 500 sanitizado. Não se registram tokens, credenciais ou erros brutos do provedor. RequestID acompanha logs operacionais. HTTP_TIMEOUT cobre toda a cadeia, inclusive auth; DB_TIMEOUT limita a operação do catálogo.

FRONTEND_ORIGIN permite uma origem exata HTTPS (ou HTTP loopback em desenvolvimento). DEV_FRONTEND_ORIGIN é alias local compatível. CORS permite Authorization/Content-Type, e OPTIONS ocorre antes de autenticar: preflight nunca substitui a proteção da requisição real. A rota antiga POST /api/v1/admin/services e os slugs fixos de ambiente foram removidos. As rotas por unidade usam sempre o slug da escolha, mantendo Bearer/membership obrigatório. O catálogo público continua sem login.

Logout/revogação não tornam todo JWT já emitido imediatamente inválido. O verifier valida o JWT localmente e não consulta o estado da sessão no Supabase a cada requisição. Consultar o estado local permite negar as próximas verificações de acesso após suspender usuário/vínculo; requisições já autorizadas podem estar em andamento. Renovação e recuperação pertencem à sessão no frontend. Não use flags de login como prova de identidade.

Responsabilidades: identity/infra valida token e consulta PostgreSQL; identity/application resolve identidade/autoriza unidade; domain guarda invariantes; cmd/api compõe dependências; internal/httpapi adapta headers, erros e contexto; frontend administra a sessão. requestctx é compartilhado entre middleware e handlers, não pertence aos casos de uso. Um único pool/queries atende os repositórios. RabbitMQ não participa do login.

## Sessão e contexto no frontend

O AdminAccessProvider mantém em memória somente a identidade local e os resumos
de unidades obtidos no Go. O SDK Supabase persiste/renova a sessão; o token não é
duplicado no contexto React. Um listener síncrono de mudanças de conta/logout
cancela leituras pendentes e limpa dados antigos. Não chamar APIs de autenticação
dentro desse listener: chamadas aninhadas podem disputar o lock do SDK.

AdminAccessGuard aguarda a validação antes de montar páginas administrativas.
ShopPanel verifica o slug contra a lista para a navegação e fornece a unidade
selecionada aos componentes. Trocar de slug remonta os componentes e cancela
consultas antigas. Essas proteções são experiência/isolamento visual, não a
autoridade de segurança: o Go verifica o vínculo em cada GET/POST administrativo.
O logout usa scope local e limpa a sessão deste navegador; não encerra todos os
dispositivos nem torna imediatamente inválido um JWT copiado anteriormente.
A sessão fica no armazenamento do SDK, portanto prevenir XSS é indispensável;
SSR com cookies não foi implementado neste incremento.

## Contrato do repositório de identity

`identity/infra/postgres.Repository` recebe `*db.Queries` ligado ao pool compartilhado
ou a uma transação do chamador. Não abre conexões nem decide acesso à barbearia.
`FindUserByID` e `FindMembership` retornam `(valor, found, erro)`: ausência significa
`found=false, erro=nil`; falha de consulta continua sendo erro. Registros inativos
retornam `found=true` e `Active=false` para a aplicação decidir o acesso.
`ListMembershipsByUser` retorna uma lista vazia não-nil quando não existem vínculos.
O mapeamento preserva IDs e flags; não utiliza construtores que ativariam registros.

Os testes em `internal/modules/identity/tests/infra/postgres` exercitam o adapter e
as queries geradas com um substituto da conexão. Cobrem ausência, falhas,
cancelamento, parâmetros, leitura de inativos, lista vazia e fechamento de resultados.
Não comprovam execução SQL real. Para executar:

```powershell
go test ./internal/modules/identity/tests/infra/postgres
```

O subteste `TestIdentityMigrations/repository` usa PostgreSQL real, as migrations e
as fixtures de provisionamento para verificar mapeamento, vínculos de dois usuários,
isolamento, ordenação e estados inativos. Roda junto com o teste de migrations abaixo;
exige a mesma base descartável e reverte suas alterações por savepoint/transação.

## RLS e conexão PostgreSQL

As tabelas novas ficam com RLS e sem policies para browser; a etapa Supabase também remove grants de anon/authenticated. Não permitir ao usuário inserir seu próprio membership. O frontend consumirá dados de negócio pelo Go, sem CRUD direto via Data API.

O JWT recebido pelo Go **não configura auth.uid() na conexão pgx**. Proprietários/papéis com bypass podem ignorar RLS. A autorização por membership precisa existir no Go; seu usuário de banco deverá receber privilégios mínimos explícitos na implementação. Esta migration não altera a exposição de shops/services pela Data API: a configuração existente precisa continuar sendo gerenciada separadamente.

Nunca colocar DATABASE_URL, chave secret/service_role ou senha no frontend. NEXT_PUBLIC_SUPABASE_URL e chave publishable são configuração pública do SDK. O Go só precisa de SUPABASE_URL e da conexão de banco apropriada; valida o token com chaves públicas, sem secret/service_role.

## Validação e próximos passos

Testar em PostgreSQL: unicidade de vínculo, FKs, cascata limitada a perfil/vínculos, bloqueio por anon/authenticated e repetição do provisionamento. Uma fixture mínima de auth.users permite testar integridade SQL, mas não comprova login no Supabase.

O teste `TestIdentityMigrations`, em tests/integration/identity_migrations_test.go, usa **IDENTITY_TEST_DATABASE_URL**, nunca DATABASE_URL implicitamente. Exige um banco vazio e descartável e uma conexão administrativa capaz de criar roles/schema. Recusa bases com shops/users ou auth.users existentes. Dentro de uma transação revertida, testa a sequência comum, a falha quando Auth está ausente, a FK com fixture mínima, constraints, RLS, provisionamento repetido, suspensão, cascata e Down/Up. Não use o projeto Supabase como destino deste teste.

```powershell
# Exemplo apenas para um PostgreSQL de testes separado, com credenciais locais:
$env:IDENTITY_TEST_DATABASE_URL = 'postgres://postgres:identity_test_only@127.0.0.1:55433/identity_migrations_test?sslmode=disable'
go test -tags=integration -run '^TestIdentityMigrations$' -count=1 ./tests/integration/...
```

Sem essa variável o teste é pulado, não aprovado. Os testes de catálogo usam DATABASE_URL e verificam autorização com repositório real e verifier substituído. internal/httpapi/tests/access_test.go percorre JWT/JWKS reais, casos de uso e handlers, com persistência substituída. Nenhum desses testes comprova login em um projeto Supabase de verdade.

Depois: testar manualmente a sessão real do projeto e o provisionamento local; implementar edição/ativação de serviços e os fluxos de profissionais/agendamentos. Antes de deploy, garantir HTTPS, banco com privilégios mínimos, política de sessões/expiração e observabilidade. CRUD de contas, papéis e painel do operador podem esperar.

Fontes oficiais: [vínculo com auth.users](https://supabase.com/docs/guides/auth/managing-user-data), [cadastro](https://supabase.com/docs/guides/auth/general-configuration), [verificação JWT](https://supabase.com/docs/guides/auth/jwts), [chaves/JWKS](https://supabase.com/docs/guides/auth/signing-keys), [RLS](https://supabase.com/docs/guides/database/postgres/row-level-security), [Next.js/SSR](https://supabase.com/docs/guides/auth/server-side/creating-a-client).

Decisão registrada no [ADR 0009](adr/0009-supabase-identity-without-roles.md).
