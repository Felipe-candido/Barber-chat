# 0032 — Conclusão automática e revisão de exceções

Data: 2026-10-31. Estado: entrega F implementada e testada localmente.
Sucede a exigência de conclusão exclusivamente manual do ADR 0014.

O usuário informa que a maioria dos clientes comparece e prefere registrar somente
exceções. Concluir automaticamente reservas scheduled/confirmed depois de ends_at,
o término do bloco reservado; não concluir ao iniciar nem alterar a confirmação
da reserva. Identificar a origem como automática, sem atribuir ação a um usuário.

Ausência/cancelamento permanecem ações explícitas. Permitir corrigir uma conclusão
automática, com motivo, ator, instante, estado anterior/novo e versão. Owner opera
na unidade, barber somente no próprio profissional. Regras públicas de cancelamento
não são ampliadas por essa mudança administrativa.

Executar no servidor, sem depender do frontend e sem escrever durante consultas.
O executor recupera atrasos/interrupções a partir de estado e prazo
persistidos, processa de forma idempotente e respeita decisões manuais concorrentes.
O marco de ativação foi fixado na migration; não promover todo o passado legado
para completed em massa. Booking continua dono das transições.

Reporting usa o estado final atual e mantém a distinção da origem automática:
cancelled/no_show não entram em valor realizado. Snapshots e histórico permanecem.
Comparecimento presumido pode superestimar indicadores se a equipe não registrar
exceções; facilitar a conferência dos atendimentos do dia. Valor dos serviços não
equivale a dinheiro recebido. Vendas e devoluções de produtos continuam independentes.

Validação realizada: prazo futuro, fronteira ends_at, execução
repetida/atrasada, job concorrendo com ausência/cancelamento/correção, isolamento
de unidade/profissional, auditoria e snapshots após retificação. Agregados de
reporting continuam na entrega G. A migration 00019 foi testada em bancos locais
descartáveis, sem aplicação no Supabase nesta etapa.

## Implementação

O ciclo de 15 segundos roda junto da API, com contexto/timeout e encerramento
coordenado antes de fechar o pool. Não depende do navegador nem de RabbitMQ.
Foi escolhido para tornar a entrega utilizável com go run ./cmd/api, mantendo
o worker de mensagens como etapa posterior. O caso de uso AutoCompleteAppointments
e sua porta podem ser compostos no worker no futuro sem mover regras para HTTP.

SQL usa candidatos vencidos e locks em profissionais antes de appointments,
seguindo o protocolo dos demais escritores; SKIP LOCKED permite múltiplas réplicas.
Estado é revalidado antes do UPDATE e trigger confere término/marco de ativação.
Uma chamada conclui até 100 reservas e registra auditoria na mesma transação SQL.
Interrupções deixam o restante elegível na próxima execução. Não há escrita em GET.

O marco é gravado na aplicação da migration e não pode ser alterado casualmente.
Novos registros começam scheduled/version=1. Registros existentes recebem origem
legacy sem reescrever situação, valores ou datas; fim anterior ao marco não recebe
baixa automática. Ends_at futuro de reserva antiga continua elegível normalmente.

Mudanças staff usam motivo, versão, chave HMAC e ator autenticado, com transação
compartilhada por booking e identity. Conta principal owner pode gerenciar todos
os profissionais da unidade; barber apenas seu profissional ativo. Inatividade
do profissional não impede o proprietário de corrigir seu histórico. Perda de
cobertura comercial não bloqueia correções. Não confiar no perfil do frontend.

Snapshot e código não mudam durante a transição; eventos são gerados pelo trigger
e não podem ser inseridos, editados ou apagados como histórico solto. Claims
incompletos não podem confirmar. Repetição da mesma chave aponta ao evento original,
mas devolve também a situação atual, preservando correções posteriores.

Correção para completed exige fim reservado atingido e para no_show exige início;
cancelled administrativo permite corrigir o passado. Não oferecer restauração
para scheduled/confirmed: uma nova reserva futura segue o fluxo normal. Restaurar
intervalo cancelado para realizado/ausência respeita a exclusão existente e pode
ser recusado quando outra reserva já ocupa o intervalo. Down recusa apagar auditoria.

Agenda traz metadados e o painel usa consulta atual antes de escrever. Histórico
mostra as últimas 100 transições e avisa truncamento. Pedido incerto fica em
sessionStorage por unidade/ator/atendimento; retry é explícito, sem nova chave.
Troca de foco não fecha a correção aberta e Escape não abandona uma escrita pendente.
Frontend usa metadados opcionais para leitura de respostas antigas, mas gestão
de situação exige a API atual e a migration; não inferir realização por relógio local.
