# Migrations

`00005_booking_flow.sql` acrescenta profissionais/serviços, política e expediente, bloqueios, contatos, agendamentos e idempotência. Inclui FKs compostas, exclusão GiST contra sobreposição, triggers de lock de calendário e RLS/revogação de acesso direto das roles de navegador. A extensão btree_gist vem da versão 1. O Down da versão 5 remove as tabelas novas e seus dados; preserva deliberadamente RLS/revogações em shops/services. Não utilizar Down para atualizar o banco real.

Para o banco configurado no `.env`, execute no PowerShell dentro de `backend-chat`:

```powershell
. .\scripts\Load-Env.ps1
$goose = Join-Path (go env GOPATH) 'bin\goose.exe'
& $goose -dir db/migrations postgres $env:DATABASE_URL status
& $goose -dir db/migrations postgres $env:DATABASE_URL up
& $goose -dir db/migrations postgres $env:DATABASE_URL status
```

`up` aplica apenas versões pendentes; não reaplique SQL manualmente se o Goose já registrou a versão. Faça backup e confira o destino antes de aplicar em um banco com dados reais. A conexão deve permitir DDL/RLS. Em Supabase, a sequência específica abaixo continua separada; a versão 5 não altera auth.users nem cria contas. Após aplicar, reinicie a API e use o painel Barbeiros para criar a agenda inicial. `db/seeds/provision_booking_calendar.sql` é alternativa manual opcional, não um passo obrigatório.

`00004_create_identity_tables.sql` cria users e shop_memberships sem papéis. No Supabase é obrigatório aplicar depois `../supabase/migrations`, com a tabela Goose separada `public.goose_supabase_version`, para ligar users.id a auth.users.id. O PostgreSQL puro do Compose recebe somente a sequência comum. Veja a ordem de execução e o provisionamento manual no [guia de autenticação](../../docs/authentication-proposal.md). Não há login/autorização Go implementados por estas migrations.

Goose v3.28.0, migrations SQL versionadas e transacionais por padrão. Os comandos estão no README principal. Executar a partir da raiz, com `DATABASE_URL` carregada.

`00001_enable_btree_gist.sql` prepara a extensão para a futura restrição de sobreposição. `00002_create_catalog_tables.sql` cria shops/services. `00003_service_currency.sql` adiciona moeda BRL ao catálogo, inclusive aos registros existentes. O Goose mantém sua própria tabela de versões. O usuário de migrations precisa de permissão para instalar a extensão; em um banco gerenciado, o operador pode precisar provisioná-la antes.

O arquivo db/seeds/development.sql contém apenas a barbearia de exemplo. Não é uma migration, não roda no boot e deve ser executado explicitamente em desenvolvimento. As instruções estão em [Testar catálogo](../../docs/catalog-testing.md).

O Down mantém a extensão deliberadamente, pois ela pode ser compartilhada por outros objetos. Não usar DROP EXTENSION CASCADE. Isso significa que o rollback da versão é possível, mas não remove a capacidade instalada. Executar rollback somente em banco descartável ou com revisão explícita.

Não editar migrations já aplicadas. Adicionar versões novas. A API e o worker não executam migrations automaticamente. Em deploy, usar um job único antes dos processos. O usuário de runtime deve ter permissões menores do que o de migrations.

`goose validate` verifica a estrutura dos arquivos; não substitui a execução em PostgreSQL. Para validar de verdade: aplicar em banco de desenvolvimento, repetir `up` (nenhuma mudança) e executar os testes com a tag `integration`. Testes de rollback de futuras tabelas devem usar um banco descartável.
