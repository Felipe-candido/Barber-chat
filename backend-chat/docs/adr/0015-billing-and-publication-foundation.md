# ADR 0015 — Configuração gratuita e operação por assinatura

Status: fundação implementada; não conectada às rotas. Data: 2026-10-29.

Atualização posterior: o cadastro self_service e o guard público foram conectados
nas entregas 0016/0017. O [ADR 0018](0018-durable-test-stripe-checkout.md) acrescenta
Checkout TEST no backend. Os limites abaixo descrevem a fundação original, não
substituem esses incrementos. Webhook, portal, publicação e trava comercial da
criação manual permanecem planejados.

## Decisão

Manter o monólito Go e adicionar `billing`. Cadastro local/permissões continuam
em identity; publicação/configuração em shops; reservas em booking. Não alterar
README ou doc.go nesta entrega. Não implementar checkout, signup ou frontend ainda.

O proprietário poderá configurar serviços, profissionais e expediente sem pagar.
A assinatura mensal por unidade habilitará novas reservas públicas/manuais e a
elegibilidade de publicação. Pagamento não publica automaticamente. Login,
configuração, cobrança e tratamento de reservas existentes não dependem dessa
habilitação. Papéis owner/barber continuam independentes de assinatura.

Um Customer Stripe pertence a uma unidade neste MVP: portal compartilhado entre
unidades poderia expor dados de cobrança a proprietários de outra unidade.
IDs externos ficam na persistência; domínio/application não importam SDK Stripe.

## Acesso e persistência

O acesso exige cobertura confirmada `[paid_from, paid_until)`, não somente status
ativo ou próximo período de faturamento. Sem trial nesta etapa. Tolerância só
existe com `grace_until` explícito em assinatura vencida anteriormente paga; não
há prazo comercial presumido. Suspensão/cancelamento efetivos negam operação;
cancelamento programado não antecipa a expiração da cobertura paga.

A migration 00009 adiciona intenção de publicação falsa por padrão, contas de
cobrança, histórico de assinaturas, tentativas de checkout e inbox de webhooks.
Índices únicos limitam contrato corrente e checkout pendente por unidade. FKs
compostas impedem assinatura ligada ao Customer de outra unidade. RLS/revogações
fecham acesso direto do browser. Down recusa descartar dados comerciais.

O inbox aceita eventos ainda sem unidade resolvida; isso não autoriza operações.
Recebimento futuro exige verificação de assinatura, ambiente correto, retenção
de payload, deduplicação e reconciliação sem presumir ordem de eventos. Checkout
exige idempotência externa; índices locais não impedem sozinhos dupla cobrança.

## Limites desta entrega

GetShopEntitlements lê o banco por porta pequena e devolve decisão com relógio
injetado; ainda não está no bootstrap/middleware nem protege booking/publicação.
Publicação também exigirá prontidão da configuração no futuro caso de uso.
Nenhuma unidade existente é ativada, cobrada ou bloqueada por esta fundação.
O rollout das regras exigirá decisão explícita para as unidades existentes.

Referências: [ciclo de assinaturas Stripe](https://docs.stripe.com/billing/subscriptions/overview)
e [constraints PostgreSQL](https://www.postgresql.org/docs/18/ddl-constraints.html).
