# ADR 0009 — Identidade Supabase e vínculos sem papéis

Status: adotada para a estrutura de dados; integração de login/autorização ainda pendente. Data: 2026-09-23.

## Decisão

Usar Supabase Auth como único provedor neste incremento. `public.users.id` é o mesmo UUID de `auth.users.id`; não há UUID local adicional, senha, hash ou cópia de e-mail. Isso simplifica a proposta anterior de mapear issuer/subject. O Go futuro ainda deve validar o issuer do projeto antes de usar o subject como UUID.

Manter `shop_memberships(user_id, shop_id)` com chave primária composta, atividade e data de criação. Não criar role, owner/staff, matriz de permissões nem CRUD. Cada pessoa tem conta própria; todos os vínculos ativos terão os mesmos poderes administrativos na respectiva unidade. A tabela de vínculos permite várias pessoas por unidade e várias unidades por pessoa sem misturar contas com barbearias.

Provisionamento manual: criar a identidade pelo Supabase Auth, depois inserir usuário/vínculo em transação SQL. Não criar trigger em auth.users nem conceder vínculo a partir de user_metadata. Uma identidade sem vínculo não deverá acessar o painel quando a autorização for implementada.

## Compatibilidade e integridade

O Compose atual oferece PostgreSQL puro, sem Supabase Auth. A migration comum 00004 cria users/memberships com RLS sem policies. Uma segunda sequência, em `db/supabase/migrations`, adiciona a FK real para `auth.users(id)` e revoga privilégios dos papéis de browser. Essa sequência exige Supabase e usa a tabela de versões `public.goose_supabase_version`, distinta da sequência principal. Ela falha se auth.users não existir; não há detecção que omita silenciosamente a FK.

No Supabase, aplicar AMBAS as sequências antes de provisionar. Em PostgreSQL puro, somente a sequência comum: esse ambiente testa o banco/domínio, mas não possui autenticação Supabase. Essa separação acrescenta um comando ao deploy e mantém Compose/sqlc utilizáveis sem simular um provedor real.

Excluir identidade no Auth elimina apenas o perfil local e seus vínculos em cascata. Não elimina shops/services. Para suspensão, preferir active=false: usuário afeta todas as unidades, vínculo somente uma unidade. Essas flags ainda não são verificadas pelo Go. Futuras referências de auditoria devem preservar histórico, sem propagar a cascata a dados de negócio.

## Limites desta entrega

Nenhuma alteração funcional no Go/frontend, endpoint novo, integração de SDK, segredo, conta ou migration aplicada automaticamente no banco configurado. O login continua demonstrativo e o catálogo continua no modo local existente. RLS dessas duas tabelas impede acesso direto por papéis sem bypass; não protege automaticamente rotas Go nem modifica as policies de shops/services. A conexão privilegiada da API não herda a identidade de um JWT.

Procedimento e fluxo futuro: [autenticação](../authentication-proposal.md).
