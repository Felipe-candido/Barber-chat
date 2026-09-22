# ADR 0007 — Serviços do catálogo e tenant para testes locais

Status: adotada. Data: 2026-09-22.

## Decisão

Implementar criar e listar serviços com casos de uso separados: NewCreateService e NewListServices. São nomes de ações explícitas; não introduzir um NewCatalog agregando responsabilidades ainda inexistentes.

O catálogo define as portas ServiceRepository e ShopResolver. O adaptador PostgreSQL de shops resolve slug ativo em UUID; catalog recebe esse recurso por injeção em cmd/api. Domínio/aplicação não importam infraestrutura nem tipos pgx. Um pool compartilhado abastece queries sqlc. Cada operação atual usa statements atômicos; não é necessário um coordenador genérico de transações.

## Identificação da barbearia e acesso local

GET público resolve o slug da rota. POST administrativo resolve exclusivamente DEV_SHOP_SLUG, configurado pelo operador. Não aceita shop_id/slug no JSON e não interpreta headers como autorização. Uma barbearia inexistente/inativa retorna 404; um catálogo vazio retorna [].

Esse POST é uma facilidade explícita de desenvolvimento: desabilitado por padrão, exige HTTP_ADDR com IP loopback, peer e Host locais, e rejeita Origin e Sec-Fetch-Site cross-site. A API registra aviso quando a escrita local está ativa. Não publicar essa configuração por proxy ou túnel. As verificações locais não equivalem a autenticação; produção requer identidade verificada e membership da barbearia antes de habilitar escrita administrativa. Uma requisição local ainda pode escrever em um banco remoto configurado: usar banco/projeto de desenvolvimento.

O INSERT confirma atividade da barbearia no próprio statement. A listagem filtra shop_id e atividade do serviço/barbearia. Isso evita confiar apenas numa leitura anterior. Não garante a semântica de locks necessária aos futuros agendamentos.

## Schema e contrato

Manter migrations 00001 e 00002 intactas. A migration 00003 adiciona moeda BRL aos preços em centavos existentes; BRL é a única moeda deste incremento. Nomes possuem até 100 caracteres Unicode. Duração positiva deve caber em INTEGER do PostgreSQL; um limite comercial menor permanece decisão de produto. Preço zero é aceito; preço omitido é rejeitado.

SQL de schema fica em migrations; queries anotadas em db/queries; seed explícito em db/seeds. Arquivos gerados são atualizados com sqlc generate. Não executar seed/migrations no boot.

Rotas do catálogo usam /api/v1, alinhadas ao contrato atualizado. JSON inválido retorna 400, modo local bloqueado 403, barbearia ausente/inativa 404, corpo acima de 64 KiB 413, media type incorreto 415, validação 422, deadline/cancelamento 503 e falha interna genérica 500. Não retornar erro bruto de banco.

## Validação e evolução

Unitários cobrem regras, prioridade do tenant resolvido, ausência de persistência inválida e fronteira HTTP local. Integração usa PostgreSQL real, transações revertidas, duas barbearias e constraints; mocks não comprovam isolamento SQL.

Editar, ativar/desativar, listar administrativamente inativos, autenticar memberships, profissionais e associações são próximos incrementos. RabbitMQ e worker não participam da criação/listagem de serviços.

O fluxo completo é explicado nos doc.go de catalog/domain, catalog/application, catalog/infra, catalog/infra/http, catalog/infra/postgres e shops/infra/postgres.
