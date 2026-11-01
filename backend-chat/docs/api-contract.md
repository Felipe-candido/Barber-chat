# Contrato com o frontend

## Histórico e indicadores de atividades

GET /api/v1/admin/shops/{slug}/reports/activity retorna o resumo; o sufixo
/history retorna o histórico paginado. Ambas exigem Bearer e acesso atual à
unidade. Owner consulta a unidade ou seleciona profissional; barber fica no seu
profissional ativo. Histórico administrativo continua disponível sem cobertura
comercial. [Contrato completo](reporting-openapi.yaml), [ADR 0036](adr/0036-activity-report-http-api.md).

from/through obrigatórios, inclusivos no fuso cadastrado, até 366 datas civis.
Filtros comuns: source=all/services/products (padrão all), customer_name, phone,
phone_country, professional_id, service_id, product_id. Serviço/produto filtram
somente seus próprios ramos em all; origem exclusiva rejeita o outro recurso.
Nome é trecho literal sem distinguir caixa, telefone é igualdade normalizada;
venda avulsa sem atendimento não corresponde a filtros de contato. Codificar +
como %2B na URL. Não enviar parâmetros vazios, desconhecidos ou repetidos.

Histórico também aceita appointment_status e sale_status, cada um no seu ramo,
limit (padrão 25, máximo 100) e cursor opaco recebido em next_cursor. O resumo
rejeita esses parâmetros exclusivos do histórico. Cursor fica vinculado ao ator,
unidade, acesso e filtros normalizados; trocar filtros exige limpar o cursor.
next_cursor vazio encerra a lista. Não há snapshot congelado entre páginas.

Respostas incluem period com datas locais, limites UTC e timezone; observed_at;
valores em centavos, contadores separados e coleções vazias como []. O resumo
contém counts, values, by_day, by_professional, by_service e by_product. Somar
serviços completed e itens de vendas recorded; previsão fica separada. Não
somar contagens de comprovantes dos grupos por produto para obter o total global.

Histórico contém items com um atendimento ou item vendido por linha. value_cents
é o valor original; recorded_value_cents é a contribuição válida, inclusive zero
após cancelamento. Situações e IDs não aplicáveis são null; contato anônimo é
string vazia. Não são retornados códigos públicos, hashes ou motivos/atores de
correção. Valores não comprovam pagamento recebido ou lucro. JSON usa no-store.

Erros: 400 invalid_report_query; 401 sessão ausente/inválida; 403 access_denied;
404 shop_not_found; 500 report_unavailable; 503 temporarily_unavailable (pode
repetir a leitura). Query string limitada a 4096 bytes, cursor a 1024 caracteres.

## Situação e conclusão automática dos atendimentos

Migration 00019 e API atual habilitam a entrega F. A conta principal (owner)
gerencia qualquer atendimento da unidade, inclusive de profissionais inativos;
barber somente os do seu profissional ativo. A autoridade é revalidada dentro
da transação. Consulta/correção não exige assinatura/publicação.

GET /api/v1/admin/shops/{slug}/appointments/{appointmentID} retorna
{appointment,history,history_truncated,replayed,result_version}, com contato
privado e auditoria limitada às últimas 100 alterações. A agenda administrativa
também inclui version,status_source,status_reason,status_actor_id,updated_at.
Metadados/auditoria não entram no comprovante público por código.

POST no sufixo /status, Bearer e Idempotency-Key (64 hex minúsculos), recebe
{status,expected_version,reason}. Status: completed/no_show/cancelled; motivo
de 1–500 caracteres sem controles, JSON até 8 KiB. Ausência exige início atingido;
realizado exige fim reservado atingido. Cancelamento administrativo também corrige
o passado; cancelamento público mantém o prazo anterior ao início. Correção
de intervalo cancelado que foi ocupado por outra reserva dá conflito.

Versão, alteração, auditoria e chave são atômicas. 409 exige consulta/revisão;
503/rede/resposta incompatível exige recuperar a mesma chave/corpo. Replay não
incrementa versão e informa estado atual mais result_version do comando original:
outra correção pode ter ocorrido depois. O painel preserva a tentativa nesta aba,
separada por usuário/unidade/atendimento, e não repete POST automaticamente.

A API verifica a cada 15 segundos até 100 reservas scheduled/confirmed cujo
ends_at passou. A fonte durável é o PostgreSQL; a verificação ocorre também ao
iniciar, recuperando atrasos após interrupções. Réplicas usam locks/SKIP LOCKED e
revalidam estado, sem sobrescrever ausência/cancelamento. O marco de ativação da
migration evita concluir em massa horários encerrados antes dela. Reservas legadas
com fim futuro ainda são elegíveis; demais antigas podem ser corrigidas pelo dono.

Exibir “Concluído automaticamente” quando source=automatic. Conclusão presume
comparecimento e não comprova pagamento. Correção preserva snapshots e vendas
de produtos. O executor roda junto da API; a migração futura dessa execução para
worker poderá reutilizar o caso de uso. Ver [contrato](appointment-lifecycle-openapi.yaml)
e [ADR 0032](adr/0032-automatic-appointment-completion.md).

## Produtos e estoque administrativos

API e painel administrativo implementados. GET /api/v1/admin/shops/{slug}/products
lista ativos para staff, com search literal, limit até 100 e cursor; owner pode
incluir inativos com include_inactive=true. POST na mesma rota cadastra produto,
saldo zero e abertura opcional em uma transação. PUT /products/{productID} substitui
name, description, price_cents e active com expected_version. PUT no sufixo
/stock-minimum altera minimum_quantity com expected_stock_version.

