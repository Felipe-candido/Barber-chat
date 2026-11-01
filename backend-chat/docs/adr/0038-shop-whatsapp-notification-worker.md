# 0038 — Notificações por WhatsApp da barbearia

Data: 2026-10-31; atualização: 2026-11-01. Entregas A e B implementadas (regras,
proteção do payload e persistência transacional). Ver [ADR 0039](0039-durable-booking-notification-plans.md).
Worker de transporte e integração Evolution ainda não implementados.

## Contexto

O usuário solicitou confirmação com informações e código da reserva, lembrete
com antecedência configurável e monitoramento da conexão via Evolution API.
Escolheu um número por barbearia, configurado pelo proprietário, e informou que
ainda precisa de ambiente local para o provider.

O backend já tem notifications como fronteira de módulo, transação de booking,
pool PostgreSQL e cliente AMQP. O worker executável apenas verifica dependências.
O código privado não pode ser reconstruído a partir de seu hash; o recibo de
replay é cifrado com a chave efêmera da requisição do cliente.

## Decisão

1. Manter um módulo Go e dois processos: API e worker. Notifications é dono do
   planejamento e entrega; booking mantém a reserva e customers a permissão.
   Não separar microsserviço/repositório nem adicionar Python.
2. Um remetente Evolution por shop_id, gerenciado por owner. Slug não identifica
   a instância remota. Isolamento, autorização e mapeamento ficam no backend.
3. Confirmação após commit e um lembrete com padrão de 1.440 minutos decorridos.
   Antecedência por unidade, inicialmente 5–10.080 minutos. Novas políticas valem
   para novas reservas; desativação/revogação bloqueia pendências existentes.
4. Reserva dentro da janela de antecedência recebe apenas confirmação. Limitar
   validade das mensagens para não enviar avisos antigos após recuperação.
5. Planejamento e invalidação compartilham a transação de booking, por portas
   declaradas no módulo consumidor e adaptadores ligados às mesmas Queries.
   Não enviar HTTP nem publicar AMQP no handler ou antes do commit.
6. Snapshot cifrado independente, com chave servidor/AAD e retenção limitada,
   permite ao worker incluir o código sem alterar a proteção do recibo atual.
   Fila/logs carregam IDs e metadados seguros, sem código, telefone ou corpo.
7. PostgreSQL é fonte de verdade para jobs, outbox, tentativas e resultados.
   RabbitMQ é transporte, mantendo a decisão do ADR 0002. Usar confirms,
   mandatory, ACK manual após persistência, deduplicação, leases e retries finitos.
8. Sem garantia de idempotência externa comprovada, timeout após possível envio
   produz unknown e reconciliação; não repetir automaticamente. Não prometer
   exactly-once. Cancelamento antes do início impede envio; tentativa em andamento
   já pode chegar ao cliente e não pode ser retirada por rollback local.
9. Conexão, aceitação, entrega/leitura e situação do atendimento são estados
   diferentes. Webhooks autenticados e reconciliador ativo mantêm conexão fresca.
   QR code é efêmero e restrito ao proprietário. Frontend não conhece API keys.
10. Preparar Evolution com PostgreSQL/Redis/volumes próprios no ambiente local.
    Revisar imagem/dependências corrigidas e fixar versão/digest antes de parear
    telefone; não adotar latest nem recomendar automaticamente a release 2.3.7,
    associada ao aviso de segurança Baileys citado no plano.

## Consequências

A reserva continua disponível com broker, worker ou WhatsApp indisponíveis. Se
uma intenção obrigatória não puder ser persistida, a transação inteira falha;
isso evita reservar silenciosamente sem registrar o trabalho esperado.

Passamos a operar três preocupações independentes: consistência da reserva,
transporte de tarefas e sessão externa do WhatsApp. Aceitar uma tarefa no broker
não significa entregar uma mensagem ao cliente. Serão necessários painel de
falhas, métricas e testes de recuperação antes da ativação geral.

Permissão transacional não implica marketing e não exige criar um CRM. Não
habilitar disparo retroativo para reservas antigas. Respostas automáticas, IA,
campanhas, AWS e novos canais ficam fora desta entrega.

## Sequência e validação

O [plano de implementação](../notification-worker-implementation-plan.md) detalha
entregas A–G, contratos propostos, critérios de aceite e fontes oficiais. Começar
pelas regras/contratos e proteção do payload; depois persistência atômica,
ambiente/conexão Evolution, transporte com adaptador falso, envio real, painel e
recuperação ponta a ponta. Não criar interfaces sem consumidor nem dezenas de
stubs para representar o plano.

A entrega A adiciona cálculo puro de janelas, DTOs internos, uma porta de
cifragem com consumidor concreto e o adaptador AES-256-GCM. O snapshot protegido
é independente do recibo idempotente. Metadados autenticados usam expiração com
precisão de microssegundos para sobreviver ao futuro round trip no PostgreSQL.

A margem final de lembrete foi ajustada para 1min antes do serviço: com 5min de
antecedência mínima há 4min de janela útil, evitando a janela vazia do plano
inicial. Expiração é exclusiva e não gera envio atrasado. Notificações continuam
desligadas por padrão. A entrega B conecta o bootstrap e adiciona a migration
00021 e a opção de autorização no POST, conforme ADR 0039. Não há envio real;
a próxima entrega é o ambiente e a conexão Evolution.
