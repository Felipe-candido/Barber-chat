# Produtos, vendas, estoque e relatório unificado — plano de implementação

Atualização: 2026-10-31. Estado: entregas A–F implementadas; primeira parte de G (contratos e regras dos indicadores) revisada. Consultas, casos de uso e rotas de G e interface de H ainda pendentes.

Decisão confirmada pelo usuário: incluir catálogo, registro de vendas e controle de estoque desde a primeira versão. Implementar em partes, explicando os arquivos e validando cada entrega antes da seguinte. Cortesia fica adiada.

## 1. Resultado esperado no painel

- Produtos: nome, descrição opcional, preço de venda, saldo disponível, estoque mínimo e situação ativo/inativo.
- Movimentar estoque: registrar entrada de mercadoria ou ajuste, com quantidade, motivo e responsável.
- Registrar venda: selecionar produtos/quantidades, revisar preços/totais e confirmar; baixa automática de estoque.
- Vender avulso ou vincular a um atendimento da mesma unidade. O cliente não precisa ter conta para uma venda.
- Consultar vendas e anular um registro incorreto com motivo e decisão explícita sobre reposição do estoque.
- Relatórios: período + origem (atendimentos, produtos ou ambos), com filtros e totais separados e combinados.

A primeira versão utiliza produtos contados em unidades inteiras e BRL. O valor da venda vem do catálogo no servidor. Preço de custo, lucro, parcelamento, fiado, comissão, fornecedores e emissão fiscal são evoluções que exigem regras próprias. O relatório inicial mede valores registrados; não presume conciliação de dinheiro recebido.

## 2. Responsabilidades dos módulos

| Módulo | Responsabilidade nesta entrega |
| --- | --- |
| catalog | Cadastro/preço dos produtos, saldo e histórico de movimentações de estoque |
| sales — novo | Venda, itens, responsável, vínculo opcional com atendimento, idempotência e anulação |
| booking | Agendamento e transições administrativas de conclusão/ausência |
| reporting | Histórico e agregações de atendimentos/produtos; somente leitura |
| identity | Acesso por unidade e escopo owner/barber |
| shops | Identidade, atividade e fuso da unidade |

O novo módulo sales se justifica porque uma venda tem ciclo próprio e pode existir sem agendamento. Produto é uma oferta do catálogo; venda é uma transação. Estoque começa junto de catalog, porque existe um único saldo por produto/unidade e as regras são pequenas. Reavaliar um módulo inventory se surgirem depósitos, transferências ou compras com ciclo próprio.

Permanece um monólito modular com um go.mod e o pool compartilhado. Não é necessário novo microsserviço ou fila para confirmar uma venda.

Domain/application não importam outros módulos de negócio. Sales declara portas específicas para autorização atual, leitura dos produtos/preços, baixa de estoque e vínculo com atendimento. Adaptadores dos módulos envolvidos implementam essas projeções dentro da mesma transação PostgreSQL, montadas em cmd/api.

Reporting possui projeções próprias e pode consultar as tabelas via SQL revisado, sempre somente leitura e com shop_id. Não passa por casos de uso que alteram vendas/estoque.

## 3. Modelo proposto

Todos os registros de negócio têm shop_id. Referências a produto, atendimento, profissional, venda e item usam FKs compostas com shop_id, conforme o recurso. UUID isolado não prova acesso.

### products — catalog

- id, shop_id, name, description, price_cents, currency=BRL, active, version, created_at, updated_at.
- Preço positivo em centavos inteiros; nome até 100 caracteres. Descrição opcional.
- Alterar nome/preço exige versão esperada; mudanças não alteram vendas antigas.
- Desativação conserva referências históricas. SKU/código de barras pode ser adicionado posteriormente.

### product_stock — catalog

- shop_id, product_id, quantity_on_hand, minimum_quantity e version.
- Uma linha por produto/unidade; quantidades inteiras e não negativas.
- Novo saldo começa em zero. Quantidade inicial entra como movimento de abertura.
- Editar o cadastro não modifica o saldo: entradas e ajustes usam operação específica, auditada.

### product_stock_movements — catalog

