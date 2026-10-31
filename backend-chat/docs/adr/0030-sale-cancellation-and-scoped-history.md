# 0030 — Consulta de vendas e anulação com reposição explícita

Data: 2026-10-31. Estado: entrega D implementada na API. Interface permanece na entrega E.

## Fronteiras e consultas

Sales continua dono da venda, seus snapshots e anulação. Catalog mantém o saldo e
o histórico de movimentos; shops fornece somente seu fuso por uma porta consumida
por sales. Domain/application não importam módulos vizinhos nem pgx/HTTP.
Bootstrap injeta os adaptadores em uma única transação/pool compartilhados.

SaleQueries.List recebe datas civis inclusivas from/through, até um ano,
anos 1900–9998, e as converte para intervalo UTC [from,until) no fuso cadastrado.
Não somar 24 horas para avançar uma data civil. Limite padrão 25, máximo 100;
ordenação por sold_at/id decrescentes e cursor vinculado ao ator, tenant,
profissional efetivo, período, estado e fuso. Cursor representa posição, não acesso;
a permissão atual é sempre revalidada e o SQL filtra o profissional antes do LIMIT.
Parâmetros desconhecidos/repetidos são erro, evitando filtros silenciosamente ignorados.

Proprietário vê todas as vendas, inclusive atribuídas a outros usuários ou a
profissionais inativos. Barber vê apenas as do profissional ativo do vínculo
atual. Detalhe fora do escopo retorna 404; filtro explícito de outro profissional
por barber retorna 403. A página carrega cabeçalhos e itens em duas consultas em
lote, sem N+1. Itens são imutáveis; cada cabeçalho contém seu estado e auditoria
na mesma linha. Paginação não é snapshot congelado entre requisições: mudanças
posteriores exigem atualizar a lista, como em outros históricos do sistema.

## Anulação e idempotência

CancelSale exige owner tanto no middleware quanto na autoridade revalidada dentro
da transação. Recebe versão esperada, motivo normalizado de 1–500 caracteres sem
controles, restock booleano explícito e chave de 32 bytes em hex minúsculo.
Fingerprint HMAC usa um formato canônico privado com protocolo 1; operação
cancel_sale possui namespace distinto de record_sale. Não persistir a chave bruta.

1. Validar entrada e acesso preliminar de owner antes de reivindicar a chave.
2. Reivindicar/comparar comando e travar/revalidar usuário, membership e unidade.
3. Travar cabeçalho da venda com FOR NO KEY UPDATE. Chaves diferentes para a
   mesma venda ficam serializadas nesse ponto.
4. Replay do comando retorna a anulação salva. Pedido com outra chave e mesmas
   instruções também retorna o estado salvo; não atribuir novamente ator/data,
   não incrementar versão e não gerar movimentos. Motivo/reposição diferentes
   resultam em conflito; a primeira transição requer versão 1 atual.
5. Registrar status=cancelled, version=2 e auditoria na própria linha da venda.
   Cabeçalho é o registro imutável dessa única transição, sem segunda tabela
   duplicando motivo/ator/data. Itens, preço, total, atribuição e sold_at permanecem.
6. Se restock=true, ler as saídas originais imutáveis e criar estornos integrais,
   ordenados por produto. Quantidades/IDs não vêm do navegador. Produto inativo
   aceita devolução. Catalog aplica o saldo por seu trigger AFTER INSERT.
7. Completar comando e carregar comprovante persistido antes do commit; timestamp
   de anulação usa microssegundos para coincidir com PostgreSQL e com JSON dos lotes.
8. Commit valida constraints diferidas. Falha após qualquer item ou excesso do
   limite inteiro de saldo desfaz anulação, estornos, saldo e comando inteiro.

Não há devolução parcial, reabertura de venda nem reembolso de pagamento nesta
versão. Cancelar venda não altera estado do agendamento; cancelar agendamento
também não estorna produtos automaticamente. Restock=false mantém a mercadoria
retirada: somente motivo/estado financeiro mudam. Futuro relatório exclui vendas
anuladas no período de sold_at; movimentação física de retorno ocorre em cancelled_at.
Valor da venda não comprova recebimento de dinheiro.

## Proteções da migration 00018

Migration nova, sem alterar a 00017. CHECK exige estado/versão/auditoria completos.
Guard do cabeçalho permite somente recorded→cancelled e rejeita alteração de
qualquer outro campo, edição posterior ou DELETE. Guard de itens permanece.

Movimento sale_reversal contém sale_item_id e reverses_movement_id. FKs compostas
exigem mesma unidade/produto/item/quantidade, e trigger exige origem kind=sale,
mesmo ator/motivo/data da anulação e restock=true. Índice único por saída permite
somente um estorno integral. A quantidade derivada positiva é a mesma do item;
não basta copiar um UUID sem a quantidade e o tenant.

Constraints diferidas exigem a saída de cada item e, se solicitado, exatamente
um estorno por item no commit. Sem solicitação não pode existir estorno. O trigger
de saldo continua AFTER INSERT, compatível com ON CONFLICT e FOR NO KEY UPDATE.
Não usar o endpoint de ajuste manual para estornar venda.

Comandos passam a ter FK de tenant/venda e guard específico de operação:
record_sale continua exigindo o ator que registrou a venda; cancel_sale pode ser
executado por outro owner, inclusive para venda de um barber. Comando incompleto
continua proibido no commit. Autorização de perfil é responsabilidade da API;
administrador do banco é uma fronteira de confiança. RLS/grants anteriores
permanecem e novas funções têm EXECUTE público/browser revogado.

Down recusa banco com vendas/comandos para não apagar auditoria; validação de
up/down/up somente em banco descartável vazio. Nenhuma migration no Supabase
é executada por esta entrega.

## Validação e próxima entrega

PostgreSQL real: replays com mesma/outra chave, chaves simultâneas, anulações
paralelas com nova venda, produto inativo, owner anulando venda de barber,
revogação, estoque excedido, rollback após primeiro estorno, FK/origem/quantidade,
histórico imutável e commit de anulação incompleta recusado. Consultas verificam
fronteiras do dia em São Paulo, paginação, filtros, escopo, JSON/CORS e no-store.
Regressões das entregas B/C continuam obrigatórias. Contrato em sales-openapi.yaml.

Próxima entrega E: conectar catálogo, estoque e vendas ao painel, mantendo a
decisão explícita de reposição e a recuperação de respostas incertas com mesma
chave/corpo. Relatórios e conclusão/no-show continuam nas entregas posteriores.