GET /products/{productID}/stock-movements é auditoria owner-only. POST no mesmo
endereço registra operation=receipt (delta positivo) ou adjustment (delta não zero),
quantity_delta e reason. Exige owner; saldo negativo é recusado também pelo banco.
Baixa de venda usa a operação específica de sales; a rota manual não aceita esse tipo.

POSTs exigem Idempotency-Key de 64 caracteres hex minúsculos e JSON até 8 KiB.
201 é criação; 200 com replayed=true recupera IDs originais e estado atual do produto,
sem reaplicar a movimentação. Depois de resultado incerto, repetir a chave/entrada
originais. Conflito de chave/versão/estoque retorna 409; acesso revogado impede replay.
Configurar catálogo/estoque não exige assinatura/publicação; novas vendas usam
seu próprio guard comercial, descrito abaixo.

Aplicar migrations até 18 com a versão atual da API. As migrations desta entrega
foram validadas em PostgreSQL local descartável; não foram aplicadas no Supabase.
Ver [contrato completo](products-openapi.yaml) e [ADR 0028](adr/0028-products-and-transactional-stock.md).

## Registro de vendas de produtos

Registro de vendas também está disponível em POST /api/v1/admin/shops/{slug}/sales,
com Bearer e Idempotency-Key de 64 caracteres hex minúsculos. Corpo contém somente
professional_id/appointment_id opcionais e itens com product_id, quantity,
expected_product_version e expected_unit_price_cents. Preço revisado é condição;
snapshots e total vêm do servidor. Barber vende em seu profissional; owner pode
atribuir ou deixar avulso. Novo registro exige cobertura em unidades self_service,
independentemente da publicação; unidades manuais preservam compatibilidade.

Venda, itens, baixas e resultado da chave são atômicos. 201 devolve o comprovante;
200 com replayed=true recupera o mesmo registro sem repetir a baixa, após validar
acesso atual. 409 informa preço/estoque/vínculo alterado ou conflito de chave.
402/403 com shop_subscription_required bloqueia novas vendas de owner/barber.
Em resultado incerto, preservar chave e entrada; não criar outro pedido automaticamente.
O banco confere total, itens e movimentações no commit. O painel oferece produtos
e vendas em /admin/{slug}/produtos e /admin/{slug}/vendas.
[Contrato de vendas](sales-openapi.yaml) e
[ADR 0029](adr/0029-recorded-sales-and-stock-atomicity.md).

GET na mesma rota recebe from e through obrigatórios (AAAA-MM-DD, inclusivos no
fuso da unidade, até um ano), professional_id/status opcionais, limit (25 padrão,
até 100) e cursor. Lista comprovantes completos em sold_at/id decrescentes;
next_cursor vazio encerra. Owner lê todas as vendas da unidade; barber só as
atribuídas ao seu profissional atual, com filtro SQL antes da paginação. GET no
sufixo /{saleID} detalha; recurso fora do escopo retorna 404.

POST /sales/{saleID}/cancel exige owner, Idempotency-Key e corpo
{expected_version,reason,restock}. Restock deve ser booleano explícito: true
devolve integralmente cada item à origem de estoque; false mantém a saída.
Itens/preços/ator original não são reescritos. A resposta inclui cancellation
{actor_id,reason,restock,cancelled_at}, status=cancelled e version=2.
Anulação, reposições opcionais e resultado idempotente são uma única transação.
Um novo pedido com mesmo motivo/reposição recupera a anulação sem novos movimentos;
instruções diferentes dão 409. Uma reversão única por saída e constraints diferidas
impedem duplicidade/anulação incompleta também em SQL. Produtos inativos podem
receber devolução; excesso do limite de estoque desfaz a anulação inteira.
Consultas/anulação/replay não exigem cobertura comercial; acesso atual é obrigatório.
Isso não processa reembolso de pagamento nem altera o agendamento vinculado.
O histórico de estoque expõe sale_item_id e reverses_movement_id para rastreio.
Ver [ADR 0030](adr/0030-sale-cancellation-and-scoped-history.md).

A interface aplica as permissões no menu/formulários e a API continua autorizando
cada chamada. Venda passa por seleção, revisão e confirmação; o mesmo formulário
abre nos detalhes de um atendimento elegível da agenda, com profissional/vínculo
fixos. Respostas incertas de POST preservam chave/corpo em sessionStorage por
unidade/ator/operação; reabrir a janela após reload oferece recuperação explícita,
sem reenvio automático. PUT de cadastro/mínimo exige conferir dados atuais após
conflito ou resposta incerta. Anular pede motivo e decisão de reposição sem padrão.
Ver [ADR 0031](adr/0031-products-stock-and-sales-panel.md). Esta interface não
acrescenta migrations; requer as migrations existentes até 00018.

## Autenticação e unidades

Login envia credenciais apenas ao Supabase. O frontend chama `GET /api/v1/admin/me` e depois `GET /api/v1/admin/shops` com Bearer, mostra as unidades elegíveis e abre o painel após a escolha. Cada operação administrativa revalida usuário, unidade e vínculo ativos. O provisionamento manual continua válido; o autocadastro começa em `/cadastro`, confirma e-mail no Supabase e continua em `/cadastro/barbearia`. Cadastrar um profissional não cria uma conta Supabase.

### Autocadastro de proprietário e barbearia

`POST /api/v1/onboarding/shops`, Bearer e `Idempotency-Key` de 32 bytes aleatórios em hexadecimal minúsculo. Corpo JSON de até 8 KiB: `{display_name,name,slug,timezone,address}`; timezone vazio/ausente assume `America/Sao_Paulo`. `address` exige `postal_code,street,number,district,city,state`; `complement` é opcional. Endereço brasileiro: CEP com oito números (hífen aceito), UF válida, número ou `S/N`. Endereço incompleto retorna 422 `invalid_shop_address`. Não aceita senha, e-mail, UUID, role, publicação ou situação financeira. Confirmação de e-mail vem de `/auth/v1/user`, não de metadata. Essa rota usa contexto próprio de onboarding e não exige usuário local prévio; não enfraquece `/admin/me`.

