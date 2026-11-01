# 0037 — Relatórios no painel administrativo

Data: 2026-10-31. Estado: entrega H implementada no frontend.
Preservar o redesign existente, sem substituir shell, tokens, marca ou agenda.

## Entrada e apresentação

/admin/{slug}/relatorios reutiliza ShopPanel, sessão e papel já verificados.
Relatórios entra depois de Vendas na navegação completa para owner/barber.
Os quatro atalhos existentes do menu mobile permanecem; Relatórios fica em Mais.
Usar grafite/latão, Fraunces/DM Sans e componentes de formulário já existentes.
Não adicionar biblioteca de gráficos, dependências ou alterações no banco.

reports-page.tsx coordena a preparação do período e os filtros. report-results.tsx
coordena resumo/histórico e sua apresentação. activity-chart.tsx renderiza o
gráfico SVG e a tabela alternativa. reports.module.css usa os tokens atuais e
ajusta a disposição, incluindo datas legíveis em telas de 320px e modo escuro.

## Período, filtros e sessão

Uma leitura inicial autorizada do resumo determina o fuso e o relógio do banco,
inclusive para barber, que não possui acesso às configurações do owner. A primeira
consulta usa um mês candidato UTC; seu resultado só é exibido se corresponder
ao mês local do observed_at da unidade. Caso contrário, consultar o período
correto antes de apresentar valores. Não assumir o fuso/relógio do navegador.

Período e origem são os filtros principais, com profissional para owner. Nome,
telefone, serviço e produto ficam em Mais filtros. Opções vêm dos grupos do
histórico autorizado, inclusive recursos hoje inativos; mantê-las ao refinar
filtros dentro da mesma unidade. Não criar cadastros nem buscar catálogo para
reconstruir nomes/preços antigos. O telefone nacional usa BR; para outro país,
solicitar +código. A API normaliza e executa a igualdade sobre E.164.

Situações de atendimento/venda ficam no histórico e não alteram os indicadores.
Essa diferença é explicada junto aos controles. Origem exclusiva limpa o filtro
incompatível. Aplicar novos filtros reinicia a paginação. Limpar filtros retorna
ao período inicial; Atualizar relatório relê resumo e histórico.

Não colocar nome/telefone nos search params da página nem persistir dados do
relatório. Estado local fica vinculado a usuário, unidade, papel e profissional.
Trocas descartam os resultados; consultas anteriores são abortadas. Usar a
requisição autenticada existente com expectedUserID, sem cache, sem gravar tokens
ou recriar clientes de sessão. Revogação detectada na leitura oculta os resultados.

## Valores e histórico

Exibir valor registrado, serviços/produtos separados e contagens distintas de
atendimentos, comprovantes e unidades. Previsão fica fora do valor registrado.
Identificar conclusões automáticas e deixar explícito que valores não provam
recebimento nem lucro. Não somar moedas; cada moeda tem seu bucket, e o gráfico
permite selecionar uma quando necessário.

Todos os indicadores vêm do resumo da API, não da soma das páginas carregadas.
O histórico usa chave composta origem/registro/item, preservando vários produtos
da mesma venda. Mostrar contato atual, nome histórico, profissional, horário no
fuso da unidade e situação. Cancelamento/anulação preserva o valor original,
com contribuição zero ao total. Situações previstas continuam identificadas.
Um atendimento gratuito concluído continua realizado; valor zero não o torna
previsão. Esse rótulo depende somente de scheduled/confirmed.

Cursor é tratado como opaco. Carregar 25 registros por vez, reiniciar após erro
de paginação e distinguir vazio, carregamento e falha. Não apresentar falha como
zero faturado. Resumo e histórico são leituras independentes; cada resposta possui
seu snapshot, e páginas sucessivas podem refletir correções posteriores.

## Contrato e visualização

src/lib/api/reports.ts possui os tipos e validação em runtime para datas/fuso,
origem, medidas inteiras, moedas, IDs, escopo profissional e estados de cada ramo.
Rejeitar totais combinados incoerentes, valores inseguros, registros fora do
período, cursores repetidos e itens duplicados. Preservar frações de segundo na
comparação de instantes: Date.parse sozinho perde microssegundos do PostgreSQL.

O gráfico usa linhas sólida/tracejada, legenda textual, eixo a partir de zero e
tabela acessível com valores exatos. Preencher dias sem registros com zero apenas
após uma resposta válida. Agrupar por dia até 31 dias, blocos de sete dias até
92 e meses nos períodos maiores. Não usar dados simulados no código do produto.
Agrupamentos por profissional/serviço/produto ficam em seção recolhível.

## Validação

Build de produção e verificação TypeScript concluíram. A rodada conjunta de
48 testes Playwright passou; após corrigir a classificação de serviço gratuito,
os 12 testes de relatórios passaram novamente. São 49 cenários distintos,
incluindo as 37 regressões de acesso, navegação e comércio. Cobrem totais
independentes das páginas, itens da mesma venda,
filtros, estados, conta barber, mudanças de unidade/acesso, respostas atrasadas,
precisão temporal, fuso do servidor, períodos longos, erros/vazio e mobile/dark.
Capturas foram inspecionadas, com tabela alternativa e sem overflow da página.

Os testes do navegador usam API e sessão simuladas; os 12 cenários PostgreSQL da
entrega G validaram as consultas/rotas separadamente. Não afirmar que essa rodada
usou a conta Supabase real. Nenhuma migration foi criada ou aplicada nesta etapa.
Não modificar README, doc.go ou arquivos locais de ambiente.

Próxima etapa de produto pode validar o fluxo com dados reais de teste, com o
usuário acompanhando. Worker/WhatsApp, recebimentos, custos/lucro, exportações
e recursos AWS continuam fora desta entrega.
