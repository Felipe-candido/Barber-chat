# ADR 0009 — Identidade Supabase e vínculos sem papéis

Status: adotada; HTTP integrado em 2026-09-30 e seleção de unidades no frontend em 2026-10-01. Decisão original: 2026-09-23.

## Decisão

Usar Supabase Auth como único provedor neste incremento. `public.users.id` é o mesmo UUID de `auth.users.id`; não há UUID local adicional, senha, hash ou cópia de e-mail. Isso simplifica a proposta anterior de mapear issuer/subject. O Go futuro ainda deve validar o issuer do projeto antes de usar o subject como UUID.

Manter `shop_memberships(user_id, shop_id)` com chave primária composta, atividade e data de criação. Não criar role, owner/staff, matriz de permissões nem CRUD. Cada pessoa tem conta própria; todos os vínculos ativos terão os mesmos poderes administrativos na respectiva unidade. A tabela de vínculos permite várias pessoas por unidade e várias unidades por pessoa sem misturar contas com barbearias.

Provisionamento manual: criar a identidade pelo Supabase Auth, depois inserir usuário/vínculo em transação SQL. Não criar trigger em auth.users nem conceder vínculo a partir de user_metadata. Uma identidade sem vínculo não deverá acessar o painel quando a autorização for implementada.

## Compatibilidade e integridade

O Compose atual oferece PostgreSQL puro, sem Supabase Auth. A migration comum 00004 cria users/memberships com RLS sem policies. Uma segunda sequência, em `db/supabase/migrations`, adiciona a FK real para `auth.users(id)` e revoga privilégios dos papéis de browser. Essa sequência exige Supabase e usa a tabela de versões `public.goose_supabase_version`, distinta da sequência principal. Ela falha se auth.users não existir; não há detecção que omita silenciosamente a FK.

No Supabase, aplicar AMBAS as sequências antes de provisionar. Em PostgreSQL puro, somente a sequência comum: esse ambiente testa o banco/domínio, mas não possui autenticação Supabase. Essa separação acrescenta um comando ao deploy e mantém Compose/sqlc utilizáveis sem simular um provedor real.

Excluir identidade no Auth elimina apenas o perfil local e seus vínculos em cascata. Não elimina shops/services. Para suspensão, preferir active=false: usuário afeta todas as unidades, vínculo somente uma unidade. Essas flags são verificadas pelo Go nas requisições administrativas. Futuras referências de auditoria devem preservar histórico, sem propagar a cascata a dados de negócio.

## Limites da entrega original (2026-09-23)

Nenhuma alteração funcional no Go/frontend, endpoint novo, integração de SDK, segredo, conta ou migration aplicada automaticamente no banco configurado. O login continua demonstrativo e o catálogo continua no modo local existente. RLS dessas duas tabelas impede acesso direto por papéis sem bypass; não protege automaticamente rotas Go nem modifica as policies de shops/services. A conexão privilegiada da API não herda a identidade de um JWT.

Procedimento e fluxo futuro: [autenticação](../authentication-proposal.md).

## Implementação HTTP (2026-09-30)

cmd/api compõe uma instância compartilhada do verifier ES256/JWKS e um único pool
PostgreSQL. Authenticate resolve usuário local ativo; AuthorizeShopAction exige
usuário, unidade e vínculo ativos. Middleware HTTP coloca identidade e escopo em
requestctx, com chaves privadas/tipos separados. Application recebe parâmetros
explícitos e não depende do contexto HTTP. /admin/me exige apenas identidade;
POST /admin/shops/{slug}/services exige também o vínculo exato.

A rota antiga /admin/services permanece para o frontend, com DEV_SHOP_SLUG, mas
usa a mesma autenticação/autorização; não há bypass local. CORS foi centralizado,
com origem explícita e preflight fora da cadeia de autenticação. Falhas de
credencial, acesso e dependência retornam respectivamente 401, 403 e 503;
inconsistências inesperadas retornam 500 sanitizado. Permissões não são cacheadas.

Sem roles, todos os membros ativos continuam com os mesmos poderes. O verifier
não consulta revogação de sessão por requisição; suspensão local é relida nos
checks, e não cancela uma operação já autorizada. Credenciais/login/refresh seguem
no Supabase/frontend. Nenhuma migration ou conta é criada no bootstrap.

## Seleção de unidades — aplicação e leitura (2026-10-01)

`ListMyShops` pertence a identity porque responde quais unidades um usuário
autenticado pode acessar. Ele recebe o UUID confirmado pela autenticação, relê
o perfil local e depende de `AccessibleShopReader` para obter ID, nome e slug.
Não escolhe uma unidade automaticamente nem concede autorização permanente.

No monólito com PostgreSQL compartilhado, o Repository de identity implementa
essa porta com uma projeção somente de leitura: um JOIN entre users,
shop_memberships e shops, restrito ao usuário e aos três estados ativos. A
chave composta do vínculo evita duplicatas; nome e ID determinam a ordenação.
O acoplamento ao schema de shops é uma escolha explícita para obter o resumo em
uma consulta, evitando uma consulta por vínculo. A porta permite substituir
essa implementação caso os limites de persistência dos módulos mudem.

Shops permanece dono do cadastro, slug e configurações da barbearia; o adaptador
de identity não altera esses dados. A consulta retorna DTOs da aplicação, sem
expor tipos sqlc. `ListMembershipsByUser` continua incluindo vínculos inativos,
pois não é a consulta da seleção. O mesmo recurso de queries/pool atende ambas
as interfaces, sem conexão ou repositório genérico adicional.

O endpoint GET /api/v1/admin/shops e o bootstrap estão conectados. Após o login,
o frontend chama /me e /shops, mostra a seleção em /admin e abre
/admin/{slug}/servicos. A lista fica somente em memória e não constitui prova
de permissão: GET/POST /api/v1/admin/shops/{slug}/services verificam novamente o
vínculo atual no banco. O contexto HTTP tipado permanece fora da application.

As configurações DEV_SHOP_SLUG/NEXT_PUBLIC_SHOP_SLUG e a rota legada de criação
foram removidas neste incremento. Não se escolhe o primeiro vínculo nem se
persiste uma unidade fixa. O SDK Supabase mantém e renova a sessão; logout
local limpa o estado da interface sem prometer revogação imediata de todo JWT.
CRUD de contas, papéis e agenda permanecem fora do escopo.
