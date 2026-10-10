# ADR 0011 — Grade configurável e ocupação por blocos

Status: regra de domínio implementada; persistência e fluxos HTTP pendentes. Data: 2026-10-02.

## Contexto e decisão

O profissional atende em uma grade, por exemplo 09:30, 10:30, 11:30, mesmo
quando um corte dura aproximadamente 40 minutos. Serviços maiores podem
ocupar vários horários. Separar duração estimada do catálogo e ocupação da
agenda no módulo `booking`.

`SlotPolicy` recebe intervalo e margem mínima adicional em minutos. Não fixa
60 minutos globalmente: a futura configuração efetiva será resolvida por
unidade/profissional. Para o exemplo inicial, usar intervalo 60 e margem 0.
O arredondamento já deixa 20 minutos livres para um serviço de 40 minutos.

O cálculo é `ceil((duração + margem) / intervalo) * intervalo`, usando inteiros.
Um serviço de 80 minutos ocupa 120 numa grade de 60; numa grade de 45, ocupa 90.
A margem é adicionada antes do arredondamento: serviço de 60 com margem 10
ocupa 120 na grade de 60. Não inserir outro cliente no restante do bloco.

Os inícios partem da abertura de cada período de expediente, não de meia-noite.
Períodos de manhã/tarde podem ter aberturas próprias; pausas internas deverão
filtrar intervalos candidatos sem reiniciar a grade do mesmo período. A reserva
inteira precisa caber antes do fechamento. Candidatos podem se sobrepor enquanto
nada estiver reservado; depois, a ocupação remove os candidatos conflitantes.

`ServiceSnapshot.DurationMinutes` conserva a duração estimada. `Appointment.EndsAt`
e os intervalos de disponibilidade representam o fim da ocupação reservada.
Guardar esse fim no atendimento, sem recalculá-lo quando a política mudar.
A futura exclusion constraint deverá cobrir toda essa ocupação, não apenas
os minutos estimados do serviço.

`ScheduleGuard.FindSlotPolicy` declara a leitura transacional da configuração
efetiva, sob o lock comum da agenda. Ausência de configuração não deve criar
silenciosamente uma política global nem usar a de outro tenant. Alterar a
política deverá seguir o mesmo protocolo de lock das reservas e bloqueios.

## Limites desta etapa

Os cálculos são de minutos civis em períodos dentro de um mesmo dia: abertura
entre 0 e 1439 e fechamento até 1440 (meia-noite seguinte). Intervalo e duração
reservável do serviço ficam entre 1 e 1440; margem entre 0 e 1440. Uma ocupação
arredondada que não cabe no período retorna lista vazia, não disponibilidade.

Não há conversão de timezone, expediente persistido, filtragem de ocupações,
tratamento de DST ou proteção contra reservas concorrentes implementados aqui.
Ao converter futuros candidatos para UTC, resolver explicitamente horários
civis inexistentes/ambíguos e conferir o intervalo real completo. Não truncar
segundos da entrada pública para fazê-la parecer alinhada à grade.

## Continuidade do produto

O código de consulta solicitado será vinculado a um único agendamento, sem
conta ou sessão administrativa. Consultar/cancelar por esse código e enviar
informações pelo WhatsApp continuam requisitos das próximas partes, não
comportamento implementado por SlotPolicy. Reservas e planejamento durável
de mensagens deverão compartilhar a transação; envio externo ocorrerá depois.
