# 0024 — Versões do envelope Stripe e recuperação por operador

Data: 2026-10-30. Estado: implementado em TEST.

## Evidência e correção

Mesmo com --latest, a CLI 1.55.3 entregou envelopes 2026-09-30.endive. A exigência
anterior de igualdade com a versão de leitura 2026-10-28.endive rejeitou todos
os eventos antes do inbox, inclusive invoice.paid de um pagamento aprovado.
A orientação anterior de que --latest garantiria a versão exata foi incorreta.
Esta decisão atualiza as restrições de envelope dos ADRs 0019 e 0023.

## Fronteira de confiança

Aceitar explicitamente os envelopes 2026-09-30.endive e 2026-10-28.endive. O
verificador só extrai ID/tipo/versão/data do evento e ID do recurso; esses campos
foram conferidos na entrega real. Não desserializa o snapshot inteiro em um SDK
nem usa seus valores como prova financeira. Leituras atuais de recursos seguem
autenticadas e fixadas em 2026-10-28.endive. A versão recebida permanece no inbox.

HMAC do corpo original, tolerância temporal, TEST obrigatório, ausência de
Connect, associações locais, Price/Customer/Session e ledger continuam exigidos.
Versões desconhecidas são recusadas. Logs distinguem unsupported_api_version
de signature_or_payload sem retornar detalhes sensíveis ao navegador.

## Eventos já recusados

cmd/reconcile-stripe-event recebe apenas um evt_ID e busca o evento original
na Stripe por HTTPS autenticado com a chave TEST. Confere envelope, ambiente e
identidade retornada e executa ReceiveWebhook com o mesmo inbox/reconciliador.
Não recebe corpo arbitrário nem fabrica Stripe-Signature para uma chamada HTTP.
É uma ferramenta de operador com acesso ao banco, não um endpoint administrativo.

O comando é repetível sem nova cobrança; a deduplicação continua por evento.
Consulta o resultado comercial após o commit e distingue receipt ignorado de
processado. Nenhum evento ou comando de recuperação publica uma unidade.
Para ativar o link, o proprietário confirma Publicar barbearia no painel.

## Validação

Cobertura de ambas as versões, rejeição de versão desconhecida, assinatura
inválida, modo live, identidade divergente, duplicatas e ausência de publicação
automática. O invoice.paid reportado foi recuperado e reconciliado; a consulta
de entitlement confirmou operation_allowed=true sem nova cobrança.

Sem migration ou dependência nova.

Referências: [versionamento de webhooks](https://docs.stripe.com/webhooks/versioning),
[consulta de eventos](https://docs.stripe.com/api/events/retrieve).
