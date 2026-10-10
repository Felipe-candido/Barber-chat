# ADR 0010 — Contratos iniciais de booking e transação compartilhada

Status: contratos adotados; comportamento e adapters ainda não implementados. Data: 2026-10-02.

## Decisão

Iniciar o módulo existente `booking` com contratos somente para `ListAvailability`
e `CreateAppointment`. Não criar CRUD completo, handlers vazios ou outro módulo
para os mesmos atendimentos. O domínio declara atendimento, estados e snapshot
de serviço; suas validações e transições serão implementadas depois.

DTOs separam a entrada pública do resultado. O cliente solicita serviço,
profissional e início; preço, duração, fim, estado e IDs internos vêm do servidor.
Contatos e consentimentos não aparecem no recibo público. A data da consulta é
civil no timezone IANA da unidade; os instantes dos intervalos são UTC.

Booking define suas próprias projeções e portas. Shops continua dono do
slug/timezone; catalog, dos serviços/profissionais e associações; customers,
dos contatos/consentimentos; notifications, dos jobs/outbox. Cliente público
não é um usuário administrativo do Supabase, e telefone informado não prova
identidade. Um adapter pode traduzir contratos entre módulos sem importar seus
domínios para `booking/application`.

`BookingTransactor` recebe um callback com `BookingTx`, conjunto específico de
capacidades para a criação. A composição fornece adapters vinculados à mesma
transação, incluindo leituras de shops/catalog, idempotência, agenda, cliente,
atendimento e planejamento de notificações. O caso de uso não recebe `pgx.Tx`.
Sucesso só pode ser retornado após commit; o callback não é repetido implicitamente.
Usar o pool fora da transação não preserva essa unidade de trabalho, conforme
as [orientações oficiais de transações em Go](https://go.dev/doc/database/execute-transactions).

Idempotência exige claim atômico por unidade/chave/operação, fingerprint da
entrada validada, retenção explícita e recibo estável salvo junto ao atendimento.
Repetição equivalente recupera esse recibo; reutilização incompatível conflita.
Falha de commit pode deixar o resultado incerto: o cliente deve repetir a mesma
chave, não gerar outra. Não armazenar corpo bruto com dados pessoais.

Disponibilidade é uma observação, não reserva. A criação deve usar o lock comum
da agenda, revalidar catálogo/intervalo e inserir com proteção SQL contra
sobreposição. Referências compostas devem impedir associação entre tenants.
O [PostgreSQL documenta exclusion constraints em ranges](https://www.postgresql.org/docs/18/rangetypes.html#RANGETYPES-CONSTRAINT)
para impedir intervalos conflitantes. Toda escrita futura de expediente,
bloqueios e atendimentos precisa seguir o mesmo protocolo de lock.

Planejar notificações significa persistir intenções/jobs/outbox, nunca publicar
no broker ou enviar mensagem antes do commit. Sem consentimento transacional,
a reserva pode existir sem jobs. Consentimento de marketing é independente.

## Consequências e próximos passos

Os contratos não garantem invariantes por si só. Implementar regras de domínio,
profissionais/associações, contatos/consentimentos, agenda, novas migrations,
adapters transacionais e testes concorrentes em PostgreSQL real antes de expor
o fluxo. O teste de arquitetura protege imports, não a integridade dos dados.

Na futura integração HTTP, adicionar `Idempotency-Key` ao cliente e à lista de
headers permitidos pelo CORS; hoje não há endpoint de criação de atendimento.
Nenhuma migration, conexão, endpoint ou dependência nova é necessária para esta
etapa de contratos. Administração de agenda, cancelamento e relatórios ficam
para incrementos com consumidores reais.