- id, shop_id, product_id, quantity_delta, reason/type, actor_user_id, created_at.
- Origem de venda: sale_item_id; anulação pode referenciar o movimento original como reverses_movement_id.
- Tipos iniciais: opening, receipt, adjustment, sale e sale_reversal.
- Entrada/abertura positivas; venda negativa; ajuste tem sinal explícito e motivo obrigatório.
- Movimento persistido é imutável. Corrigir gera movimento compensatório.
- Unicidade por origem impede duas baixas do mesmo item; uma reversão total do movimento original no máximo uma vez.
- Movimentos ligados a itens/reversões preservam shop_id/product_id por referências compostas. Não usar source_id genérico sem referência verificável.

O plano utiliza um trigger AFTER INSERT de movimento para aplicar o delta ao saldo, com UPDATE condicional e rejeição de saldo negativo. Assim, inserir o movimento e atualizar o saldo formam uma operação atômica no banco. Uma origem duplicada não pode atualizar o saldo novamente; evitar efeitos colaterais em BEFORE INSERT combinados com ON CONFLICT DO NOTHING.

O papel da API terá os privilégios mínimos necessários. A aplicação não poderá editar saldo diretamente nem alterar/excluir movimentos. Se a função precisar de privilégios elevados, terá proprietário e search_path explícitos e acesso restrito. Credenciais administrativas do banco continuam sendo uma fronteira de confiança; RLS não restringe o proprietário/superusuário automaticamente.

CHECK protege o saldo de uma linha; não comprova sozinho que ele equivale à soma de outras linhas. Essa consistência será garantida pelo caminho de escrita controlado, pelo trigger e por testes de reconciliação entre saldo e movimentos.

### sales e sale_items — sales

Venda:

- id, shop_id, status (recorded/cancelled), sold_at, recorded_by_user_id.
- professional_id opcional para atribuição; appointment_id opcional.
- professional_name_snapshot, total_cents, currency, version, created_at.
- cancelled_at, cancelled_by_user_id, cancellation_reason e restock, quando anulada.

Item:

- id, shop_id, sale_id, product_id, quantity.
- product_name_snapshot, unit_price_cents, currency, line_total_cents.
- Quantidade positiva; limites de itens/quantidades/valores definidos no contrato, com proteção de overflow na multiplicação e soma.
- Um produto aparece uma vez por venda: normalizar a entrada e impor unicidade (shop_id, sale_id, product_id).
- Snapshots são selecionados pelo servidor e ficam imutáveis depois da confirmação.

O total nasce da soma dos itens verificados pelo servidor. CHECK valida o total de cada linha; consistência do cabeçalho com os itens será feita pela operação transacional controlada, não por um CHECK que consulta outras linhas.

### Repetição e auditoria

- Chave de idempotência por unidade, ator e operação, com fingerprint da entrada normalizada e resultado recuperável.
- Mesmo pedido/chave recupera o registro; entrada diferente retorna conflito.
- Resposta incerta mantém chave/corpo no frontend; não gerar nova chave nem repetir automaticamente.
- Respostas repetidas revalidam o acesso atual antes de revelar a venda.
- Auditoria de alterações relevantes: ator, instante e ação; sem tokens ou dados pessoais em logs.
- Anulação tem sua própria operação idempotente e controle de versão.

## 4. Permissões e habilitação comercial propostas

| Ação | Proprietário | Barbeiro |
| --- | --- | --- |
| Consultar produtos ativos, preço e saldo | Sim | Sim |
| Criar/editar/desativar produto | Sim | Não |
| Entrada e ajuste de estoque | Sim | Não |
| Registrar venda | Sim | Sim, atribuída ao seu profissional ativo |
| Escolher outro profissional responsável | Sim | Não |
| Consultar vendas | Toda a unidade | Somente as atribuídas ao seu profissional |
| Anular venda e decidir reposição | Sim | Solicita ao proprietário |
| Relatório completo e estoque gerencial | Sim | Não |
| Relatório pessoal de atendimentos/vendas | Sim | Somente seu profissional, imposto no servidor |

Essas são regras propostas para manter os dois perfis existentes, sem editor de permissões. Confirmar a experiência de relatório pessoal antes da interface.

