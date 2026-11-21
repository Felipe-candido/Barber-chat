# ADR 0051 — Folgas e férias por barbeiro

Status: aceito e implementado em 19/11/2026.

A disponibilidade já consultava `schedule_blocks`, mas o painel não permitia
administrar esses períodos. A agenda agora oferece “Folgas e férias”. O
proprietário escolhe o profissional; cada barbeiro administra somente as próprias
folgas. O expediente semanal do cadastro permanece inalterado.

Datas inicial e final são inclusivas, com dias completos no fuso da barbearia.
Os limites persistidos são instantes UTC em intervalo semiaberto. A conversão
considera horário de verão, inclusive mudanças à meia-noite. É possível bloquear
até 366 datas civis, de hoje em diante, dentro dos próximos dois anos. A lista
mostra períodos em andamento e futuros; bloqueios antigos permanecem no banco.

As escritas usam a mesma linha de profissional e o mesmo `FOR UPDATE` dos
agendamentos. Só depois do lock revalidam usuário/membership e verificam conflitos.
Não se bloqueia um período com atendimentos não cancelados nem outro bloqueio.
O trigger existente reforça a proteção contra reservas concorrentes. Nenhum
agendamento é alterado ou cancelado automaticamente. A listagem não publica os
motivos pessoais das ausências e não exige assinatura ativa.

PUT recebe um UUID gerado uma única vez no navegador. Mesmo ID e datas recuperam
a resposta original; dados diferentes exigem outro recurso. Isso permite resolver
falhas de rede sem duplicar períodos. DELETE é idempotente e restrito à mesma
unidade e profissional. Não há edição de um bloqueio: remover e cadastrar outro
passa novamente pelas regras de disponibilidade.

Não é necessária migration: os dados usam a tabela e os triggers já existentes.
Os testes reais de PostgreSQL cobrem disponibilidade pública, reservas impedidas,
remoção, permissões por unidade/profissional, revogação, conflitos e quatro disputas
concorrentes. Os testes de domínio cobrem dias de 23/25 horas e a mudança à
meia-noite. Os testes de navegador cobrem o formulário, confirmação de remoção,
escopo, falhas e recuperação usando o mesmo ID.
