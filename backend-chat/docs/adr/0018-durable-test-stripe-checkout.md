# ADR 0018 — Checkout Stripe durável, primeiro em modo de teste

Status: backend de Checkout implementado; webhook, frontend e produção pendentes.
Data: 2026-10-29.

Atualização: o [ADR 0019](0019-signed-stripe-webhooks-and-paid-coverage.md)
implementa o webhook TEST e a reconciliação financeira. As seções abaixo
registram o limite original da entrega de Checkout.

## Limite desta entrega

POST /api/v1/admin/shops/{slug}/billing/checkout, owner ativo de unidade
self_service. Corpo contém somente intent=publish|manual_booking. Configurar a
unidade continua gratuito. Checkout não publica, não cria reserva e não grava
cobertura paga, mesmo quando a Stripe retorna Session complete.

Aceitar apenas sk_test_ e objetos livemode=false. STRIPE_MONTHLY_PRICE_ID aponta
para preço ativo, mensal em BRL, fixo, positivo, por unidade e sem trial. Não
escolher valor comercial pelo código nem aceitar price/customer/tenant/URLs do
browser. Sem credenciais, responder billing_not_configured sem quebrar login.

Unidades manuais continuam fora do rollout: não abrir cobranças acidentalmente.
Customer é da unidade, não do usuário; não compartilhar portal entre tenants.

## Consistência sem transação distribuída

1. Transação curta revalida e bloqueia usuário/membership e unidade ativa;
   reserva e bloqueia billing_account. Contrato corrente impede novo Checkout.
2. Reutiliza a tentativa pendente ou grava uma nova, com UUID, preço e retorno
   congelados. Um índice único já impede duas tentativas correntes por unidade.
3. Fora de qualquer transação, valida preço e cria Customer com chave estável
   por unidade; cria Session com chave estável por tentativa. Só metadados de
   associação, não dados mutáveis do cadastro, entram nessas criações.
4. Persiste cada vínculo confirmado em transações curtas e revalida owner. Não
   retornar URL antes do commit. Falha de banco/rede não implica rejeição externa.
5. Recuperação explícita reutiliza os parâmetros congelados. Session conhecida
   é consultada, nunca substituída porque houve timeout ou passou o relógio local.

O contrato de idempotência Stripe pode expirar após 24h. Sem vínculo externo
confirmado, esta implementação deixa de criar/repetir após 20h e exige
reconciliação. Não supor que expirar uma chave significa ausência de assinatura.
Session complete fica em review aguardando a próxima etapa; não abre outro
Checkout. Só status expired confirmado na Stripe libera outra tentativa em um
novo pedido explícito. Retorno/cancelamento de navegação não prova expiração.

A migration 00013 acrescenta modo da conta e snapshot da requisição. Registros
anteriores são unknown/protocol 0, sem inventar ambiente ou parâmetros: exigem
revisão antes de alcançar o provedor. RLS/grants fechados da migration 00009
continuam válidos. Down recusa remover registros do protocolo novo.

## Adaptadores e validação

Domain contém intenção/janela de recuperação. Application define portas pequenas
para autorização, unidade, persistência e provedor. Identity/shops fornecem suas
projeções por adapters compostos no bootstrap. Application não importa outro
módulo nem SDK/driver. Chamadas externas nunca seguram locks SQL.

O adapter Stripe usa net/http para quatro operações concretas: consultar Price,
criar Customer, criar/consultar Session. Origem fixa api.stripe.com, versão
2026-10-28.endive, timeout e limite de resposta; sem redirecionamentos ou retries
automáticos. IDs, metadados, modo e URL checkout.stripe.com são conferidos antes
de retornar a projeção. Não registrar segredo, URL de Checkout ou erro bruto.
Não introduzir SDK completo apenas para essas operações; reavaliar ao crescer.

Testes usam provider/transport simulados, mas concorrência, transações,
autorização, isolamento e guard de rollback são comprovados no PostgreSQL real.
Não houve compra nem teste com credenciais reais Stripe nesta entrega.

## Próxima etapa, não implementada

Webhook assinado e inbox/reconciliação de faturas e assinatura antes de liberar
operação. O rollout live exige decisão e migração explícitas de ambiente; jamais
aproveitar cobertura ou IDs do sandbox para liberar produção. Ver
[plano de execução](../billing-next-steps.md).

Referências: [idempotência Stripe](https://docs.stripe.com/api/idempotent_requests),
[Checkout Sessions](https://docs.stripe.com/api/checkout/sessions/create),
[consulta de preço](https://docs.stripe.com/api/prices/retrieve) e
[webhooks](https://docs.stripe.com/webhooks).