Actor é sempre o usuário autenticado. Atribuição ao profissional é uma informação distinta: owner pode registrar venda em nome de um barbeiro; sem atribuição, ela pertence ao grupo da unidade, exibido como “Sem barbeiro”.

Consulta/correção de registros existentes e configuração do catálogo permanecem disponíveis após perda de cobertura. Para unidades self_service, a proposta é exigir a mesma habilitação comercial vigente ao confirmar novas vendas que já exigimos para novas reservas, revalidada na transação; publicação do link não é necessária. Unidades manuais preservam a compatibilidade atual. Replay autorizado de uma venda já salva não efetua nova operação comercial. Validar essa política na entrega dos contratos.

## 5. Transação de registrar venda

1. Autenticar e resolver slug/perfil; extrair usuário/unidade do contexto.
2. Validar quantidade, itens únicos e limites; normalizar a entrada para idempotência.
3. Iniciar transação, revalidar autoridade e recuperar eventual resultado da mesma chave antes de testar estoque/cobertura para uma venda nova.
4. Para venda nova, revalidar habilitação comercial e profissional atribuído. O servidor impõe o profissional do barber.
5. Se houver atendimento, verificar mesma unidade e profissional compatível. Inicialmente aceitar scheduled/confirmed/completed; cancelled/no_show não aceitam novo vínculo. Vincular não conclui o atendimento automaticamente.
6. Obter locks em ordem consistente com os fluxos existentes e ordenar produtos por UUID, incluindo linhas de saldo, para operações com vários produtos.
7. Ler produtos ativos e preços/versionamentos atuais dentro dessa proteção. Se o preço revisado pelo usuário mudou, devolver conflito para nova revisão, sem venda parcial.
8. Gravar venda e itens com snapshots e totais calculados no servidor.
9. Inserir movimentos de saída dos itens, na ordem definida; o trigger aplica as baixas. Falta em qualquer produto aborta tudo.
10. Persistir resultado idempotente e confirmar a transação.
11. Retornar comprovante da venda; frontend atualiza vendas/estoque por consulta.

Conferir o saldo no formulário é apenas informativo. A decisão final acontece no banco. Se existe uma unidade e dois barbeiros vendem simultaneamente, apenas uma transação pode consumir essa unidade. A outra recebe conflito de estoque, sem venda parcial nem saldo negativo.

O esquema de locks completo deve ser definido antes dos adaptadores, considerando identidade/profissional, vínculo com booking, produtos e saldo. Evitar rotas que adquiram os mesmos locks em ordem inversa. Nenhuma chamada HTTP externa ocorre dentro dessa transação.

## 6. Entradas, ajustes e anulação

Entrada: proprietário registra quantidade recebida e motivo; cria movimento positivo com idempotência. Estoque mínimo gera indicador na lista, sem worker necessário.

Ajuste: selecionar “Adicionar” ou “Retirar”, quantidade positiva e motivo. Retirada não pode deixar saldo negativo. Auditoria identifica quem registrou a correção; não permitir sobrescrever o campo de saldo.

Anulação: proprietário informa motivo e escolhe explicitamente se repõe os produtos. Repor só faz sentido quando a baixa era indevida ou os produtos efetivamente retornaram. Sem retorno físico, o saldo não aumenta. A venda anulada sai dos valores válidos e permanece no histórico.

Anular e repor são persistidos na mesma transação; a reversão por item ocorre no máximo uma vez. Anulação de produto agora inativo ainda pode repor sua quantidade. Venda já anulada retorna seu estado atual, sem nova reposição. Devolução parcial é evolução posterior, com itens/quantidades próprios.

Cancelar um agendamento vinculado não anula a venda automaticamente. São fatos distintos; seu vínculo é para consulta e atribuição, não para duplicar ou sincronizar seus ciclos sem confirmação.

## 7. Relatório: atendimentos, produtos ou ambos

Filtros principais:

- Período de/até, interpretado no fuso da unidade, com limite inicial de 366 datas civis.
- Origem: Todos / Atendimentos / Produtos.
- Profissional: todas as opções para owner; próprio profissional para barber.
- Serviço, aplicável ao ramo de atendimentos.
- Produto, aplicável ao ramo de vendas, incluindo produtos atualmente inativos.
- Histórico com filtros de situação e paginação. Busca por telefone permanece nos registros com contato associado; venda avulsa anônima não inventa um contato.

