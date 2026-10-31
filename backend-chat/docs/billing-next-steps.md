# Assinaturas — próximas etapas guiadas

Atualização: 2026-10-30. A trilha distingue etapas entregues e planejadas. Não
implementar todas de uma vez: explicar os arquivos e validar cada entrega antes
de prosseguir.

## Ponto de partida entregue

- Cadastro Supabase + provisionamento local de owner/unidade self_service, com
  endereço obrigatório; configuração de serviços/profissionais sem cobrança.
- Configurações owner via API e frontend, com versão e revisão de conflitos.
- Billing com contas, contratos, cobertura temporal, tentativas e inbox.
- Guard público para unidades self_service; manuais preservam compatibilidade.
- Checkout TEST com parâmetros congelados, recuperação idempotente e revalidação
  de acesso. Configurações já consulta condições, prepara e abre o pagamento.
- Webhook TEST assinado, inbox/reconciliação e períodos pagos foram implementados;
  teste com conta Stripe ainda pendente. Sem scheduler local de retry do inbox.
- Consulta comercial e publicação explícita owner entregues na API e interface;
  sem migration nova nesta integração ou alterações nas unidades manuais.
- Customer Portal TEST entregue nas Configurações do owner: cartão, faturas e
  cancelamento ao fim do período, com política dedicada e isolamento por unidade.
- Criação manual self_service exige cobertura na transação; replay autorizado e
  gestão de reservas existentes continuam disponíveis. Recuperação no formulário
  administrativo integrada, mantendo rascunho e confirmação explícita.

## 1. Webhook e confirmação financeira — backend TEST entregue

Implementação e validações locais concluídas no
[ADR 0019](adr/0019-signed-stripe-webhooks-and-paid-coverage.md). A descrição abaixo
é o roteiro de estudo dessa entrega. Teste integrado à conta Stripe e política
completa de estornos/disputas ainda pendentes antes de produção.

Objetivo: transformar pagamento confirmado em cobertura local, sem confiar no
redirect do browser nem no status active isoladamente.

Arquivos/capacidades esperados:

- billing/infra/stripe: verificar assinatura Stripe-Signature sobre corpo bruto,
  tolerância temporal e ambiente; mapear somente os eventos suportados.
- billing/application: receber evento validado e reconciliar contrato/cobertura.
- billing/infra/postgres: inbox durável com unicidade por event_id; associação
  por conta/Session locais, transações e locks comuns com o Checkout.
- Handler público de webhook, sem Bearer Supabase, mas autenticado pela assinatura
  Stripe. Retornar sucesso só depois da aceitação durável; falha deve permitir
  redelivery. Sem goroutine descartável como garantia de processamento.

Tratar eventos duplicados, fora de ordem e anteriores ao vínculo local. Usar
invoice.paid e linhas/períodos elegíveis para cobertura; reconsultar assinatura
atual quando necessário. Validar Customer, Subscription, Price e ambiente contra
os registros locais; metadados isolados não autorizam acesso. Pagamento falhado,
suspensão, cancelamento e renovação devem produzir decisões explícitas. Não
conceder tolerância ou trial silenciosamente.

Definir o mecanismo executável de processamento/retry do inbox nesta entrega:
nenhum evento pode receber 2xx e ficar eternamente pending. A alternativa inicial
pode processar após persistir, usando redelivery da Stripe em falhas; a evolução
para job durável precisa também descobrir eventos não resolvidos/reinícios.
Evitar chamadas Stripe sob transações longas; leituras reconciliadas devem ter
serialização/fencing para não aplicar um snapshot antigo depois de outro novo.

Aprendizado: assinatura HMAC, autenticação de máquinas, inbox, at-least-once,
projeção local e consistência eventual.