Retorna 201 `{shop_id,name,slug,timezone,replayed:false}` após commit atômico de usuário local, unidade, endereço, owner e chave de criação. Mesmo pedido normalizado/chave retorna 200 e `replayed:true`, com dados atuais da unidade e acesso revalidado. Usuário existente não é renomeado ou reativado. 409 `shop_slug_taken` pede outro slug; 409 `idempotency_conflict` indica chave usada com outra entrada. Resultado incerto (503/rede) exige repetir o pedido original e a mesma chave, sem gerar outra unidade. O frontend preserva a tentativa enviada apenas nesta aba até a confirmação. Não registrar tokens/chaves ou enviar na URL.

`SUPABASE_PUBLISHABLE_KEY` no backend aceita a chave publicável ou anon legada do mesmo projeto. Não usar service_role. Ausência retorna 503 `registration_not_configured`, sem impedir login existente. A senha, confirmação de e-mail e obtenção da sessão continuam no Supabase. A conta Auth permanece se o usuário precisar corrigir o cadastro local; não há transação distribuída nem exclusão compensatória de Auth.

Migration 00010 marca as novas unidades da API como `self_service`, ativas para configuração e não publicadas; 00011 armazena endereços completos sem inventar dados legados. Catálogo público, profissionais, disponibilidade e criação pública exigem publicação solicitada e cobertura de acesso confirmada em billing. A criação revalida dentro da transação após recuperar um possível replay: uma reserva salva continua recuperável após suspensão. Consultar/cancelar uma reserva existente não recebe esse guard. Unidades manuais mantêm comportamento anterior até rollout comercial explícito. Configurações owner, Checkout TEST com revisão na interface, webhook TEST, publicação explícita e trava comercial da criação manual estão implementados nas seções abaixo. O formulário manual inclui recuperação da assinatura; Customer Portal TEST também está disponível ao owner; live continua desabilitado.

Após a migration 00006, há somente dois perfis fixos: `owner` e `barber`. Os vínculos antigos tornam-se proprietários; novos vínculos precisam declarar o perfil. `GET /api/v1/admin/shops/{slug}/access` retorna `{role,professional_id}`. POST de serviço/profissional exige proprietário; ambos podem ler o catálogo. Barbeiro só consulta a agenda de seu profissional ativo, filtrada no banco antes da paginação. A interface consulta o perfil e não oferece gestão de equipe, contas ou preços ao barbeiro.

Health/readiness, identidade, catálogo, contas e booking estão descritos no [OpenAPI](openapi.yaml). Readiness verifica conectividade PostgreSQL, não migrations nem RabbitMQ. WhatsApp não está implementado.

## Configurações básicas da barbearia

`GET/PUT /api/v1/admin/shops/{slug}/settings` exige Bearer, usuário local ativo e
vínculo de proprietário ativo. Barbeiros não leem nem alteram esse recurso. Não
exige assinatura para configurar a unidade. O slug da rota é resolvido pela API;
nenhum UUID do JSON define o tenant. A identidade e o membership são conferidos
novamente dentro da transação, com lock compartilhado até seu encerramento.

GET retorna `{shop_id,name,slug,timezone,phone,description,address,version}`. Telefone
e descrição ausentes são `""`; endereço legado ainda não preenchido é `null`.
Não inventar endereço para registros antigos. PUT exige o endereço completo,
inclusive ao editar uma unidade legada, e substitui todos os campos editáveis:

```json
{
  "version": 1,
  "name": "Barbearia Centro",
  "phone": "(11) 91234-5678",
  "description": "Atendimento com horário marcado.",
  "address": {
    "postal_code": "01310-100",
    "street": "Avenida Paulista",
    "number": "100",
    "complement": "Sala 2",
    "district": "Bela Vista",
    "city": "São Paulo",
    "state": "SP"
  }
}
```

Nome: 1–150 caracteres; telefone comercial brasileiro válido com DDD, até 40
caracteres de entrada, normalizado para E.164 (`+5511912345678`); descrição de texto
simples com até 1000 caracteres. Telefone/descrição são chaves obrigatórias do PUT;
enviar `""` para limpar. Endereço segue as regras do cadastro, complemento opcional.
Slug, fuso, publicação, perfil e cobrança são somente leitura/não aceitos no PUT.

Migration 00012 acrescenta descrição e versão em `shops`, reutilizando `phone` e
`shop_addresses`. PUT grava dados básicos/endereço numa única transação e retorna
200 com a versão incrementada. Não modifica agendamentos, vínculos ou assinatura.
Versão desatualizada/repetida: 409 `shop_settings_conflict`; recarregar e revisar,
sem sobrescrever automaticamente. Em erro de rede/503, consultar GET para conferir
se o salvamento ocorreu antes de decidir tentar novamente. Respostas sem cache,
JSON estrito de até 8 KiB. Campos inválidos: 422 `invalid_shop_settings` ou
`invalid_shop_address`.

O proprietário acessa `/admin/{slug}/configuracoes` pelo menu do painel. O
formulário usa estes GET/PUT e exige endereço completo também para unidades
legadas. Slug e fuso são somente leitura. Configurar não publica a unidade nem
inicia cobrança. Em conflito ou resultado incerto, a tela bloqueia nova escrita
e oferece consulta explícita à API; nunca repete PUT automaticamente. A revisão
preserva mudanças remotas nos campos não editados localmente e exige confirmação
e novo salvamento para reaplicar o rascunho com a versão atual. Se a consulta
falhar, o rascunho permanece bloqueado. A autorização do backend continua sendo
a autoridade, inclusive quando o perfil mostrado no navegador está desatualizado.

