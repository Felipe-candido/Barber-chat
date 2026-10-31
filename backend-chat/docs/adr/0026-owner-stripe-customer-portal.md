# 0026 — Portal Stripe por unidade, exclusivo do proprietário

Data: 2026-10-30. Estado: implementado em TEST.

## Fronteira e autorização

O portal pertence a billing. Identity verifica o proprietário e shops projeta a
unidade na mesma transação já utilizada pelo Checkout. O novo caso de uso reutiliza
esse contexto transacional; não cria módulo, pool ou serviço separado.

POST /api/v1/admin/shops/{slug}/billing/portal aceita somente {}. O tenant vem do
middleware autenticado. Revalidamos owner ativo e unidade self_service, e buscamos
o Customer TEST associado a um contrato conciliado com tentativa do protocolo
atual. Checkout abandonado não basta. Contratos vencidos/cancelados podem acessar
faturas e regularização, independentemente da cobertura ou publicação.

A chamada Stripe acontece fora dos locks SQL. Antes de devolver a URL, conferimos
novamente owner, Customer e slug. Alteração de acesso durante a rede não revela
a credencial. O endpoint não cria Customer, contrato, tentativa ou cobertura.

## Política Stripe explícita

STRIPE_PORTAL_CONFIGURATION_ID aponta uma configuração dedicada, sem usar o
default da conta. O adapter consulta e valida sua política antes de cada sessão:
TEST, ativa, metadado barber_portal_protocol=1, sem Connect/login público, cartão
e faturas habilitados, cancelamento at_period_end sem prorrata, sem troca de plano
ou retenção por cupom. Também valida que o Customer TEST pertence à unidade.

cmd/configure-stripe-portal provisiona a política TEST com chave de idempotência
própria; se o ID já estiver configurado, apenas valida o registro existente. Não
altera o default nem assinaturas. Na API 2026-10-28.endive, a criação exige pelo
menos duas opções de motivo de cancelamento, mesmo com a coleta desabilitada.

O adapter fixa API/origem, pt-BR e retorno da unidade. Confere Customer/configuração,
ambiente, retorno, ausência de deep link/Connect e host HTTPS billing.stripe.com.
A URL é uma credencial: sem log, persistência, cache ou dados de contato. Cada
clique explícito cria uma sessão nova, sem retry automático; isso não gera cobrança.

## Interface e estado financeiro

Gerenciar assinatura aparece nas Configurações do owner para contrato existente,
inclusive cancelado. Abre em outra aba, preservando rascunho. O frontend valida
tenant, TEST e URL; restringe a abertura a quatro minutos desde a criação, com
margem de 30 segundos. open_until é uma janela local, não o TTL declarado pela
Stripe. Fechar/trocar unidade elimina a URL da memória; resposta tardia é descartada.

Conferir situação somente consulta commercial-status. O retorno ?portal=returned
não modifica assinatura/publicação. Webhooks existentes reconciliam a mudança:
cancelamento programado mantém cobertura confirmada até seu limite; cancelamento
efetivo/ausência de cobertura impede novas reservas conforme os guards existentes.
Atualizar cartão não equivale a quitar uma fatura. Publicação continua independente.

## Verificação e limites

Testes de contrato Stripe rejeitam política incompatível, Customer de outro tenant,
sessão LIVE, destino inválido e Connect/deep link. PostgreSQL verifica owner,
barbeiro/outsider, corpo estrito, CORS, revogação durante rede e ausência de novas
assinaturas/publicação. Navegador cobre rascunho, retorno, erros, expiração e mobile.
Validação na Stripe TEST criou política e sessão de um contrato existente; não
executou troca de cartão ou cancelamento real desse contrato. Sem migration nova.

Referências: [sessões](https://docs.stripe.com/api/customer_portal/sessions/create),
[configuração](https://docs.stripe.com/api/customer_portal/configurations/create),
[opções do portal](https://docs.stripe.com/customer-management/configure-portal).