Em “Todos”, filtro de produto restringe somente os itens vendidos; filtro de serviço restringe somente os atendimentos. Filtros de período/profissional valem para os dois ramos. Interface explica essa regra e oferece limpar filtros. Selecionar apenas um ramo oculta/reseta filtros incompatíveis.

Datas e significado:

- Atendimentos: starts_at no fuso da unidade, preservando a decisão do ADR 0014. Somente completed entra no valor realizado; agendados/confirmados aparecem como previsão separada.
- Produtos: sold_at da venda; itens de vendas recorded entram no valor vendido. Nome/preço históricos vêm dos snapshots. Sale vinculada não herda a data do agendamento.
- Na primeira versão, anulação exclui a venda do período original e corrige seus totais históricos. A auditoria mantém a data real da anulação. Isso não é um relatório de fluxo de caixa por data de estorno.
- Movimentos de estoque têm sua própria data de ocorrência, inclusive reposições. Não reescrever datas de movimentos anteriores.

Indicadores:

- Valor dos atendimentos realizados.
- Valor dos produtos vendidos.
- Total combinado: soma desses valores, na mesma moeda.
- Quantidade de atendimentos concluídos.
- Quantidade de vendas e unidades de produtos, em contadores separados.
- Série diária com duas parcelas, serviços/produtos; agrupamentos por profissional e produto.
- Valores anulados/cancelados identificados separadamente do total válido.

Exemplo de aceite: dois cortes de R$50 concluídos + três pomadas de R$30 vendidas = R$100 em serviços, R$90 em produtos e R$190 combinados. São dois atendimentos e três unidades de produto; o número de vendas depende de quantos comprovantes foram registrados. Uma pomada vinculada a um corte entra uma vez como produto, sem aumentar de novo o valor do serviço.

Não somar atendimentos depois de join direto com vários itens vendidos: isso multiplicaria seu valor. Agregar cada origem primeiro ou usar UNION ALL de projeções com granularidade definida. Cartões, gráfico e grupos devem observar um snapshot de leitura coerente.

O indicador inicial será chamado “Valor registrado”, com parcelas explícitas. Não chamar de lucro ou dinheiro recebido. Custo de mercadoria e registro de pagamentos ficam para uma decisão própria.

## 8. Telas e rotas propostas

Navegação inicial: Produtos, Vendas e Relatórios, respeitando o escopo do perfil. Estoque é gerenciado dentro de Produtos, sem uma seção independente.

- Produtos: busca, preço, saldo, alerta de mínimo; owner tem cadastro/edição e botão “Movimentar estoque”.
- Vendas: histórico simples, botão “Registrar venda”, seleção de vários itens, quantidades, total e confirmação. Conta barber já inicia com seu profissional.
- Agenda: ação “Registrar venda” abre o mesmo formulário com atendimento/profissional preenchidos.
- Relatórios: período e origem no topo, três valores principais, gráfico diário e lista detalhada/paginada. Consulta de receita geral permanece exclusiva do owner.

Rotas administrativas propostas, ainda não registradas:

- GET/POST /api/v1/admin/shops/{slug}/products
- PUT /api/v1/admin/shops/{slug}/products/{productID}
- PUT /api/v1/admin/shops/{slug}/products/{productID}/stock-minimum
- GET/POST /api/v1/admin/shops/{slug}/products/{productID}/stock-movements
- GET/POST /api/v1/admin/shops/{slug}/sales
- GET /api/v1/admin/shops/{slug}/sales/{saleID}
- POST /api/v1/admin/shops/{slug}/sales/{saleID}/cancel
- POST /api/v1/admin/shops/{slug}/appointments/{appointmentID}/complete
- POST /api/v1/admin/shops/{slug}/appointments/{appointmentID}/no-show
- GET /api/v1/admin/shops/{slug}/reports/activity
- GET /api/v1/admin/shops/{slug}/reports/activity/history

Contratos finais definirão escopo, versão esperada, idempotência e paginação antes de registrar as rotas. Rota acessível por barber sempre impõe profissional do contexto no servidor. Não aceitar actor_id/shop_id/status/totais como autoridade vinda do JSON.