## Estado comercial e publicação — backend

`GET /api/v1/admin/shops/{slug}/commercial-status` é exclusivo do proprietário.
Consulta apenas o banco, sem chamar Stripe. Retorna `shop_id`, `slug`,
`publication_managed`, `publication_requested`, `public_booking_enabled`,
`can_publish`, `blockers`, `readiness` e `subscription`.

`subscription` contém `status`, `plan_key`, `operation_allowed`, `reason`,
`coverage_until` (fim exclusivo ou null) e `cancel_at_period_end`. Não expõe
Customer, Subscription ou Price IDs externos. `readiness` contém os booleanos
`address`, `service`, `professional`, `schedule`. É preciso pelo menos um serviço
ativo elegível, um profissional ativo vinculado a esse catálogo e uma janela
recorrente que comporte serviço + margem arredondados à grade. Não exige horário
livre hoje: agenda cheia não deve impedir publicação. Disponibilidade e conflitos
continuam sendo verificados por booking ao reservar.

`PUT /api/v1/admin/shops/{slug}/publication`, owner, JSON estrito até 1 KiB:
`{"requested":true}` publica; `{"requested":false}` retira do ar. 204 somente após
commit, sem corpo. Publicar exige cobertura e prontidão dentro da transação;
despublicar não exige pagamento nem configuração completa. 409
`publication_blocked` orienta consultar GET para ver pendências. Barbeiro/outro
tenant recebe 403. IDs, roles, preços e URLs não são aceitos no corpo.

`publication_requested` é intenção persistida; `public_booking_enabled` combina
intenção e cobertura atual. Pagamento não cria intenção. Perder cobertura oculta
o agendamento, sem apagar a intenção: renovação válida volta a habilitar uma
unidade que permaneceu publicada. Se o owner retirou do ar explicitamente, pagar
novamente não republica. Prontidão é validada ao publicar, não uma promessa de
disponibilidade perpétua: catálogo/agenda continuam revalidados nas reservas.

Transação SERIALIZABLE, locks de usuário/membership, shop e conta financeira;
nenhuma chamada externa. Conflito de serialização/erro de commit retorna 503.
Não repetir escrita automaticamente: consultar estado e decidir. Publicação é
separada de settings_version; PUT de dados básicos não pode sobrescrevê-la.
Não cancela assinatura nem apaga reservas. Unidades `manual` mantêm o acesso
anterior e retornam `publication_managed=false`, `status=not_required`; tentativa
de mudar publicação retorna 409 `publication_not_managed`. Prontidão não é
avaliada nesse modo. Não há migration nova. Em Configurações, o owner consulta
o estado comercial, confirma publicação/retirada e compartilha somente o link
habilitado pela API; alterações locais não salvas bloqueiam a publicação.

Ver [OpenAPI](shop-publication-openapi.yaml) e
[ADR 0020](adr/0020-explicit-shop-publication.md).

## Checkout de assinatura — TEST com interface em Configurações

`POST /api/v1/admin/shops/{slug}/billing/checkout`, Bearer e owner ativo, unidade
self_service. Primeiro consultar `GET /api/v1/admin/shops/{slug}/billing/offer`.
Retorna `{shop_id,offer_id,plan_key,amount_cents,currency:"BRL",interval:"month",
interval_count:1,livemode:false,recovering}` com preço lido da Stripe. Uma tentativa
pendente usa seu Price original, mesmo após mudança de configuração. A consulta
não cria conta, Customer ou tentativa e revalida acesso após a chamada externa.

POST recebe JSON até 1 KiB, somente `{"intent":"publish","offer_id":"<64 hex>"}`
ou intenção `manual_booking` com a mesma confirmação. `offer_id` é uma versão das
condições selecionadas pelo servidor, não uma credencial. O backend recalcula;
mudanças retornam 409 `checkout_offer_changed`, exigindo nova consulta e revisão.
Não aceita valor, IDs de Stripe/tenant ou URLs. Clientes antigos que enviavam só
intent precisam passar a consultar e confirmar a oferta.
Retorna 200 `{attempt_id,intent,url,expires_at,livemode:false}` para Session aberta
confirmada e vinculada localmente. Sem cache. A tentativa pendente é recuperada,
não substituída; preço, retorno e intenção originais ficam preservados.

Requer STRIPE_SECRET_KEY de teste, STRIPE_MONTHLY_PRICE_ID de preço ativo mensal
BRL fixo/positivo/sem trial e STRIPE_WEBHOOK_SECRET, além de FRONTEND_ORIGIN. Ausência retorna 503
`billing_not_configured` sem afetar login/configurações. Chave live é rejeitada.
409 `billing_not_required` preserva unidades manuais; `subscription_exists` impede
outro contrato corrente. `checkout_confirmation_pending` aguarda confirmação
financeira, sem acesso; `checkout_requires_review` bloqueia recuperação insegura,
inclusive tentativa não vinculada com 20h ou registros legados sem parâmetros.
`checkout_expired` exige expiração confirmada pela Stripe; só outro pedido
explícito inicia tentativa nova. 503 `checkout_unavailable` não prova rejeição:
repetir este endpoint para recuperar, não criar assinatura por outro caminho.

