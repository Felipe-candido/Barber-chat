# 0022 — Oferta revisada e Checkout na interface

Data: 2026-10-30. Estado: implementado em TEST; validação com conta Stripe pendente.

## Contexto

O owner precisa conhecer o valor antes de abrir o pagamento. Uma tentativa durável
pode preservar um Price diferente da configuração atual. Aceitar um preço do
browser ou mostrar um valor fixo no frontend poderia divergir da cobrança.

## Decisão

GET billing/offer escolhe o plano no servidor ou na tentativa pendente e lê seu
preço ativo mensal BRL na Stripe, sem criar registros. Revalida owner e seleção
após I/O. Expõe valor em centavos e um checksum de unidade/plano/Price/condições,
sem o identificador externo. O checksum não concede permissão e não é uma reserva.

POST billing/checkout exige intent + offer_id e recalcula as condições antes de
reservar a tentativa. Revalida a seleção sob os locks existentes. Recuperação
mantém os parâmetros Stripe do protocolo 1; nenhuma opção foi acrescentada à
criação congelada. Antes de liberar uma Session aberta, exige moeda e totais
iguais à oferta revisada. Divergência retém o link para revisão; não apaga a
tentativa nem cria outro contrato para contornar o problema.

O provider depende de secret de teste, Price e webhook secret no bootstrap: o
painel não abre cobrança sem o canal configurado de confirmação financeira.
Isso não comprova que o endpoint já esteja acessível pela Stripe; homologação
com Stripe CLI/conta ainda é necessária.

## Interface e recuperação

Configurações do owner mostra condições retornadas pela API. GET não cobra.
Preparar Checkout exige clique explícito; um bloqueio síncrono evita duplo POST.
O link validado abre em nova aba para preservar Configurações. Edição não salva
bloqueia início/abertura. O frontend não armazena cartão, segredo ou sessão Stripe.
O link deixa de ser oferecido antes da expiração informada.

Resposta perdida não é rejeição confirmada: nova consulta recupera as condições
da mesma tentativa, mas nunca reenvia POST sozinha. Retorno do Checkout e botão
Conferir assinatura fazem somente leitura comercial. Webhook continua sendo a
fonte de confirmação; publicação/agendamento não são repetidos automaticamente.

## Consequências

Sem migration, nova dependência ou conversão de unidades manuais. Clientes HTTP
precisam enviar offer_id; intent isolada passa a retornar 422. Contratos atuais
não recebem botão de nova assinatura. Customer Portal, integração do bloqueio no
formulário manual e política live permanecem etapas separadas.