## 9. Entregas em ordem, para estudar e implementar

| Entrega | Implementação | Critério de conclusão / aprendizado |
| --- | --- | --- |
| A — Contratos e regras | Tipos Product/StockMovement em catalog; base concreta de sales com Sale/Item; DTOs/ports/errors; complementar contratos de reporting | Explicar a fronteira dos módulos, snapshots, centavos, versão e autorização; testes das regras puras |
| B — Catálogo e estoque | Nova migration, constraints/RLS/privilégios/trigger, queries/sqlc, repositórios e rotas de produto/movimentos | Criar produto, registrar abertura/entrada/ajuste e listar saldo; comprovar restrições e isolamento no PostgreSQL |
| C — Venda transacional | RegisterSale, portas dos participantes, idempotência, projeção de booking, baixa e comprovante | Uma unidade com duas vendas concorrentes permite somente uma; falha parcial desfaz tudo; resposta perdida não duplica baixa |
| D — Consulta e correção | Listar/detalhar vendas, CancelSale, reposição opcional e auditoria | Anulação repetida não repõe duas vezes; produto inativo pode receber reversão; barcode/estoque parcial não são pré-requisitos |
| E — Interface de produtos/vendas | Páginas e formulário compartilhado com agenda; estado incerto e atualização dos dados | Owner configura; barber vende no próprio escopo; revisão de preço, estoque insuficiente, teclado e mobile funcionam |
| F — Concluir atendimentos | Conclusão automática após ends_at, ausência/cancelamento como exceções e correção auditada no booking | Job idempotente no servidor; não sobrescrever decisão manual; versão/tenant e origem automática preservados; sem marcar pagamento como recebido |
| G — API de histórico/relatórios | Casos de uso e SQL read-only de reporting, filtros, agregações e paginação | Exemplo R$190 confere; joins não duplicam; datas/fuso, anulações, produtos inativos e escopo pessoal estão testados |
| H — Relatório no painel | Filtros de origem/período/profissional/produto, cartões, gráfico e detalhes | Fluxo venda/atendimento → relatório confere; troca de unidade, vazio/erro, filtros e responsividade validados |

Entregas A–F concluídas, migrations até 00019, testes PostgreSQL e Playwright. Ver [ADR 0027](adr/0027-product-stock-sales-foundation.md), [ADR 0028](adr/0028-products-and-transactional-stock.md), [ADR 0029](adr/0029-recorded-sales-and-stock-atomicity.md), [ADR 0030](adr/0030-sale-cancellation-and-scoped-history.md), [ADR 0031](adr/0031-products-stock-and-sales-panel.md) e [ADR 0032](adr/0032-automatic-appointment-completion.md). A primeira parte de G consolidou os contratos Activity*, as regras de valores/contagens e a origem automática, conforme o [ADR 0033](adr/0033-unified-activity-report-contracts.md). Próxima parte: consultas SQL e leitura concreta com testes PostgreSQL; depois casos de uso/HTTP. Não implementar tudo de uma vez; explicar os arquivos antes de prosseguir. Não criar adapters vazios, novo go.mod, pool ou dependências sem uso concreto.

Os arquivos concretos serão adicionados conforme a entrega: catalog/domain/product.go e stock.go; catalog/application/product*.go e stock*.go; sales/domain/sale.go; sales/application/{dto,ports,errors,register_sale,cancel_sale,list_sales}.go; adaptadores HTTP/PostgreSQL; reporting/application para as novas consultas. Testes continuam em cada módulo/tests e integração em tests/integration.

Criar migrations novas, numeradas no momento da implementação. Não modificar migrations aplicadas. Aplicação no Supabase é uma etapa separada após testes locais e autorização vigente; este plano não executa nenhuma migration.

## 10. Verificações obrigatórias por entrega

### Ajuste da entrega F — conclusão automática

Decisão de 2026-10-31: como comparecimento é o caso comum informado pelo usuário,
o atendimento passa a ser concluído automaticamente após o término reservado
(ends_at), quando ainda estiver scheduled/confirmed. Não confundir confirmação
da reserva com conclusão do serviço. Exibir “Concluído automaticamente”.