Nenhuma URL de retorno ou Session complete ativa/publica/reserva. A confirmação
financeira é feita apenas pelo webhook assinado descrito abaixo. Em Configurações, o owner consulta as condições,
confirma a preparação e abre o Checkout hospedado em nova aba. O link só é
exibido após conferir host HTTPS Stripe, validade, ambiente e subtotal/total BRL.
Nenhum POST é repetido automaticamente: resposta incerta exige consulta e nova
confirmação para recuperar a tentativa original. Conferir assinatura apenas
reconsulta estado comercial; publicar exige outra ação. [Contrato](billing-openapi.yaml),
[decisão](adr/0018-durable-test-stripe-checkout.md) e
[planejamento das próximas entregas](billing-next-steps.md).

## Customer Portal Stripe TEST

`POST /api/v1/admin/shops/{slug}/billing/portal`, owner autenticado, JSON `{}` de
até 1 KiB, sem campos adicionais. O servidor escolhe Customer/configuração/retorno;
não recebe IDs Stripe nem URLs do navegador. Exige unidade self_service e contrato
local reconciliado. Cobertura vencida/cancelamento não impedem consultar faturas
ou regularizar cartão. Unidades manuais mantêm acesso anterior sem serem cobradas.

Resposta 200 `{shop_id,url,open_until,livemode:false}`, no-store. A URL HTTPS
billing.stripe.com é uma credencial apenas em memória, com janela local de quatro
minutos para abertura; open_until não é a expiração oficial Stripe. O frontend
abre em outra aba e mantém o cadastro não salvo. Revalidação de owner/tenant após
a chamada externa impede divulgar o link se o acesso mudou durante a operação.

`STRIPE_PORTAL_CONFIGURATION_ID` define uma política TEST dedicada, validada por
`go run ./cmd/configure-stripe-portal`: cartão, faturas, cancelamento ao fim do
período, sem mudança de plano/cupons/login público. Requer secret key TEST, webhook
configurado e FRONTEND_ORIGIN. 409 `portal_not_available` pede aguardar conciliação;
503 `portal_not_configured`/`portal_unavailable` não disparam retry automático.
Nenhuma conta, assinatura ou cobertura é criada por essa rota. `portal=returned`
é somente navegação; Conferir situação consulta commercial-status. Webhook continua
sendo a autoridade para atualização financeira. Sem migration nova.
Ver [ADR 0026](adr/0026-owner-stripe-customer-portal.md).

## Webhook Stripe TEST

`POST /api/v1/webhooks/stripe` usa `Stripe-Signature`, sem Bearer Supabase. Exige
corpo original JSON até 512 KiB, timestamp de entrega dentro de cinco minutos,
STRIPE_WEBHOOK_SECRET, evento snapshot 2026-09-30.endive ou 2026-10-28.endive e livemode=false. Não
aceita Connect. 400 para assinatura/envelope/ambiente/versão inválidos; 503 se
desconfigurado ou processamento pendente; 200 `{"received":true}` somente após
commit de processamento, duplicata terminal ou descarte explícito.

Eventos suportados de Checkout, assinatura e fatura estão no
[OpenAPI](billing-openapi.yaml). Inbox deduplica e guarda referências mínimas,
sem corpo bruto com PII. Reconciliação consulta estado atual na Stripe, valida
vínculos locais e aplica contrato, faturas, tentativa e inbox atomicamente. Uma
revisão por unidade impede snapshots antigos de sobrescrever consultas novas.
Faturas geram intervalos pagos exatos; não unir lacunas nem usar status active
ou próximo faturamento como prova de pagamento. O protocolo legado é preservado.
As duas versões de envelope são uma lista explícita: HMAC/tempo/ambiente/conta
continuam obrigatórios, a versão recebida é preservada e dados financeiros vêm
de leituras autenticadas fixadas em 2026-10-28.endive, não do snapshot recebido.

Recuperação por operador: `go run ./cmd/reconcile-stripe-event evt_...` busca o
evento na Stripe TEST por HTTPS autenticado e executa o mesmo reconciliador.
Não recebe payload arbitrário, não forja assinatura, não cria cobrança nem
publica. Exige as credenciais de servidor e acesso ao banco; não é rota pública.

