# ADR 0020 — Estado comercial e publicação explícita

Status: backend implementado e validado localmente. Data: 2026-10-30.

## Responsabilidades

Shops possui a intenção de publicação e o caso de uso ShopPublication. Billing
mantém a política financeira e fornece somente status/plano/cobertura/motivo;
catalog projeta os profissionais ativos vinculados a serviços elegíveis; booking
verifica se uma janela recorrente comporta a duração arredondada pela SlotPolicy.
Identity revalida proprietário ativo. Portas são definidas por shops/application;
adaptadores concretos de cada módulo e composição em cmd/api evitam imports
entre camadas de negócio de módulos diferentes. Não é necessário microsserviço.

GET commercial-status expõe um snapshot local, sem dados pessoais de cobrança,
IDs externos ou chamadas Stripe por navegação. A projeção financeira reaproveita
EvaluateEntitlement e o ledger de faturas do ADR 0019; não duplica suas regras.
O plan_key serve como identificador local, não como preço para exibição. Preço
comercial validado para o frontend ainda é trabalho da etapa de interface.

PUT publication recebe estado desejado explícito. Não alternar booleano evita
uma repetição inverter a ação. Publicar exige endereço válido, serviço elegível,
profissional vinculado e expediente compatível, além da cobertura. Bastam um
profissional/serviço utilizáveis; não é necessário que todo cadastro esteja pronto.
A menor duração elegível de cada profissional é suficiente para testar a
existência de alguma combinação utilizável, sem N+1 de queries.

Não confundir prontidão com disponibilidade atual: feriados, bloqueios e reservas
podem ocupar datas sem invalidar a configuração recorrente. Cada reserva ainda
verifica data, timezone, horário e conflito no fluxo de booking. Mudanças futuras
de catálogo/expediente não apagam a intenção de publicação automaticamente.

Despublicar não depende de cobrança nem de prontidão e não altera assinatura,
reservas ou permissões. Settings permanece uma substituição dos dados básicos;
publicação não participa de settings_version e não pode ser escrita naquele PUT.

## Consistência e concorrência

Usar transação SERIALIZABLE para que leituras de cadastro/catálogo/expediente e
ledger componham uma visão coerente, sem transações financeiras externas. Owner
é revalidado sob locks de usuário e membership até o commit. Apenas a escrita de
publicação obtém FOR UPDATE na unidade, antes da leitura financeira; não converter
um lock compartilhado de shop em exclusivo. Ordem compatível com Checkout:
identidade, shop, conta financeira. GET não obtém lock exclusivo da unidade.

O adapter de billing obtém FOR SHARE na conta existente. O webhook usa FOR UPDATE
e avança sua revisão, portanto uma atualização financeira concorrente não pode
ser ultrapassada por publicação com snapshot antigo: pode haver serialization
failure e 503. Sem criar conta durante consulta e sem provider I/O sob lock.

Configuração/assinatura podem mudar depois do commit. Serialização define uma
ordem válida, não congela o negócio indefinidamente. O guard público continua
checando intenção, atividade e cobertura em cada requisição; expiração não
depende de timer. Dados já salvos continuam consultáveis/canceláveis.

Não repetir callback automaticamente. Após falha/rede/commit incerto, consultar
estado antes de nova decisão. Intenções simultâneas opostas são serializadas;
uma ação posterior pode substituir a anterior. Não há garantia de CAS para
publicação nem retry silencioso sobre a intenção de outro proprietário.

## Compatibilidade e limites

Sem migration nova: reutilizar publication_requested da 00009 e esquema até 14.
Não modificar nem aplicar migrations no Supabase nesta etapa. Unidades manuais
continuam com acesso legado, não são cobradas nem ocultadas automaticamente.
GET informa publication_managed=false/not_required; PUT retorna 409 para esse
modo. Uma futura migração comercial dessas unidades exige decisão do operador.

Fluxo TEST: configurar → Checkout → webhook confirma cobertura → proprietário
publica → guard público libera. Pagamento inicial não publica sozinho. Perder
cobertura oculta o acesso sem apagar intenção; renovação pode reabilitar quem
continuou publicado. Despublicação explícita prevalece sobre renovação.

Frontend, trava comercial do agendamento manual, Portal e modo live permanecem
fora desta entrega. A integração externa com a conta Stripe também está pendente;
as verificações financeiras desta etapa usam fixtures em PostgreSQL descartável.

## Verificação

Testes unitários de política/coordenador e testes HTTP/PostgreSQL para estados,
roles, tenant, corpo estrito, ledger vazio/inelegível, duração de 80 minutos em
grade de 60, compatibilidade manual, proprietário revogado, falha real de commit,
escritas concorrentes e corrida com reconciliação financeira. Regressões de
Checkout/webhook/guard público executadas separadamente.
