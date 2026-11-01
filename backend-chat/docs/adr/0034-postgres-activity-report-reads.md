# 0034 — Leituras PostgreSQL dos relatórios de atividades

Data: 2026-10-31. Estado: consultas, adaptadores e testes PostgreSQL implementados.
Casos de uso, composição na API, codificação do cursor e HTTP ainda pendentes.
Complementa os contratos do ADR 0033, sem alterar as regras de valores.

## Consultas e granularidade

db/queries/reporting.sql projeta um atendimento ou um item vendido por linha,
por UNION ALL. Cada ramo aplica shop_id, profissional autorizado, intervalo e
filtros antes de ordenar, limitar ou agrupar. Produtos não são ligados aos
atendimentos para calcular o valor de serviços: isso duplicaria valores quando
houvesse vários itens. O vínculo opcional da venda serve à busca pelo contato.

O histórico ordena (occurred_at, source, record_id, item_id) em ordem decrescente.
O item de atendimento usa UUID zero internamente, evitando comparações com NULL.
Origem usa COLLATE C na comparação e ordenação; a chave completa desempata
vários itens de várias vendas ocorridos no mesmo instante. A validação do
fingerprint e o transporte opaco do cursor pertencem à próxima parte.

Indicadores usam GROUPING SETS para total, dia local, profissional, serviço e
produto. Contam comprovantes DISTINCT sale.id e somam somente itens que passaram
pelo filtro de produto. O Go monta as coleções a partir desses agregados; não
carrega todos os registros do período na memória para calcular os totais.
Moeda permanece explícita; o esquema atual aceita somente BRL. Cada grupo usa
o último nome histórico correspondente, mantendo o ID estável.

Busca por nome usa parâmetro ILIKE com escape de %, _ e barra invertida. Telefone
usa igualdade sobre o valor previamente normalizado. Contato é o cadastro atual
da unidade; uma correção altera as buscas posteriores. Vendas avulsas sem
atendimento não correspondem aos filtros de contato.

## Fronteiras e transação

identity/infra/postgres/reporting.go adapta o acesso atual para ActivityAudience:
usuário, membership e unidade ativos; barber também exige profissional ativo.
Owner não precisa que o profissional histórico esteja ativo para consultar.
shops/infra/postgres/reporting.go fornece somente ID/fuso da unidade ativa.
As consultas dessas dependências ficam em reporting_dependencies.sql e são
geradas pelo sqlc, assim como as projeções de atividades.

O transactor recebe o pool compartilhado e uma função de composição dos
adaptadores. Abre REPEATABLE READ, READ ONLY e contexto limitado a 30 segundos
(ou ao prazo menor do chamador). Todos os adaptadores recebem as mesmas Queries
vinculadas à transação. ObservedAt vem de transaction_timestamp do PostgreSQL,
indicando o início da leitura; o histórico pode usar o mesmo metadado no caso de
uso futuro. Rollback possui contexto próprio e limitado para liberar a conexão.

Os readers recebem uma consulta interna já autorizada. Não são endpoints e não
aceitam diretamente JSON de usuário. O futuro caso de uso deve consultar Access,
resolver ProfessionalFor, carregar o fuso de Shops, normalizar filtros e então
consultar os readers. IDs/fingerprint não substituem esse passo. O middleware
também não dispensa a verificação atual dentro da leitura. Nenhuma rota foi
habilitada nesta entrega.

Uma correção concorrente não altera os resultados da transação já iniciada;
a próxima transação observa a mudança. Isso não congela o histórico entre
requisições/páginas. Escritas feitas acidentalmente nessa transação são rejeitadas
pelo PostgreSQL, além da separação arquitetural entre leitura e comandos.

## Índices e validação

A migration 00020 acrescenta índices B-tree de appointments por unidade/data/ID
e por unidade/profissional/data/ID, incluindo cancelados. O GiST de sobreposição
é parcial e não cobre todo o histórico nem a ordenação por starts_at. Sales já
possui índices equivalentes e sale_items possui chave por unidade/venda/produto.
Não alterar constraints de reservas, snapshots, vendas ou estoque.

A migration foi validada com up/down/up no banco Docker descartável
codex_reporting_20261031. Não aplicada ao Supabase. O EXPLAIN inspecionado com
45 atendimentos escolheu sequential scan, adequado ao pequeno conjunto local;
isso não é uma medição de desempenho em produção. Medir com volume representativo
antes de adotar caches, materializações ou mais índices.

Dez cenários PostgreSQL passaram: vazio/limites, exemplo R$190 e snapshots
inativos, filtros/itens/contato atual, ausências/cancelamentos/previsão, isolamento
por unidade/profissional, paginação empatada, limites civis/DST, cancelamento
concorrente com snapshot coerente, revogação/transação somente de leitura e
proveniência automática/limites monetários. As fronteiras dos módulos e os testes
puros de reporting também passaram; sqlc generate e go mod tidy concluíram.

Após uma falha temporária da revisão automática por limite de uso, os checks
foram executados: go vet ./..., go test ./..., builds da API/worker e validação
do Compose passaram. O container descartável foi parado ao final, preservando
seus dados e volumes. Nenhuma mudança foi aplicada ao Supabase.

Próximo incremento: casos de uso de resumo/histórico, normalização/validação dos
inputs, vinculação do cursor ao ator/escopo/filtros e testes de aplicação. Depois,
handlers, composição no bootstrap e OpenAPI das rotas efetivamente registradas.
