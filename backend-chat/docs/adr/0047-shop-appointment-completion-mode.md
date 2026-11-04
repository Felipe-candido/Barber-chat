# Fechamento automático ou manual por barbearia

Data: 03/11/2026. Status: implementado.

## Decisão

Booking controla o estado dos atendimentos e também sua política de fechamento.
Usar uma tabela 1:1 com shops e endpoint próprio, evitando acoplar a mudança ao
cadastro/endereço da unidade, ao faturamento ou às configurações de mensagens.
Identity implementa a porta de acesso com locks dos vínculos atuais. Somente o
proprietário com vínculo ativo consulta e altera essa opção para toda a unidade.
O barbeiro continua finalizando seus próprios atendimentos na agenda, sem receber
acesso à tela Configurações, ao cadastro, à assinatura ou ao WhatsApp do proprietário.

A migration 30 preenche todas as lojas existentes com automatic e cria a mesma
configuração para novas lojas por trigger. Preserva o corte global da migration
19 para manter a elegibilidade anterior. Manual não altera estados já concluídos:
apenas impede novas conclusões automáticas de scheduled/confirmed.

Manual -> automatic usa clock_timestamp como novo automatic_since. Horários que
terminaram antes dessa ativação permanecem em aberto para revisão manual. Salvar
automatic sem trocar o modo preserva o corte. Não reabrir atendimentos, fabricar
histórico ou concluir um passivo antigo em lote.

## Transações e concorrência

PUT usa compare-and-swap de version e revalida acesso dentro da transação. Ator,
unidade e função não vêm do JSON. Conflito retorna 409. Resposta perdida requer
GET antes de outra escrita: não repetir automaticamente o PUT com nova versão.

O processo automático toma FOR SHARE na política antes de bloquear profissionais
e atendimentos, com SKIP LOCKED. Revalida modo e corte sob esse lock. A atualização
da política conflita com o lock e, após o commit do modo manual, não sobra uma
conclusão automática autorizada por uma leitura antiga. Uma conclusão que já
detinha o lock pode terminar antes de a troca de modo ser confirmada.

Um trigger adicional em appointments exige modo automático e ends_at >= corte,
inclusive para tentativas diretas no banco ou versões antigas do processo. Mantém
as validações de transição, snapshots, exclusão de horários e auditoria existentes.
A tabela tem RLS e não concede acesso aos papéis anon/authenticated da Data API.

## Fechamento, produtos e relatórios

Os comandos de situação já existentes continuam com motivo, versão, idempotência
e trilha de auditoria. Realizado exige fim do horário reservado; ausência exige
início. Owner atua em qualquer atendimento; barber apenas no profissional próprio.
Produtos continuam sendo vendas vinculadas ao atendimento, com suas transações de
estoque. Não misturar persistência de venda e alteração de situação em um comando
novo. Falha em uma operação não deve induzir repetição automática da outra.

Relatórios reconhecem valor de serviço por status completed, sem inferência pelo
relógio. Uma reserva manual em aberto permanece fora desse valor; venda vinculada
continua contada separadamente. Ausência/cancelamento não anulam vendas de produtos.

O rollback da migration recusa remover a proteção após qualquer alteração de
configuração. Não fazer downgrade que reative silenciosamente o automático.
Aplicar a migration antes de publicar a nova API. Não há migração automática no
bootstrap nem dependência do worker de WhatsApp para esse processo.
