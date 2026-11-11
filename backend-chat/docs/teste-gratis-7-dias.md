# Tarefa: teste grátis de 7 dias e mensalidade de R$ 49,90

Pedido do dono do produto (10/11/2026). Escrito para o Codex implementar no backend.

## O que o produto precisa

- Mensalidade de **R$ 49,90** (4990 centavos, BRL, mensal).
- **7 dias grátis** ao assinar: a barbearia publica o link e recebe agendamentos
  durante o teste; a primeira cobrança acontece no 8º dia.
- Cancelar durante o teste não gera cobrança.
- Um teste por barbearia (e por cliente Stripe). Recriar conta não pode renovar o teste.

## Onde o código bloqueia hoje (regras deliberadas, mudar com ADR)

1. `infra/stripe/checkout.go` (`ReadCheckoutPrice`): recusa price com
   `recurring.trial_period_days`. Manter assim: o teste não deve vir do price,
   e sim do Checkout Session.
2. `CreateCheckoutSession`: não envia `subscription_data[trial_period_days]`.
3. `application/start_checkout.go:137`: exige `amount_total == offer.AmountCents`.
   Com teste, o Checkout Session aberto tem `amount_total = 0`.
4. `CheckoutOffer.ConfirmationID` e `CheckoutAttempt` (parâmetros congelados,
   protocolo 1): o teste precisa entrar no que é congelado e confirmado,
   provavelmente como protocolo 2, sem reinterpretar tentativas antigas.
5. `infra/stripe/reconciliation.go`: status `trialing` vira `SubscriptionSuspended`
   (teste "trial never grants" em `tests/infra/stripe/reconciliation_test.go`).
6. `domain/subscription.go`: não existe status de teste; `Active` exige cobertura
   paga confirmada (`PaidFrom/PaidUntil`), e a fatura do teste é R$ 0.

## Proposta (a decidir no ADR)

- Checkout com `subscription_data[trial_period_days]=7` e
  `payment_method_collection=always` (cartão na entrada, reduz abuso).
- `subscription_data[trial_settings][end_behavior][missing_payment_method]=cancel`.
- Novo estado local `trialing` com cobertura explícita `[trial_start, trial_end)`
  vinda da subscription do Stripe, que libera agendamento como `active`, sem
  contar fatura de R$ 0 como pagamento.
- Ao fim do teste, só a fatura paga confirmada estende a cobertura (regra atual).
- Elegibilidade: teste só se a barbearia nunca teve subscription/attempt pago;
  registrar isso no banco para não depender só do Stripe.
- Oferta mostrada antes do checkout: "7 dias grátis, depois R$ 49,90/mês"; o
  `OfferID` deve mudar se `trial_days` mudar.
- Testes: checkout com teste, webhook `customer.subscription.created` em
  `trialing`, `trial_will_end`, primeira fatura paga, cancelamento no teste,
  falha de pagamento no 8º dia, segunda tentativa de teste negada, reconciliação.

## Configuração (fora do código)

- No Stripe (modo teste), criar o produto "Na Régua" com price mensal de
  R$ 49,90 BRL **sem** trial no price; usar o ID em `STRIPE_MONTHLY_PRICE_ID`.
- Frontend: quando o teste estiver no ar, definir `NEXT_PUBLIC_TRIAL_DAYS=7`
  na Netlify. Até lá a landing promete só "configure grátis" e mostra R$ 49,90.
