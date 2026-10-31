# 0031 — Produtos, estoque e vendas no painel

Data: 2026-10-31. Estado: interface da entrega E implementada e validada no navegador.

## Experiência e limites

Adicionar Produtos e Vendas à navegação existente de owner/barber. Preservar
DM Sans, tokens de cor/espaçamento, componentes de formulário e diálogos do painel.
Produtos usa lista com nome, preço, saldo e indicador de estoque baixo; as ações
de gestão são owner-only. Vendas separa seleção, revisão e confirmação, com um
comprovante persistido ao final. Sem gráficos ou totais gerenciais antecipados.

Owner cria/edita/desativa produto, altera mínimo, registra entrada/ajuste e lê
movimentações. Barber consulta apenas produtos ativos e registra vendas atribuídas
ao seu profissional pelo servidor. A interface não substitui autorização da API.
Histórico de vendas usa datas civis inclusivas, situação, filtro de profissional
para owner e paginação do servidor. Não calcular receita a partir de página parcial.

O formulário de venda é o mesmo em Vendas e nos detalhes da Agenda. Ao abrir pela
agenda, fixar appointment_id e professional_id; não enviar nome/telefone do cliente
no pedido de venda. Registro de produtos não conclui atendimento. Reservas
cancelled/no_show não oferecem esse atalho. Venda avulsa também permanece disponível.

Anulação de venda exige owner, consulta atual, motivo e escolha sem padrão entre
devolver ou manter a retirada do estoque. Mostrar auditoria no comprovante e
preservar itens/valores de vendas anuladas. Não oferecer edição de itens persistidos.

## Comunicação com a API

products.ts e sales.ts tipam pedidos e validam respostas em runtime, incluindo
tenant, IDs, inteiros seguros, estados, preços, totais e auditoria. Registro de
venda confere os itens da resposta com a revisão enviada antes de confirmar sucesso.
Reutilizar requestJSON, sessão Supabase e expectedUserID; não armazenar outro token.
GET não recebe retries de escrita. Paginação é explícita e requisições anteriores
são abortadas quando filtros/unidade mudam, impedindo mistura de páginas.

Valores digitados em reais viram centavos por concatenação decimal validada,
sem multiplicar ponto flutuante. Quantidades são inteiras. O saldo exibido é
informativo; o banco continua sendo a decisão final, inclusive na última unidade.
Conflito de preço/estoque exige seleção/revisão explícita de dados atuais.

## Comandos e resposta perdida

use-commerce-command guarda em sessionStorage uma tentativa versão 1 com chave
aleatória de 32 bytes e corpo copiado antes de enviar POST. Namespace contém
shop_id, actor_id e operação/recurso. Cadastro de produto, movimentação, registro
de venda e anulação têm tentativas distintas. Registro de venda usa o mesmo
namespace na Agenda e em Vendas, evitando esquecer um pedido incerto ao trocar
de entrada. Não armazenar contato do cliente ou credenciais na tentativa.

503, erro de rede e resposta incompatível não comprovam rollback: bloquear edição
do pedido e permitir apenas recuperação explícita com chave/corpo originais ou
conferência do histórico. Reabrir o formulário após reload recupera o pedido;
nenhum POST é reenviado automaticamente. Fechar a janela não apaga a tentativa.
Dados inválidos no armazenamento não são enviados. Encerramento manual exige
conferência explícita e explica que não desfaz algo já salvo.

Uma rejeição definitiva antes de haver incerteza permite revisar os campos com
nova chave. Após resultado já incerto, manter a tentativa até sucesso ou conferência,
inclusive se a recuperação for negada por acesso/assinatura. Sucesso só libera
novo pedido depois de validar o resultado e limpar a tentativa.

PUT de cadastro/mínimo usa a versão consultada. Conflito ou resposta incerta não
gera repetição automática; usuário confere a lista antes de outra edição. Essas
operações não aplicam delta de estoque. Configuração e consulta de histórico não
dependem de cobertura comercial. Se nova venda recebe shop_subscription_required,
preservar revisão: owner abre Configurações em outra aba; barber procura o dono.

## Organização e validação

src/components/commerce agrupa páginas, formulários, comprovante, detalhes e estilos.
Hooks compartilham coordenação de comandos e leitura paginada; regras de autorização,
preço, estoque e idempotência persistente continuam no Go/PostgreSQL.
App Router recebe páginas concretas em /admin/[slug]/produtos e /vendas, usando
o layout e o contexto de identidade/unidade já existentes.

Testes Playwright usam API/Supabase fictícios e verificam payloads, headers,
recuperação após reload, revisão de preço, falta de estoque, perfis, anulação,
paginação, vínculo com agenda, teclado e layouts de celular/desktop. Isso valida
a integração do frontend com o contrato; não substitui os testes reais PostgreSQL
das entregas B–D nem constitui um teste com pagamento/Supabase reais.

Validação concluída: TypeScript, build de produção isolado, formatação e suíte
completa com 165 testes aprovados. Após a revisão visual e o ajuste de recuperação/
consulta dos detalhes, os 19 cenários de produtos/vendas foram recompilados e
aprovados novamente. Capturas de celular/desktop foram conferidas visualmente.

Sem nova dependência, migration, aplicação no Supabase ou alteração de README/doc.go.
Próxima entrega: F, desenhar com o usuário conclusão/ausência e correção de enganos
dos atendimentos antes de implementar seus endpoints e relatórios.
