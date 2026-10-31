# 0023 — Compatibilidade do filtro de cartão e recuperação de Checkout

Data: 2026-10-30. Estado: implementado em TEST.

## Problema observado

A API fixada em 2026-10-28.endive usa allowed_payment_method_types para filtrar
cartão. O pedido anterior enviava payment_method_types e recebia 400
invalid_request_error nesse parâmetro. Essa resposta ficou associada à chave
original; trocar apenas o corpo produziu idempotency_error.

Além disso, o listener local estava encaminhando para um endereço de exemplo
(4242/webhook) e usando a versão da conta (2026-09-30.endive). A API do projeto
ouve em 127.0.0.1:8080/api/v1/webhooks/stripe e exige a versão snapshot fixada.

## Decisão

Não descartar a tentativa local nem gerar outro UUID. Recuperar primeiro o pedido
original com seus parâmetros e chave. Sucesso antigo continua válido; Session já
vinculada continua sendo consultada por GET, sem repetir criação.

Somente um 400 com código invalid_request_error ou parameter_unknown e parâmetro
exatamente payment_method_types autoriza o formato corrigido. Esse ramo usa a
chave determinística original + :card-filter-v2 e mantém unidade, tentativa,
Customer, Price, retorno e metadados. Cada formato possui um corpo estável.
Se a resposta corrigida se perder, repetir a recuperação retorna a mesma Session.

Timeout, 5xx, conflito de idempotência, outro parâmetro ou resposta de sucesso
malformada nunca acionam a alternativa. Mantém-se a janela de recuperação de 20h,
menor que o prazo mínimo de retenção de chaves da Stripe. Uma falha genérica não
é evidência para abrir outra assinatura. Esta é uma transição de formato após
rejeição explícita, não uma política geral de repetição com novas chaves.

Nenhuma migration ou dependência nova. O contrato HTTP do painel não muda.

## Diagnóstico e operação

ProviderFailure preserva somente status, códigos/parâmetros permitidos e request
ID da Stripe para o log interno. Mensagem bruta, URL, chave, contato e IDs de
recursos não são registrados nem retornados ao navegador.

Usar stripe listen --latest --all-snapshot --events-from '@self' com o destino
do projeto. Conferir no banner a versão 2026-10-28.endive e manter o segredo
do listener no backend. --latest acompanhará futuras versões; não relaxar a
validação para aceitar estruturas desconhecidas.

## Validação

Testes cobrem sucesso no formato anterior, rejeição explícita, dois formatos
estáveis, resposta corrigida perdida e recuperação sem duplicação. Falhas
ambíguas não usam outra chave; logs e respostas não expõem mensagens privadas.
A tentativa TEST existente da unidade reportada foi recuperada pelo caso de uso
e vinculada, sem pagamento ou publicação. O ciclo de pagamento/webhook ainda
depende de reiniciar o listener com destino e versão corretos.

Referências: [Checkout Session](https://docs.stripe.com/api/checkout/sessions/create),
[idempotência](https://docs.stripe.com/api/idempotent_requests) e
[Stripe CLI listen](https://docs.stripe.com/cli/listen).
