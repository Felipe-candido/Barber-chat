# ADR 0021 — Habilitação comercial da reserva administrativa

Status: backend implementado e validado localmente. Data: 2026-10-30.

## Política e fronteiras

Novas reservas administrativas self_service exigem cobertura confirmada agora.
PublicationRequested é independente: o dono pode atender pelo painel e manter
o link privado. Não exigir cobertura na data futura do atendimento. Não bloquear
login, configurações, agenda ou gestão de reservas existentes por perda de acesso
comercial. Unidades manuais preservam o comportamento anterior.

Booking consome StaffBookingEligibility, uma porta pequena definida em seu
application/ports.go. Billing/infra/postgres implementa BookingEligibility usando
GetShopEntitlements e EvaluateEntitlement existentes; não duplicar política de
faturas, grace ou cancelamento. Cmd/api monta o adapter com o mesmo sqlc/tx e
relógio servidor. Sem imports entre application/domain de módulos diferentes,
dependências novas, serviços externos, migrations ou alterações no Supabase.

ShopContext usa SelfService no lugar do antigo RequiresPublication: a projeção
agora seleciona tanto política pública como administrativa. O valor vem do banco,
nunca de JSON. Fonte diferente de manual continua tratada conservadoramente como
submetida à política comercial. FindAgendaShop não usa esse flag para bloquear
leitura administrativa.

## Idempotência e autorização

Preservar ordem: resolver unidade ativa sob lock compartilhado, claim idempotente,
lock exclusivo do profissional, revalidar usuário/membership, recuperar recibo
anterior ou verificar habilitação para uma reserva nova. Não inverter calendário
e membership, pois a desativação de contas segue esse protocolo.

Replay de reserva salva não é uma nova operação comercial. Ele continua exigindo
autorização atual, mesmo depois de suspensão, e devolve o comprovante original
cifrado, não uma fotografia do estado atual. Chave/corpo diferentes conflitam;
janela de retenção e validação de entrada permanecem. Uma criação negada reverte
também o claim: não deixa cliente, reserva ou tentativa incompleta persistidos.

O guard é executado no caso de uso, não apenas no handler ou frontend. Falta do
adapter em self_service falha fechada como indisponibilidade, não libera e não
finge ausência de pagamento. Falhas operacionais preservam a causa internamente,
mas a resposta/log não expõe SQL, token, chave, telefone ou código da reserva.

## Concorrência financeira

Booking continua READ COMMITTED para preservar o protocolo existente da agenda.
Billing obtém FOR SHARE na conta financeira existente, usando a query agora
compartilhada LockBillingOperationAccount (renomeada de publicação). Só depois
consulta assinatura e ledger; essa segunda instrução enxerga uma reconciliação
que tenha concluído enquanto o guard esperava pelo lock da conta.

O webhook usa FOR UPDATE e mantém a conta bloqueada até o commit financeiro.
Se ele passar primeiro, a criação observa a nova situação; se a reserva passar
primeiro, o webhook aguarda seu commit. O lock é mantido até completar reserva e
recibo, sem chamadas Stripe sob transação. Ausência de conta não cria conta ao
consultar e não concede acesso. O relógio é consultado ao avaliar a habilitação;
isso não congela o tempo nem promete cobertura indefinida após essa decisão.

Ordem administrativa: shop → claim → profissional → usuário/membership → conta
financeira. Publicação/Checkout mantêm seus protocolos; reconciliação não precisa
do profissional/membership. SQL administrativo direto é privilegiado e deve
respeitar a serialização financeira; constraints de agendamento por si só não
comprovam assinatura. Nenhuma alteração na exclusion constraint de horários.

## HTTP e experiência futura

Owner sem cobertura recebe 402 shop_subscription_required; barber recebe 403
com o mesmo código e orientação para chamar o dono. Não encaminhar barbeiro ao
Checkout nem divulgar situação detalhada, Customer ou contrato. Cobrança/status
comercial continuam owner-only. Falha operacional da elegibilidade retorna 503.

Nenhum redirect, metadata ou pagamento cria reserva automaticamente. A próxima
entrega de frontend deverá preservar o rascunho, consultar o estado após retorno,
e exigir revisão/confirmação explícitas. Não repetir tentativas com resultado
incerto usando outra chave. Portal e live continuam pendentes, assim como teste
end-to-end na conta Stripe; fixtures financeiras desta entrega são locais.

## Verificação

Unitários cobrem ordem, replay, legado, adaptação ausente e falhas operacionais.
HTTP/PostgreSQL reais cobrem proprietário/barbeiro/outro tenant, estados e ledger,
rascunho pago não publicado, ausência de dados parciais, recuperação após bloqueio,
permissão revogada, agenda/configurações/cancelamento, cancelamento programado,
unicidade de horário, falha deferida de commit e ambas as ordens de corrida entre
reserva e reconciliação. Esperas de lock são observadas em pg_stat_activity, não
presumidas a partir de um atraso arbitrário. Regressões de booking, contas, billing
e publicação são executadas no banco descartável separado.
