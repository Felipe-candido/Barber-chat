# Migrations

Goose v3.28.0, migrations SQL versionadas e transacionais por padrão. Os comandos estão no README principal. Executar a partir da raiz, com `DATABASE_URL` carregada.

`00001_enable_btree_gist.sql` prepara a extensão para a futura restrição de sobreposição. `00002_create_catalog_tables.sql` cria shops/services. `00003_service_currency.sql` adiciona moeda BRL ao catálogo, inclusive aos registros existentes. O Goose mantém sua própria tabela de versões. O usuário de migrations precisa de permissão para instalar a extensão; em um banco gerenciado, o operador pode precisar provisioná-la antes.

O arquivo db/seeds/development.sql contém apenas a barbearia de exemplo. Não é uma migration, não roda no boot e deve ser executado explicitamente em desenvolvimento. As instruções estão em [Testar catálogo](../../docs/catalog-testing.md).

O Down mantém a extensão deliberadamente, pois ela pode ser compartilhada por outros objetos. Não usar DROP EXTENSION CASCADE. Isso significa que o rollback da versão é possível, mas não remove a capacidade instalada. Executar rollback somente em banco descartável ou com revisão explícita.

Não editar migrations já aplicadas. Adicionar versões novas. A API e o worker não executam migrations automaticamente. Em deploy, usar um job único antes dos processos. O usuário de runtime deve ter permissões menores do que o de migrations.

`goose validate` verifica a estrutura dos arquivos; não substitui a execução em PostgreSQL. Para validar de verdade: aplicar em banco de desenvolvimento, repetir `up` (nenhuma mudança) e executar os testes com a tag `integration`. Testes de rollback de futuras tabelas devem usar um banco descartável.