Aceite local validado: pagamento inicial, renovação, duplicata, evento fora de ordem, falha de
commit, queda de processo, pagamento atrasado e cancelamento testados; nenhum
evento de teste pode conceder acesso de produção. Testar com Stripe CLI/testes
oficiais ainda pendente, além dos testes locais simulados. [Referência oficial](https://docs.stripe.com/webhooks).

## 2. Estado comercial e publicação em shops

Backend entregue. Ver [ADR 0020](adr/0020-explicit-shop-publication.md) e
[contrato HTTP](shop-publication-openapi.yaml). Integração visual entregue em Configurações.

Objetivo: ter consulta administrativa de estado comercial e uma ação explícita
de publicação pelo owner, separadas da tela de dados básicos.

- Consultar assinatura/cobertura, publicação e motivo do bloqueio sem dados
  sensíveis ou chamadas Stripe a cada navegação.
- Publicar apenas com cobertura válida e cadastro pronto: endereço, serviço,
  profissional elegível e expediente utilizável. Revalidar em transação.
- Guard público combina unidade ativa, intenção de publicação e cobertura.
- Permitir despublicar sem cancelar assinatura e sem apagar reservas.
- Pagamento não publica automaticamente; depois de confirmado, o dono confirma
  a publicação. Redirect e metadata de Checkout não fazem essa escrita.

Aprendizado: separar permissões administrativas de habilitação comercial e
política de publicação de estado financeiro.

Aceite local validado: loja não paga não expõe catálogo/horários/criação pública; paga mas não
publicada também não. Configurações continuam acessíveis nos dois casos.
Testes PostgreSQL cobrem prontidão, ledger, isolamento, commit e concorrência.
Etapa 3 também concluída no backend; a etapa 4 foi entregue parcialmente abaixo.

## 3. Trava comercial no agendamento manual

Backend entregue. Ver [ADR 0021](adr/0021-staff-booking-commercial-eligibility.md).
Sem migrations novas nesta integração. Interface de recuperação também entregue.

Objetivo: mesma habilitação para novas reservas públicas e administrativas de
unidades self_service, sem bloquear a gestão do que já foi contratado/agendado.

- Booking consulta/revalida cobertura na mesma transação da criação manual.
- Preservar recuperação de uma reserva idempotente já salva, mesmo após bloqueio.
- Não bloquear login, configurações, agenda ou consulta/cancelamento existente.
- Owner pode abrir Checkout; barbeiro sem habilitação recebe orientação para
  chamar o proprietário, não acesso à cobrança ou um checkout próprio.
- Não recriar agendamento automaticamente após o retorno do pagamento.

Aprendizado: invariantes entre módulos, isolamento de tenant e idempotência.

Aceite local validado: criação bloqueada sem cobertura, replay existente recuperado, concorrência
com mudança comercial e ausência de regressão em cancelamentos/agenda.
Owner recebe 402 e barber 403, ambos com shop_subscription_required; falha de
dependência retorna 503. Pagamento habilita criação manual sem exigir publicação.
Integração do formulário manual entregue na etapa 4. Customer Portal entregue abaixo;
próxima implementação: cortesia administrativa, antes dos cupons.

## 4. Interface simples de assinatura e publicação

Entregue em Configurações: estado comercial, publicação/retirada explícitas e
compartilhamento condicionado à API. Também entregue a consulta de oferta mensal
em BRL, revisão das condições, preparação idempotente e abertura do Checkout em
nova aba. Ver [ADR 0022](adr/0022-reviewed-subscription-offer-and-checkout-ui.md).
Conferência de pagamento é manual via GET; não há polling nem repetição de POST.
Formulário manual também integrado: recuperação no mesmo modal, Checkout com
intent=manual_booking em outra aba, consulta de confirmação e revisão explícita
de horário/dados antes de reservar. Barbeiro recebe orientação para o owner.
As observações administrativas usam rotas privadas próprias e funcionam em
unidades ainda não publicadas. Ver [ADR 0025](adr/0025-manual-booking-subscription-recovery.md).
O ciclo de pagamento TEST foi validado com a conta Stripe na etapa anterior;
cenários do formulário foram cobertos localmente com contratos simulados.

Objetivo: completar Configurações sem transformar o painel em gestão financeira.

- Um bloco de assinatura com valor/moeda/recorrência vindos do servidor e estado
  legível. Preço deve ser exibido antes da confirmação de redirecionamento.
- Ações claras: assinar/publicar, conferir pagamento, gerenciar assinatura.
- Tratar checkout=returned/canceled somente como navegação; consultar API para
  decidir estado. Nunca exibir pago por causa do parâmetro da URL.
- Reaproveitar tentativa pendente; bloquear cliques duplicados e explicar
  resultados incertos. Polling limitado, com cancelamento e alternativa manual.
- Link público compartilhável só quando o estado comercial permitir.
- Formulário manual bloqueado oferece assinatura ao owner sem perder rascunho;
  depois do pagamento exige revisão e confirmação do agendamento.

Aprendizado: máquina de estados no frontend e separação de confirmação e intenção.

Aceite: testes de navegador com retorno antes/depois do webhook, pagamento
recusado, timeout, troca de unidade/conta e acesso de barbeiro; celular/teclado.

## 5. Customer Portal — entregue em TEST

Disponível em Configurações > Assinatura e agendamento online > Gerenciar assinatura.
Ver [ADR 0026](adr/0026-owner-stripe-customer-portal.md). O endpoint recebe somente
{}, revalida owner antes/depois da rede e exige contrato local conciliado. Sem
migration nova. Unidades antigas/manuais não são matriculadas automaticamente.

STRIPE_PORTAL_CONFIGURATION_ID seleciona a política TEST dedicada. Para provisionar
em outra área TEST, executar `go run ./cmd/configure-stripe-portal` na pasta do backend
e salvar o ID retornado no .env. Com ID já configurado, o comando apenas valida a
política existente. Nunca substituir secret key/webhook secret por esse ID.

Configuração e sessão foram validadas na Stripe TEST com contrato existente, sem
alterar cartão, cancelar ou cobrar essa assinatura. A janela open_until é local
para abrir o link; não representa o prazo oficial da sessão Stripe. Link somente
em memória. Conferir situação é GET; retornos não modificam estado financeiro.

Objetivo: delegar cartão, faturas e cancelamento ao portal hospedado da Stripe.

- Endpoint owner resolve exclusivamente o Customer da unidade autorizada.
- URL de retorno fixa no servidor; configuração do portal no ambiente correto.
- Cancelamento no fim do período preserva cobertura paga até o prazo confirmado;
  alterações feitas no portal chegam pelo mesmo webhook/reconciliador.
- Nunca expor secret key nem aceitar Customer ID arbitrário do frontend.

Aceite: isolamento entre unidades, mudança de cartão, cancelamento programado,
retomada e falhas sem promessa falsa de atualização imediata.
[Referência oficial](https://docs.stripe.com/customer-management).

### Próximas entregas, ainda não implementadas

1. Cortesia administrativa: concessão explícita por unidade, motivo, prazo,
   auditoria e revogação pelo operador. Compartilhar a decisão de habilitação
   com os guards atuais, mantendo a publicação separada. Não simular fatura paga
   nem assinatura Stripe. Testar término, isolamento e convivência com contrato pago.
2. Cupons Stripe: começar por desconto parcial/temporário e definir a regra para
   ciclos de valor zero antes de habilitar descontos de 100%. A política atual
   exige fatura positiva; liberar cupom no Checkout sem ajustar/testar a cobertura
   pode cobrar corretamente e ainda bloquear o aplicativo. Só depois habilitar
   o campo na interface e tratar renovação/cancelamento/expiração do desconto.
3. Concluir homologação do ciclo financeiro e preparar AWS conforme etapa 6,
   incluindo testes controlados de mudança de cartão e cancelamento/retomada.

## 6. Preparação para produção e AWS

Somente após fechar o ciclo TEST completo:

- Decidir preço comercial, suporte e tratamento de cobrança/tributação com os
  responsáveis adequados; não presumir que emitir cobrança cobre obrigação fiscal.
- Separar dados/segredos/webhooks test e live. Hoje o código rejeita sk_live_;
  habilitar produção exige mudança explícita e política de migração, não só trocar
  uma variável. Rever unidades manuais com autorização do operador.
- Resolver tentativas review e eventos não processados com ferramenta auditável;
  nunca remover registros para liberar cobrança por tentativa.
- HTTPS, IAM mínimo, segredos na plataforma, backups/restauração, observabilidade,
  alertas do inbox e reconciliação periódica. Limitar abuso de Checkout/webhook.
- Implantar primeiro ambiente de homologação AWS; migrations como etapa explícita
  do deploy, não no bootstrap. Definir custo e rollback antes de produção.

Aceite: ciclo real controlado somente após autorização do proprietário do SaaS,
sem uso de cartão real neste desenvolvimento inicial.

## Como estudar cada entrega

1. Ler o contrato de entrada/saída e escrever os cenários de falha esperados.
2. Entender a regra no domínio e a coordenação no caso de uso.
3. Implementar adapter concreto e schema apenas quando necessários.
4. Testar regra pura, fronteira HTTP/provedor e transações no PostgreSQL real.
5. Explicar o caminho da requisição e como recuperar uma falha antes de avançar.

Operação atual: aplicar migrations com Goose e reiniciar API. Não é necessário
rodar o worker para o Checkout entregue. Para habilitar testes manuais futuros,
configurar STRIPE_SECRET_KEY=sk_test_..., STRIPE_MONTHLY_PRICE_ID=price_... e
STRIPE_WEBHOOK_SECRET=whsec_... no backend, além de FRONTEND_ORIGIN. Nunca enviar
segredos no chat ou NEXT_PUBLIC_. A integração de oferta/Checkout na interface
não adiciona migration: mantenha o banco na versão 14 já entregue.

### Teste integrado pendente, sem cobrança real

O botão está em Configurações > Assinatura e agendamento online > Ver plano e assinar.
Instalar a Stripe CLI pela
[instrução oficial](https://docs.stripe.com/cli), autenticar na mesma conta/test
sandbox das chaves e usar somente cartão de teste. Não foi instalada nem logada
automaticamente nesta entrega.

```powershell
stripe login
stripe listen --latest --all-snapshot --events-from "@self" --forward-to http://127.0.0.1:8080/api/v1/webhooks/stripe
```

Conferir a versão dos eventos: o adapter aceita explicitamente envelopes
2026-09-30.endive e 2026-10-28.endive. Nesta CLI, --latest continuou informando a
primeira; esse parâmetro não deve ser tratado como garantia de versão exata.
Leituras dos recursos financeiros continuam fixadas em 2026-10-28.endive.
Para endpoint cadastrado no Dashboard, escolher uma das versões suportadas. Não usar
--live, thin events ou Connect nesta implementação.

`localhost:4242/webhook` é apenas um endereço de exemplo da Stripe; não é a
rota deste projeto. O backend mantém a assinatura obrigatória e não aceita
versões desconhecidas. As aspas duplas em "@self" funcionam em cmd e PowerShell.

Na API atual, o filtro de cartão é `allowed_payment_method_types[0]=card`.
O antigo `payment_method_types` foi removido e causava 400 na Stripe, traduzido
em 503 no painel. Esse erro pode ficar associado à chave original. O adaptador
primeiro recupera o pedido original: só um 400 invalid_request_error ou
parameter_unknown referente exatamente a payment_method_types permite a forma
corrigida com sufixo determinístico `:card-filter-v2` da MESMA tentativa. O UUID,
Customer, Price, retorno e associação permanecem congelados. Uma resposta perdida
da forma corrigida reutiliza esse mesmo sufixo; nunca cria outra tentativa local.
Sucesso antigo é recuperado sem mudar o formato; Session vinculada continua GET-only.
Timeout, 5xx, erro em outro parâmetro, resposta malformada ou conflito de idempotência
nunca autorizam a alternativa. Não apagar registros nem trocar chaves manualmente.
Ver [criação de Session](https://docs.stripe.com/api/checkout/sessions/create) e
[idempotência](https://docs.stripe.com/api/idempotent_requests).

Logs do backend agora incluem status/código/parâmetro permitidos e request ID da
Stripe para localizar a falha no Workbench, sem mensagens brutas ou credenciais.

Copiar o whsec_ exibido pela CLI para STRIPE_WEBHOOK_SECRET no .env do backend,
sem compartilhá-lo. Manter STRIPE_SECRET_KEY de teste, STRIPE_MONTHLY_PRICE_ID
mensal BRL e FRONTEND_ORIGIN. Reiniciar a API e manter o listener aberto.

Com uma unidade self_service e seu owner autenticado, revisar o preço na tela,
preparar o Checkout e abrir a URL para concluir com dados de teste da Stripe.
Para cliente HTTP, consultar billing/offer antes e enviar intent + offer_id para
billing/checkout. Um `stripe trigger invoice.paid` isolado costuma criar outro Customer:
isso não substitui o fluxo com os vínculos locais e não deve liberar a unidade.

Conferir no banco, com acesso servidor, state=processed no inbox, contrato local,
fatura e cobertura. Consultar GET /api/v1/admin/shops/{slug}/commercial-status
com o Bearer do proprietário; corrigir blockers e publicar via PUT
/api/v1/admin/shops/{slug}/publication com {"requested":true}. Não alterar
publication_requested manualmente nem tratar o retorno do Checkout como pagamento.

Se um evento de um pagamento já realizado foi recusado antes da correção, o
operador pode recuperá-lo sem gerar outro pagamento:

```powershell
cd C:\Users\felip\Barber-chat\backend-chat
. .\scripts\Load-Env.ps1
go run ./cmd/reconcile-stripe-event evt_ID_DO_EVENTO_EXISTENTE
```

Esse comando usa a chave TEST do backend para consultar o evento original na
Stripe e passa a projeção validada ao reconciliador/inbox existentes. Não assina
um payload local, não aceita JSON arbitrário e não cria outra assinatura.
O resultado operation_allowed informa a cobertura atual; publicação continua
sendo confirmada pelo owner no painel. Ver [ADR 0024](adr/0024-stripe-envelope-version-and-operator-replay.md).

```sql
-- Diagnostic query: identifiers/state only, no contacts, payload or secrets.
SELECT stripe_event_id, event_type, state, attempts, failure_code, received_at
FROM billing_webhook_events
WHERE receipt_protocol = 1 AND state = 'pending'
ORDER BY received_at;
```

Enquanto não existir job local, tratar 503 via redelivery/reenvio do evento pela
Stripe. A janela automática do provedor é finita: acompanhar pending e usar o
reenvio explícito após corrigir dependências. Nunca excluir o inbox/contrato nem
abrir outra assinatura para tentar resolver uma pendência. Para reenvio a um
endpoint registrado, consultar a opção de resend no Dashboard/Stripe CLI; para
listener local, consultar as opções suportadas pela versão instalada.
