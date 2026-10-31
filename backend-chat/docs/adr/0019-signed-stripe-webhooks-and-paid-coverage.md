# ADR 0019 — Webhook assinado, reconciliação e cobertura confirmada

Status: backend TEST implementado e validado com provedor simulado/PostgreSQL
real; teste integrado à conta Stripe ainda pendente. Data: 2026-10-29.

## Fronteira e autenticação

POST /api/v1/webhooks/stripe não usa Bearer Supabase: autentica a Stripe com
HMAC-SHA256 de `timestamp.corpo_original`, Stripe-Signature e segredo próprio
whsec_. Verifica timestamp de entrega com tolerância de cinco minutos, múltiplos
v1 para rotação, comparação constante, limite de corpo de 512 KiB e envelope
snapshot da versão 2026-10-28.endive. Só livemode=false, sem Connect. Não usar o
timestamp de criação do evento como prazo de entrega; redelivery pode ser antiga.

STRIPE_WEBHOOK_SECRET é opcional e exclusivo do servidor. Sem configuração,
endpoint retorna 503, sem quebrar login/agenda. Versão/ambiente incompatíveis ou
assinatura inválida retornam 400. CORS de browser não autentica o webhook.

## Recebimento durável e processamento

O inbox possui chave única por event_id. Guarda somente ID/tipo e referências
validadas, não o corpo bruto com contato/endereço/cartão. Colunas da migration
00014 identificam protocolo, recurso, versão e criação. Protocolos legados não
são reinterpretados automaticamente. Uma colisão não sobrescreve o registro.

Processamento síncrono nesta etapa: persistir recebimento, resolver o recurso
na Stripe, reivindicar revisão local, consultar estado atual fora de transação
e aplicar projeção/faturas/completion do inbox atomicamente. Retornar 2xx somente
para evento processado/ignorado confirmado por commit. Falhas de rede, banco,
commit, vínculo ou fencing retornam 503 e deixam pending para redelivery. Não
há goroutine solta nem promessa de que persistir pending sozinho executará algo.

A retomada vem de nova entrega da Stripe, inclusive após reiniciar a API. A
janela/retries do provedor são finitos: depois disso exige reenvio explícito ou
reconciliação operacional. Ainda não há scheduler local do inbox. Pendências
devem ser monitoradas antes do lançamento em produção; não deixar pending
silencioso nem excluir registros para liberar cobrança. Um futuro job durável
poderá usar o mesmo caso de uso, com autorização e política de reprocessamento.

## Concorrência e associação segura

Resolver Customer pelo recurso consultado, encontrar conta local de teste e
validar tentativa/contrato conhecidos. Metadados apenas localizam candidatos;
não concedem acesso. Validar também Session da Stripe, Customer, Subscription,
tentativa, unidade e preço congelado. Se o webhook chegar antes de a API gravar
a Session, consultá-la pela assinatura e confrontar a tentativa já persistida.
Checkout cria e vincula Customer antes de criar Session, permitindo essa ordem.

Conta/unidade serializa escritas locais. Cada reconciliação recebe uma revisão
crescente; aplicar exige a mesma revisão ainda atual. Nenhuma chamada Stripe
segura locks SQL. Se outro processamento começou depois, o snapshot antigo
não pode sobrescrever a projeção e precisa de redelivery. Eventos são gatilhos
para leitura atual, não comandos ordenados pela data do evento. Duplicata já
terminal dispensa novas consultas Stripe.

## Cobertura e estados

Registrar uma fatura por invoice_id, vinculada por FK composta ao contrato da
mesma unidade. Cobertura vem da linha de assinatura, não de current_period_end
ou status active. Elegibilidade atual exige fatura paid, mensal BRL do preço
esperado, quantidade um, sem proration/crédito, cobrança automática, total pago
positivo e integral, e InvoicePayments paid sustentados por PaymentIntent. Não
conceder acesso por marcação manual/out-of-band, trial ou tolerância implícita.
Listas truncadas e formatos ambíguos não viram cobertura presumida.

Cada período é [início,fim). Na leitura de habilitação, usar os intervalos de
faturas elegíveis: unir sobreposições/adjacências sem preencher lacunas não
pagas. PaidFrom/PaidUntil continuam uma projeção compatível, não autoridade para
contratos reconciliados. Protocolo 0 preserva a avaliação legada, sem inventar
faturas históricas. Um único SELECT fornece snapshot coerente ao guard público.

Renovação paga acrescenta cobertura; falha não estende o prazo anterior.
Cancelamento programado preserva o período já pago; cancelamento efetivo nega
operação. Pausa/estado desconhecido/plano incompatível ficam suspensos, sem
acesso. Atualização de fatura inelegível invalida sua cobertura registrada.
Estornos de Charge/disputas sem alteração da fatura não têm política automática
nesta entrega; são requisito antes de live, que continua bloqueado no código.

Não publica barbearia, não altera memberships/roles e não cria reserva.
Configuração, consulta e cancelamento de reservas existentes continuam separados
de cobrança. Status comercial/ação de publicação e trava manual são próximas
entregas, não implementadas por este webhook.

## Verificação

Testes unitários: assinatura, replay temporal, corpo alterado, versão/ambiente,
objetos Stripe e intervalos com lacuna. Integração em PostgreSQL descartável:
concorrência, deduplicação, isolamento, estado ativo não pago, renovação/falha,
cancelamento/suspensão, fencing, vínculo antecipado, falha real no commit com
trigger deferida, rollback e redelivery após reiniciar o caso de uso. Migrations
Up/Down/Up; Down recusa remover dados reconciliados. RLS e revogação de grants
impedem acesso direto do browser ao ledger novo.

Fontes: [assinatura e entrega](https://docs.stripe.com/webhooks),
[ciclo de assinaturas](https://docs.stripe.com/billing/subscriptions/webhooks),
[período da linha](https://docs.stripe.com/api/invoice-line-item/object) e
[InvoicePayments](https://docs.stripe.com/api/invoice-payment/list).