Ausência e cancelamento são exceções explícitas; owner pode corrigir na unidade,
barber somente em seu escopo. A correção de uma conclusão automática exige motivo,
controle de versão e auditoria. Nenhuma decisão manual pode ser sobrescrita pelo
processamento automático. Cancelamento público mantém suas próprias regras.

O processamento ocorre na API, com estado/instantes persistidos como fonte de
verdade, recuperação após interrupções e idempotência; não depende de tela aberta,
timer do navegador ou escrita durante GET. O marco de ativação está persistido
na migration 00019; o passado anterior a ele não é concluído silenciosamente.
Concorrência entre job e correção manual foi verificada em PostgreSQL real.

Relatórios contam o estado final atual e preservam a origem automática. Ausências
e cancelamentos saem dos totais de realizados, sem apagar snapshots. Como há
presunção de comparecimento, valores podem estar superestimados se exceções não
forem registradas. Conclusão não comprova pagamento recebido. Ver
[ADR 0032](adr/0032-automatic-appointment-completion.md), que sucede esta regra
específica do ADR 0014. Regra implementada na entrega F; executor da API, ciclo de
15 segundos e lote de até 100, com marco de ativação persistido na migration 00019.

- Isolamento: produto/profissional/atendimento/venda/item de outra unidade nunca entra na operação, mesmo com UUID válido.
- Atualização de preço: venda antiga conserva snapshot; revisão desatualizada recebe conflito antes de qualquer baixa.
- Concorrência: última unidade, dois produtos em ordens opostas, venda junto com retirada de estoque, anulação concorrente.
- Idempotência: mesmo corpo/chave, corpo diferente, resposta perdida, acesso revogado e repetição após estoque zerado.
- Rollback: falha depois do primeiro item/movimento não deixa venda ou baixa parcial.
- Banco: saldo negativo recusado; origem/reversão repetida recusadas; tenant incorreto recusado; edição/exclusão de movimentos vedada ao papel da API.
- Reconciliação: saldo atual equivale à soma das movimentações válidas de cada produto.
- Relatório: serviço ligado a venda de vários itens contado uma vez; dois tenants com mesmos nomes separados; períodos no fuso correto; cancelamento/reflexo histórico explícito; operações sem profissional em grupo próprio.
- Frontend: sem retry automático de escrita incerta; token verificado; rascunho preservado; cliques duplicados bloqueados; loading/erro/vazio distintos; celular e teclado.

Comandos de validação serão executados dentro de backend-chat, com banco local descartável:

```powershell
goose -dir db/migrations validate
# DATABASE_URL deve apontar para o banco descartável desta validação.
goose -dir db/migrations postgres "$env:DATABASE_URL" up
sqlc generate
gofmt -w cmd internal tests
go mod tidy
go vet ./...
go test ./...
# Testes de integração exigem as variáveis de bancos descartáveis definidas na entrega.
go test -tags=integration ./tests/integration/...
go build ./cmd/api
go build ./cmd/worker
docker compose --env-file .env.example config --quiet
```

Os testes de concorrência/trigger serão executados em PostgreSQL real; mocks não comprovam integridade de estoque. Verificar configuração dos scripts de frontend no momento da entrega e executar typecheck/build/Playwright apropriados.

## Referências e relação com o plano anterior

- [ADR 0014 — histórico e indicadores](adr/0014-reporting-read-models-and-learning-path.md): preservar snapshots, fuso e distinção entre valor realizado e recebimento; ampliar as projeções para vendas.
- [Trilha de aprendizado](learning-roadmap.md): este plano detalha uma nova frente de produto e altera a sequência, incluindo produtos/estoque/vendas antes do relatório unificado.
- [Transações PostgreSQL](https://www.postgresql.org/docs/current/tutorial-transactions.html): atomicidade das etapas da venda.
- [Locks PostgreSQL](https://www.postgresql.org/docs/current/explicit-locking.html): proteção de linhas e ordem consistente de aquisição de locks.
- [Constraints PostgreSQL](https://www.postgresql.org/docs/current/ddl-constraints.html): CHECK, unicidade e FKs; validação entre linhas exige mecanismo apropriado.

Sem implementação de cortesia, worker de negócio, integração de cobrança dos produtos ou infraestrutura AWS nesta entrega de planejamento.
