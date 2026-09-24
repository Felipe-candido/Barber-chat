# Autenticação: Supabase Auth e acesso por barbearia

Atualizado em 23/09/2026. **Preparado: migrations e procedimento manual. Não implementado: login real, validação JWT, autorização Go e CRUD de contas.** As migrations não são aplicadas no boot. O frontend /login ainda é uma demonstração e o POST de serviços continua dependendo do modo local DEV_SHOP_SLUG.

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
3. `db/supabase/provision_membership.sql`: procedimento manual, não é migration nem seed automático. Insere perfil/vínculo sem duplicação e recusa reativar acessos suspensos.

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

Suspender users.active afeta todas as unidades; suspender shop_memberships.active afeta somente o vínculo. Excluir no Auth elimina perfil/vínculos por cascata, sem excluir barbearia/serviços. Essas flags e ausências **só bloquearão requisições quando o Go implementar as verificações abaixo**. A estrutura de banco não representa proteção já ativa na API.

## Como o login e o Go funcionarão depois

1. Next.js autentica no Supabase, que entrega sessão e access token. Bibliotecas previstas: @supabase/supabase-js e @supabase/ssr; não instaladas nesta etapa.
2. O frontend envia `Authorization: Bearer <access_token>` ao Go. Nunca envia senha ao catálogo.
3. O Go valida assinatura, algoritmo permitido, issuer do projeto, audience esperada, expiração e subject. Para chaves assimétricas, usa JWKS do projeto com cache, rotação e timeout. Decodificar o payload não autentica. Escolheremos uma biblioteca JWT/JWKS ao implementar; não escrever criptografia própria.
4. O sub validado identifica users.id. Consultar usuário ativo, membership ativo e barbearia ativa em cada operação administrativa. O cliente pode solicitar uma unidade, mas não conceder seu próprio acesso.
5. Uma unidade elegível pode ser selecionada automaticamente; várias exigem seleção; nenhuma resulta em acesso negado. Não escolher o primeiro vínculo arbitrariamente. Não autorizar por e-mail, slug, user_metadata ou role=authenticated do Supabase.
6. Os casos de uso recebem o shop_id autorizado. O catálogo/agendamento filtra por esse tenant, inclusive em buscas por ID. Sem papéis, a decisão inicial é identidade válida + vínculo elegível.

Token ausente/inválido deverá resultar em 401; identidade válida sem acesso elegível, em 403 (podendo ocultar recursos conforme contrato futuro). Ainda não há rotas de sessão/me nem middleware JWT. A integração futura precisa substituir DEV_SHOP_SLUG nas rotas administrativas e permitir Authorization no CORS; o modo local não pode ser bypass em produção. O catálogo público por slug continua sem login.

Logout/revogação não tornam todo JWT já emitido imediatamente inválido. Consultar o estado local por requisição permitirá bloquear acesso de negócio ao suspender usuário/vínculo. Sessões, renovação, recuperação e expiração serão tratadas na implementação, sem flags de login no localStorage.

Responsabilidades: identity/infra valida token e consulta PostgreSQL; identity/application resolve identidade e autoriza unidade; domain guarda invariantes; cmd/api compõe dependências; o frontend administra a experiência de sessão. RabbitMQ não participa do login. Nenhum código Go de produção foi implementado nesta etapa.

## RLS e conexão PostgreSQL

As tabelas novas ficam com RLS e sem policies para browser; a etapa Supabase também remove grants de anon/authenticated. Não permitir ao usuário inserir seu próprio membership. O frontend consumirá dados de negócio pelo Go, sem CRUD direto via Data API.

O JWT recebido pelo Go **não configura auth.uid() na conexão pgx**. Proprietários/papéis com bypass podem ignorar RLS. A autorização por membership precisa existir no Go; seu usuário de banco deverá receber privilégios mínimos explícitos na implementação. Esta migration não altera a exposição de shops/services pela Data API: a configuração existente precisa continuar sendo gerenciada separadamente.

Nunca colocar DATABASE_URL, chave secret/service_role ou senha no frontend. Futuramente, NEXT_PUBLIC_SUPABASE_URL e chave publishable serão configuração pública do SDK; ainda não são necessárias para aplicar migrations ou cadastrar vínculos manualmente.

## Validação e próximos passos

Testar em PostgreSQL: unicidade de vínculo, FKs, cascata limitada a perfil/vínculos, bloqueio por anon/authenticated e repetição do provisionamento. Uma fixture mínima de auth.users permite testar integridade SQL, mas não comprova login no Supabase.

O teste `TestIdentityMigrations`, em tests/integration/identity_migrations_test.go, usa **IDENTITY_TEST_DATABASE_URL**, nunca DATABASE_URL implicitamente. Exige um banco vazio e descartável e uma conexão administrativa capaz de criar roles/schema. Recusa bases com shops/users ou auth.users existentes. Dentro de uma transação revertida, testa a sequência comum, a falha quando Auth está ausente, a FK com fixture mínima, constraints, RLS, provisionamento repetido, suspensão, cascata e Down/Up. Não use o projeto Supabase como destino deste teste.

```powershell
# Exemplo apenas para um PostgreSQL de testes separado, com credenciais locais:
$env:IDENTITY_TEST_DATABASE_URL = 'postgres://postgres:identity_test_only@127.0.0.1:55433/identity_migrations_test?sslmode=disable'
go test -tags=integration -run '^TestIdentityMigrations$' -count=1 ./tests/integration/...
```

Sem essa variável o teste é pulado, não aprovado. Os testes de catálogo existentes continuam usando DATABASE_URL e não validam login.

Depois: integrar sessão no Next; verificar JWT e autorizar unidade no Go; testar tokens inválidos, troca maliciosa de tenant, revogação e usuários com duas unidades. Só então habilitar acesso administrativo remoto. CRUD, papéis e painel do operador podem esperar.

Fontes oficiais: [vínculo com auth.users](https://supabase.com/docs/guides/auth/managing-user-data), [cadastro](https://supabase.com/docs/guides/auth/general-configuration), [verificação JWT](https://supabase.com/docs/guides/auth/jwts), [chaves/JWKS](https://supabase.com/docs/guides/auth/signing-keys), [RLS](https://supabase.com/docs/guides/database/postgres/row-level-security), [Next.js/SSR](https://supabase.com/docs/guides/auth/server-side/creating-a-client).

Decisão registrada no [ADR 0009](adr/0009-supabase-identity-without-roles.md).