Nenhum webhook publica ou cria agendamento. Falhas permanecem pending e retornam
503 para redelivery; não há scheduler local de retry nesta etapa. Antes de live,
definir estornos/disputas, monitorar pendências, limitar abuso e validar na conta
Stripe. Live continua desabilitado. Ver [ADR 0019](adr/0019-signed-stripe-webhooks-and-paid-coverage.md)
e [teste integrado pendente](billing-next-steps.md#teste-integrado-pendente-sem-cobrança-real).

## Contas de acesso dos barbeiros

Painel `/admin/{slug}/contas`, apenas para `owner`. Todas as rotas abaixo exigem Bearer, vínculo de proprietário ativo e corpo JSON de até 8 KiB, sem campos desconhecidos:

- `GET /api/v1/admin/shops/{slug}/accounts`: lista `{id,professional_id,email,user_id,state,active}`; `user_id` pode ser nulo e `state` é `pending`, `linked` ou `review`.
- `POST /accounts`: `{professional_id,email}`, retorna 201 com a conta vinculada e solicita convite por e-mail ao Auth.
- `POST /accounts/{accountID}/link`: `{user_id}`, recuperação explícita de solicitação pendente/em revisão; busca UUID no Auth e compara e-mail antes de vincular.
- `POST /accounts/{accountID}/status`: `{active:true|false}`, retorna `{active}` e altera somente acesso nesta unidade, não a conta global nem o cadastro do profissional.
- `POST /accounts/{accountID}/replace`: `{email}`, diferente do atual; exige desativar primeiro o membership vinculado. Retorna 201 com uma nova conta/UUID para o mesmo profissional e solicita convite. Disponível também para solicitações pendentes/em revisão.

A migration 00008 preserva o registro substituído como histórico (`replaced_at`), com índice único apenas para a conta atual por profissional. A troca remove apenas o membership inativo de barbeiro nesta unidade, não usuários, outras unidades ou agendamentos. Transação com lock do profissional serializa troca, vínculo e ativação; reler após o lock impede requisições antigas de reativar/vincular o UUID substituído. A chamada Auth ocorre depois do commit: se falhar, o NOVO registro fica pendente/em revisão. Não há compensação que restaure o acesso antigo nem envio automático duplicado. 409 `staff_account_must_be_inactive` exige desativação; 404 no UUID antigo exige atualizar a lista.

Os sufixos POST usam a mesma base da rota GET. Nenhuma senha, role ou chave administrativa é aceita do navegador. Há uma solicitação única por profissional, inclusive em falha ambígua. Conta Auth e banco local não compartilham uma transação: 409 `staff_account_needs_review` exige conferir a lista e resolver a solicitação pelo UUID; nunca repetir envio automaticamente. 409 `staff_account_exists` indica profissional já vinculado/reservado; 503 `staff_accounts_not_configured` indica chave ausente.

Configuração: `SUPABASE_SERVICE_ROLE_KEY` opcional apenas no `.env` Go (chave legada service_role JWT), `FRONTEND_ORIGIN` exata, e allowlist de redirects Supabase contendo `{FRONTEND_ORIGIN}/definir-senha`. Chave ausente não bloqueia o restante da API. SMTP padrão Supabase restringe destinatários; configure SMTP próprio para convites reais. O barbeiro abre o convite e define senha em `/definir-senha`; o frontend envia a senha diretamente ao Auth. Nunca adicionar a chave administrativa ao `.env.local` ou a variáveis `NEXT_PUBLIC_*`.

## Criação administrativa de agendamentos

`POST /api/v1/admin/shops/{slug}/appointments` exige Bearer e `Idempotency-Key`. Mesmo corpo e comprovante do POST público. Proprietário pode escolher qualquer profissional elegível da unidade; barbeiro só pode escolher o próprio profissional, independentemente de alterações no JSON. A identidade e unidade vêm do contexto autenticado, nunca do corpo.

Além dos middlewares, a transação revalida o acesso atual, inclusive em replay de reserva. O núcleo público é reutilizado: chave de idempotência → lock do profissional → verificação de acesso → disponibilidade atual → contato → reserva → comprovante → commit. A exclusão SQL de sobreposição continua como proteção final. A interface atualiza a agenda após criar e mantém a tentativa bloqueada em falha ambígua.

## Cadastro administrativo de profissionais

`GET/POST /api/v1/admin/shops/{slug}/professionals` exige Bearer e membership ativo. POST também exige `owner`. GET retorna array de `{id,name,active,service_ids}`. POST cria profissional, associações a serviços ativos da mesma unidade e calendário inicial em uma transação; retorna 201 com o mesmo formato.

```json
{
  "name": "João",
  "service_ids": ["11111111-1111-4111-8111-111111111111"],
  "schedule": {
    "interval_minutes": 60,
    "buffer_minutes": 0,
    "weekdays": [1, 2, 3, 4, 5, 6],
    "opens_at_minute": 570,
    "closes_at_minute": 1110
  }
}
```

Domingo = 0; minutos contam desde meia-noite no fuso da unidade. O exemplo atende segunda a sábado, 09:30–18:30. Intervalo: 1–1440; margem: 0–1440. É necessário pelo menos um serviço e um dia, sem duplicatas; o período deve comportar ao menos um bloco. O painel começa com intervalo 60 e margem 0, editáveis antes de cadastrar. Ainda não há edição de profissional/calendário, gestão de pausas ou folgas pela UI.

JSON administrativo tem limite de 64 KiB e rejeita campos extras, inclusive `shop_id`. POST de profissional não possui idempotência: numa falha de rede ambígua, atualizar a lista e conferir antes de iniciar novo cadastro.

## Fluxo público implementado

Chat/formulário em `/b/{slug}`, sem conta de cliente ou token administrativo. As rotas usam a base `/api/v1/public/shops/{slug}`:

| Método e sufixo | Resultado |
| --- | --- |
| GET /services | Serviços ativos |
| GET /professionals?service_id=UUID | `{shop_name,shop_timezone,today,last_booking_date,professionals:[{id,name}]}` |
| GET /availability?service_id=UUID&professional_id=UUID&date=YYYY-MM-DD | `{shop_timezone,slots:[{starts_at,ends_at}]}` |
| POST /appointments | Cria/reproduz reserva; exige Idempotency-Key |
| POST /appointments/lookup | Consulta uma reserva com `{"code":"..."}` |
| POST /appointments/cancel | Cancela uma reserva com `{"code":"..."}` |

Disponibilidade cobre hoje até 90 dias adiante no fuso da barbearia, omitindo inícios passados. Não há reserva temporária ao selecionar. Os horários seguem a abertura do expediente: 09:30, 10:30, 11:30 em grade de 60 minutos. A ocupação é `ceil((duração + margem) / intervalo) * intervalo`: serviço de 40 ocupa 60; de 80 ocupa 120. Todo o período deve estar livre e caber no expediente. Datas/horas civis ambíguas ou inexistentes por horário de verão são omitidas conservadoramente.

Corpo de criação (IDs ilustrativos; início deve vir da disponibilidade):

```json
{
  "service_id": "11111111-1111-4111-8111-111111111111",
  "professional_id": "22222222-2222-4222-8222-222222222222",
  "starts_at": "2026-10-03T12:30:00Z",
  "customer": {
    "first_name": "Ana",
    "last_name": "Silva",
    "phone": "+5511912345678",
    "phone_country": "BR"
  }
}
```

Nenhum preço, duração, fim, tenant ID, status ou consentimento é aceito do navegador. O servidor reconsulta as informações atuais. Telefone é normalizado para E.164 com metadados, sem comprovar titularidade. Cliente só é persistido ao confirmar, na transação da reserva.

Resposta 201 (primeira criação) ou 200 (repetição): `{appointment,code,replayed}`. O objeto `appointment` contém `id,shop_name,status,service_name,duration_minutes,price_cents,currency,professional_id,professional_name,starts_at,ends_at,shop_timezone`. Início/fim são instantes RFC3339; `ends_at` é o fim reservado, não necessariamente o término estimado do corte. Snapshots preservam serviço/preço/duração/nome do profissional/fuso. Nome e telefone do cliente não aparecem no comprovante público.

Consulta/cancelamento retornam `{appointment,can_cancel}`. O código aleatório de 160 bits permite consultar e cancelar apenas aquela reserva naquela unidade; não dá acesso à conta/histórico. Ele não vai na URL nem nos logs; a tabela de agendamentos armazena só SHA-256. Quem possuir o código poderá usá-lo: guardar em local privado. Cancelamento permitido estritamente antes do início; repetir cancelamento já concluído não altera novamente a versão. Não há expiração automática do código de consulta nesta etapa.

## Assinatura na criação administrativa

`POST /api/v1/admin/shops/{slug}/appointments` preserva o corpo/chave da criação
pública, mas exige Bearer e vínculo válido. Owner pode escolher profissional
elegível da unidade; barber somente o próprio profissional ativo. A transação
revalida essa permissão, inclusive para devolver um comprovante anterior.

Novas reservas de unidades `self_service` exigem cobertura confirmada no ledger
local. Não basta contrato active nem retorno do Checkout. Publicação não é
exigida para criar manualmente: uma unidade paga pode manter o link oculto.
Cobertura é avaliada no momento da criação, não na data futura do atendimento.
Unidades `manual` continuam operando sem adesão comercial automática.

Sem cobertura, owner recebe **402** `shop_subscription_required`, com orientação
para gerenciar assinatura. Barber recebe **403** com o mesmo código e orientação
para chamar o proprietário; não recebe URL de Checkout nem detalhes financeiros.
O endpoint de Checkout continua owner-only. IDs, role, preço ou flags comerciais
enviados pelo navegador não concedem elegibilidade. Falha na consulta financeira
ou ausência do adapter retorna **503**, não uma instrução para pagar novamente.

A ordem preserva recuperação: resolver unidade → reivindicar chave → lock da
agenda → revalidar funcionário → devolver resultado salvo, se houver → verificar
cobertura para reserva nova → revalidar catálogo/horário → persistir e confirmar.
Uma tentativa bloqueada é revertida sem contato, reserva ou chave pendente. Mesmo
pedido/chave já concluído recupera o comprovante após suspensão, dentro de sua
retenção; corpo diferente continua 409. Replay não é consulta do estado atual.

O guard obtém lock compartilhado na conta financeira e lê cobertura em uma query
posterior, dentro da mesma transação READ COMMITTED. Assim vê reconciliação que
terminou enquanto aguardava o lock; se a reserva passar primeiro, a reconciliação
aguarda seu commit. Não há chamada Stripe nem mutex local. Escritas SQL
privilegiadas fora da API devem respeitar esse protocolo; a exclusão de horários
protege sobreposição, não substitui a política de assinatura.

Login, configuração, agenda, consulta/cancelamento de reservas existentes não
recebem essa trava. Pagamento não recria uma reserva rejeitada nem publica o
link. O formulário administrativo usa GET privados em
`/api/v1/admin/shops/{slug}/booking/professionals?service_id=...` e
`/api/v1/admin/shops/{slug}/booking/availability?service_id=...&professional_id=...&date=...`.
São observações autenticadas, sem exigência de pagamento/publicação, com vínculo
revalidado na transação e profissional do barbeiro filtrado no servidor. As
rotas públicas continuam protegidas. Nenhum GET persiste contato ou reserva.

Ao receber shop_subscription_required, a interface mantém o rascunho em memória.
Owner consulta a situação e, se não houver contrato corrente, revisa e prepara
Checkout com intent=manual_booking. Pagamento abre em outra aba. Contrato
pendente/atrasado não oferece outra assinatura. Barbeiro recebe orientação para
o proprietário, sem consulta financeira ou acesso ao Checkout.

Conferir assinatura faz GET; após cobertura confirmada, Retomar revisão faz
nova consulta de horários. Horário ocupado leva à seleção preservando o contato.
Criar exige outro clique explícito. Em resultado anterior incerto, a mesma chave
e corpo permanecem congelados e o comprovante deve ser recuperado antes da
disponibilidade: a própria reserva salva pode ocupar o horário consultado.
O modal confirma descarte do rascunho e bloqueia fechamento durante operações
incertas/em andamento. O rascunho não sobrevive a recarga forçada; há aviso e
nenhum dado pessoal é gravado em localStorage/sessionStorage.
Ver [ADR 0021](adr/0021-staff-booking-commercial-eligibility.md).

## Repetição segura e concorrência

O frontend gera `Idempotency-Key` com 32 bytes criptograficamente aleatórios, representados por 64 caracteres hexadecimais minúsculos. É uma credencial secreta para recuperar a resposta daquela tentativa. Repetir mesma chave e entrada normalizada devolve o comprovante original, sem criar outro atendimento; entrada diferente retorna 409. Janela de repetição: 24 horas; chave expirada retorna 409 e não é reutilizada automaticamente. Registros expirados permanecem como proteção contra reuso; ainda não existe limpeza automática.

A chave não é gravada em texto puro. O banco guarda seu hash e um comprovante cifrado AES-GCM, cuja chave deriva do segredo da requisição. O fingerprint do corpo é HMAC com a chave da tentativa. A resposta original pode ter status antigo após cancelamento; o frontend consulta o código novamente para mostrar o estado atual.

Após falha ambígua de rede/servidor, o formulário mantém chave e entrada em memória, impede edição e permite repetir a confirmação. Não fechar/recarregar nessa situação: há aviso, mas não persistência da tentativa após recarga forçada. Sem comprovante e sem a tentativa, consultar a barbearia antes de reservar novamente.

Criação, cancelamento e alterações de calendário compartilham bloqueio da linha do profissional. Depois de obter o lock, o servidor reconsulta disponibilidade. A restrição PostgreSQL `appointments_no_overlap` impede qualquer sobreposição de intervalos não cancelados para o mesmo tenant/profissional, inclusive em escritas SQL diretas. Conflito vira 409 `slot_unavailable`. Uma transação que falha não deixa cliente novo ou reserva parcial.

A migration 00005 habilita RLS e revoga acesso de PUBLIC/anon/authenticated às tabelas de negócio, inclusive shops/services. O Go necessita uma conexão servidor autorizada; o navegador nunca acessa essas tabelas diretamente. Regras de expediente/bloqueios exigem o fluxo Go e o protocolo comum de locks: a exclusão SQL protege sobreposição entre reservas, não substitui toda validação de negócio.

## Erros e limites

Erros: `{"error":{"code":"...","message":"..."}}`; sem SQL, dados pessoais ou detalhes internos.

- 400: JSON inválido, campos desconhecidos ou objeto adicional.
- 401/403: autenticação/acesso administrativo ou origem CORS não permitida.
- 402: assinatura necessária para nova reserva administrativa feita por owner;
  barber recebe 403 com `shop_subscription_required`, sem acesso à cobrança.
- 404: unidade/opção/reserva não encontrada no escopo solicitado.
- 409: horário ocupado, conflito/expiração de idempotência ou cancelamento encerrado.
- 413/415: corpo excessivo ou Content-Type diferente de application/json.
- 422: campos de entrada inválidos.
- 429: limite de requisições; Retry-After 60.
- 500/503: falha inesperada ou indisponibilidade/deadline.

Booking limita corpos a 8 KiB. As respostas não devem ser armazenadas em cache. CORS permite a origem exata FRONTEND_ORIGIN e headers Authorization, Content-Type e Idempotency-Key; OPTIONS não autentica. Limites básicos são locais ao processo: 20 POST/IP/minuto, 120 GET/IP/minuto e 1000 requisições/minuto, usando RemoteAddr sem confiar em X-Forwarded-For arbitrário. Produção com proxy/múltiplas instâncias precisa política de edge e proteção contra abuso.

Criação pública e administrativa aceita o campo opcional `whatsapp_notifications`
(boolean, padrão false). Ele registra autorização transacional para confirmação e
lembrete daquela reserva; no fluxo administrativo, o funcionário declara que
coletou a autorização. Não implica marketing ou comprovação de posse do telefone.
A autorização participa da identidade da requisição: mudar o campo usando a mesma
Idempotency-Key retorna conflito. Omissão/false preserva os fingerprints anteriores.

Com `NOTIFICATION_PLANNING_ENABLED=true`, configuração de chaves válida e migration
00021 aplicada, o backend persiste permissão e plano na mesma transação da reserva.
Somente política habilitada da unidade e autorização concedida geram jobs com
conteúdo cifrado. Sem essas condições, a reserva funciona com plano dispensado.
A chave global desligada mantém o fluxo anterior, sem acessar as tabelas novas.
Replays não criam nem reconstroem notificações retroativas.

Cancelamento, ausência, conclusão, revogação e desativação da política invalidam
jobs pendentes por triggers PostgreSQL. Estas proteções independem da chave global
de planejamento. Persistência de jobs não significa envio: scheduler/outbox,
consumo RabbitMQ e entrega pela Evolution ainda não estão implementados.

## Conexão WhatsApp da unidade — entrega C

GET /api/v1/admin/shops/{slug}/whatsapp/connection, POST /whatsapp/connect e
POST /whatsapp/disconnect usam autenticação, acesso à unidade e owner. Revalidam
usuário/membership/loja no banco. Os POSTs recebem corpo vazio ou objeto vazio.
Nenhum aceita instanceName, shop_id, URL ou chave do navegador. Status retorna
state, desired_connected, stale, provider_configured, verified_at,
last_connected_at e error_code sanitizado.

Connect retorna {connection,qr}; qr é null ou {png,discard_after}. A imagem PNG
temporária é exclusiva do owner, com no-store/no-referrer, sem persistência/log.
Autorização é revalidada após I/O antes de devolver QR. Não há GET /whatsapp/qr.
409 connection_busy informa lease em andamento; 503 evolution_activation_required
indica ativação necessária pelo operador, sem conceder acesso ao provider.

POST /api/v1/webhooks/evolution só é habilitado com a integração configurada.
Header X-BarberChat-Evolution autentica a instância com segredo derivado por HMAC;
não é assinatura do corpo. O evento connection.update apenas marca rechecagem
durável, sem confiar no estado/apikey do JSON. Corpo limitado a 64 KiB.
O reconciliador consulta ativamente a instância, recupera leases e mantém último
estado desejado. Connected com mais de dois minutos sem verificação vira
unknown/stale na resposta. Conexão não significa mensagem enviada/entregue.

Detalhes e comandos: [Evolution local](evolution-local.md).
