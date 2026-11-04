# 0046 — Diretório de contatos por barbearia

Data: 2026-11-02. Status: implementado.

Expandir customers e a tabela existente, sem novo módulo, serviço ou dependência.
Contatos é o nome da seção porque um cadastro manual pode existir sem reserva.
Owner e barber consultam/cadastram todos os contatos da unidade por decisão do
usuário. Não ampliar permissões da agenda, relatórios ou mensagens.

ManageContacts coordena Create/Get/List. Portas próprias tratam normalização,
acesso e persistência em transação curta. Identity revalida membership, usuário e
loja ativos com locks compartilhados; tenant vem do contexto, não do body.

UNIQUE(shop_id,phone_e164) arbitra concorrência entre booking e cadastro manual.
Manual retorna conflito sem trocar nomes; booking continua INSERT ON CONFLICT
DO NOTHING dentro de sua transação. Um telefone compartilhado representa um
contato, sem comprovar identidade ou posse. Não compartilhar entre barbearias.

Sobrenome opcional só no cadastro manual. Migration 29 ajusta a constraint e
adiciona índice de paginação. Projeções de nome em agenda/lifecycle/reporting
usam btrim; booking mantém seus campos obrigatórios. Down recusa perder contatos
sem sobrenome. Não precisa importar registros antigos.

Paginação por created_at/id; cursor vinculado à unidade e hash dos filtros, sem
nome/telefone em claro. Ele não é autorização nem segredo. Busca por nome literal
e telefone exato normalizado; queries parametrizadas e RLS preservada.

Após resposta perdida, bloquear POST e consultar telefone. Registro encontrado
exige conferência; ausência permite revisão explícita. Não armazenar PII em
storage/logs nem repetir comandos automaticamente. Cadastro não cria reserva,
mensagens, permissão de WhatsApp, login ou assinatura.

Edição, telefone, arquivamento, seleção no booking e histórico por contato ficam
para extensões. Não são pré-requisitos da lista/cadastro entregues.

Validação: PostgreSQL real para concorrência, rollback, cursor, isolamento e
revogação; HTTP para autenticação/CORS/DTOs; navegador para ambos os papéis,
busca, cadastro, conflito, resposta perdida e mobile. Banco real do SaaS não é
usado pelos testes. Publicação exige migration antes de atualizar API/frontend.
