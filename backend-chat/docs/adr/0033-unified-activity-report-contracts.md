# 0033 — Contratos unificados de histórico e indicadores

Data: 2026-10-31. Estado: contratos e regras puras da entrega G revisados;
consultas PostgreSQL, casos de uso, HTTP e painel ainda não implementados.

## Responsabilidade e acesso

Manter reporting como módulo de leitura derivada no monólito. Não criar um
segundo cadastro de atendimentos, CRM, módulo financeiro ou banco de relatórios.
Domain/application não importam booking, sales ou identity: seus adaptadores
convertem os estados e o acesso atuais para portas pertencentes a reporting.

ActivityReportInput/ActivityHistoryInput e ActivityOverview/ActivityHistoryPage
formam o contrato único para serviços, produtos ou ambos. Remover os contratos
anteriores de relatório apenas de serviços: não tinham consumidores executáveis.
Isso evita duas regras de autorização/paginação para a mesma capacidade.

Owner consulta toda a unidade, incluindo vendas sem profissional; barber somente
o profissional do acesso atual. Validar ator, unidade, membership e atividade do
profissional dentro da leitura, em todas as páginas. Não aceitar papel ou escopo
de acesso vindos do formulário. Essa decisão substitui a restrição inicial
exclusiva do owner do ADR 0014; o total geral continua exclusivo do proprietário.

## Período e filtros

- Datas inicial/final inclusivas, fuso IANA da unidade, máximo de 366 datas
  civis; traduzir para intervalo UTC [início, próximo dia após o fim).
- Atendimentos usam starts_at; vendas usam sold_at, mesmo quando vinculadas a
  atendimento de outra data. Correções afetam o período original do registro.
- Em all, serviço filtra somente atendimentos, produto filtra somente itens.
  Com origem exclusiva, rejeitar filtros de recurso/situação do outro ramo.
  Nome, telefone, período e profissional se aplicam aos dois ramos.
- Nome busca literal sem distinguir maiúsculas/minúsculas; escapar caracteres
  especiais de LIKE. Telefone exige igualdade após normalização, não busca parcial.
  Os detalhes de validação desses inputs serão implementados no caso de uso.
- Nome/telefone vêm do contato atual da unidade. Venda avulsa sem atendimento
  não corresponde a esses filtros; não inventar contato nem identificar uma
  pessoa globalmente por telefone. Não restringir consulta a catálogo ativo.

## Indicadores e granularidade

Valor registrado = serviços com estado persistido completed + itens de vendas
recorded, separados por moeda. Scheduled/confirmed são previsão; cancelled e
no_show não contribuem ao valor válido. O histórico mantém seu valor original,
inclusive após cancelamento. Cancelar atendimento não anula sua venda vinculada.

Conclusão automática entra no valor dos serviços, identificada como automática.
CompletedAutomatically é subconjunto de Completed, nunca outro atendimento.
Não completar registros durante GET nem presumir pagamento recebido. O ADR 0032
continua dono do processamento automático e das correções auditadas.

Agrupar valores de cada origem antes de combiná-los. Filtro por produto soma
somente os itens correspondentes e conta DISTINCT sale.id; não soma sales.total.
Uma venda com dois produtos pode aparecer em dois grupos: não somar a contagem
dos grupos para obter o número global de vendas. Quantidade de atendimentos,
comprovantes e unidades são medidas separadas; não existe total de “clientes”.

Usar snapshots para nomes/preços e IDs estáveis para agrupamentos. Rótulo de
grupo usa o último snapshot correspondente pela ordem do histórico. Centavos
e contadores são inteiros limitados ao intervalo exato de JSON; combinação que
exceder esse limite falha explicitamente. Não usar float para calcular valores.
Previsão fica fora da soma; não combinar moedas, calcular lucro nem chamar o
resultado de dinheiro recebido. Médias/tickets ficam para uma decisão posterior.

## Leitura e paginação a implementar

Reutilizar pool do bootstrap, em transação curta read-only/repeatable-read para
acesso, timezone e projeções coerentes. Toda origem SQL deve filtrar shop_id e
o profissional efetivamente autorizado. ObservedAt informa o instante do
snapshot da resposta; não congela páginas subsequentes diante de correções.

Histórico: um atendimento ou um item vendido por linha, com status tipado por
origem. Ordem descendente (occurred_at, source, record_id, item_id), com chave de
item não nula interna para atendimentos. Página padrão 25, máximo 100, leitura
de N+1 para detectar próxima página. Cursor opaco vinculado a ator, unidade,
período, origem, situações, filtros normalizados e profissional efetivo.
Fingerprint não autoriza; sempre refazer a autorização. A codificação/validação
do cursor será implementada junto aos casos de uso e ao adaptador HTTP.

Não retornar códigos de consulta pública, hashes, recibos criptografados ou
motivos/atores das correções neste histórico gerencial. Auditoria detalhada
permanece no endpoint administrativo do atendimento.

## Validação desta parte e próxima parte

Testes puros cobrem R$100 de serviços + R$90 de produtos = R$190, previsão
separada, estados excluídos preservando valores originais, proveniência,
contadores incoerentes, precisão monetária e escopo owner/barber. Esses testes
não comprovam SQL, isolamento de tenant, joins nem concorrência no PostgreSQL.

Próxima parte: consultas e adaptadores de leitura concretos, com testes em banco
descartável, seguidos pelos casos de uso e rotas. A entrega G só estará pronta
quando essas partes forem implementadas e testadas; a entrega H é a interface.
