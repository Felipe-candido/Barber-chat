# ADR 0052 — Edição de barbeiros e expediente semanal

Status: aceito e implementado em 21/11/2026.

O proprietário agora edita nome, ativação, serviços habilitados, dias, períodos
de atendimento, intervalo entre inícios e margem adicional pela tela Equipe.
Dias podem ter horários diferentes e mais de um período para representar pausas.
O contrato permite até 28 períodos semanais; a interface oferece até quatro por
dia. Fechamento 1440 representa 24:00, sem estender o período ao dia seguinte.

Catalog coordena o perfil e as associações; booking lê e substitui somente a
política e os horários semanais por uma porta transacional. Identity revalida o
proprietário. Os adapters compartilham a mesma transação e o lock da linha do
profissional, também usado por reservas, folgas e alterações de contas.

Uma revisão opaca deriva dos valores editáveis ordenados. PUT exige a revisão
consultada, impedindo sobrescrita de alterações concorrentes. Um resultado já
idêntico ao corpo enviado pode ser recuperado com a revisão anterior, sem executar
novas escritas. Perfil, serviços e expediente são gravados ou revertidos juntos.

Serviços novos precisam estar ativos e agendáveis na mesma unidade. Associações
antigas podem ser mantidas mesmo após o serviço sair do catálogo, ou removidas;
isso não volta a oferecer serviços inativos. É permitido retirar todos os serviços
ou horários para pausar novas opções de agendamento. Desativar o profissional
também impede acesso pela membership de barbeiro, preservando a conta para
reativação. Proprietários continuam acessando e editando profissionais inativos.

Reservas, preços/duração/nomes históricos, folgas e contas nunca são apagados por
esta edição. Reservas existentes fora do novo expediente permanecem na agenda;
não há cancelamento ou remanejamento automático. A interface informa essa regra.

Não há migration: a edição usa as tabelas, FKs e triggers já existentes. Testes
PostgreSQL verificam persistência, novas disponibilidades, pausas, snapshots e
folgas preservados, permissões, revogação, serviços de outra unidade, validações,
rollback após escritas parciais, recuperação de resposta perdida e concorrência.
