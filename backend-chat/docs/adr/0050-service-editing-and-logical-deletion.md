# ADR 0050 — Edição e exclusão lógica de serviços

Status: aceito e implementado em 18/11/2026.

O catálogo permitia apenas cadastrar e listar serviços ativos. O proprietário agora
pode editar nome, descrição, preço e duração, ou excluir um serviço mediante
confirmação no painel. Barbeiros continuam com acesso de leitura ao catálogo.

As rotas PUT e DELETE usam autenticação, autorização por slug e OwnerOnly antes de
ler os dados. Toda escrita filtra simultaneamente service ID e ShopID autorizado,
com barbearia ativa. IDs de outra unidade são tratados como não encontrados.

Excluir significa marcar `active=false`. Isso preserva FKs, vínculos profissionais
e snapshots dos atendimentos existentes, e retira a opção das consultas usadas em
novos agendamentos. Repetir DELETE é seguro; PUT só altera serviços ativos e nunca
reativa um serviço excluído. Cada operação usa uma instrução SQL atômica.

Não há migration nem alteração da duração, preço ou nome de reservas já feitas.
Edições simultâneas seguem a última gravação, sem versão otimista nesta etapa.
Reativação e visualização de serviços excluídos permanecem fora deste incremento.

Os testes de HTTP cobrem validação, escopo, permissões e preflight. O teste
PostgreSQL `TestServiceManagementPreservesAppointments` cobre isolamento, remoção
do catálogo, repetição da exclusão e preservação dos comprovantes existentes.
