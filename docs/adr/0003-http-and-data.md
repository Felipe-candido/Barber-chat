# ADR 0003 — Chi, pgx e SQL explícito

Status: adotada; Chi escolhido pelo usuário. Data: 2026-09-17.

Atualização: o [ADR 0006](0006-automatic-dotenv.md) adiciona o parser de `.env` para inicialização automática local; substitui a decisão inicial de dispensar um parser de configuração.

## Decisão

Chi v5 sobre `net/http`, para aprender um framework/router preservando handlers padrão. `slog`, `context`, `os` e `testing` cobrem logging, cancelamento, configuração e testes. `pgx/v5` com pool para PostgreSQL; Goose CLI fixado para migrations SQL. Cliente `rabbitmq/amqp091-go` para AMQP.

## Consequências

Chi evita adotar contextos de handler próprios de Gin/Echo neste começo, mas exige selecionar middlewares e implementar validação explicitamente. Consultas futuras usam SQL parametrizado; sqlc poderá ser incorporado na primeira fatia de persistência relevante. Nenhum ORM, gerador, container de DI ou parser de configuração é necessário agora. Não existem queries de negócio ou diretórios vazios para representá-las.
