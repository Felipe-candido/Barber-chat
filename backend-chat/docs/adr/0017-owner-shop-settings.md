# ADR 0017 — Configurações básicas, autorização transacional e versão

Status: API e frontend implementados. Data: 2026-10-29.

Configurações pertencem a shops. Identity continua dono das regras de usuário e
membership; implementa uma porta pequena consumida por shops, composta em cmd/api.
Domain/application não importam outro módulo. O parser de telefone já instalado
é uma biblioteca pura, sem I/O, não um serviço de Auth ou envio de mensagens.

GET/PUT /api/v1/admin/shops/{slug}/settings são exclusivos do proprietário. O
middleware deriva usuário e tenant do token/slug. A transação revalida usuário e
membership ativos e mantém seus locks até o commit. Esse guard não bloqueia shops:
adquirir FOR SHARE da unidade antes de tentar atualizá-la causaria conversão de
lock/deadlock em duas edições concorrentes. A escrita condicional da unidade
adquire seu próprio lock e revalida active e settings_version no PostgreSQL.

Nome, telefone comercial, descrição e endereço são editáveis. Slug e fuso apenas
consultáveis: troca de link ou calendário exige uma política específica. O payload
não aceita IDs, role, origem de provisionamento, publicação ou estado financeiro.
Configurar não exige nem concede assinatura. Não há Stripe nesta entrega.

Migration 00012 reutiliza shops.phone, acrescenta description e settings_version
e mantém endereços na tabela da migration 00011. Nenhum dado legado é normalizado
ou completado silenciosamente. GET permite address=null no legado; PUT exige
endereço completo. Não afeta snapshots ou horários dos agendamentos existentes.

PUT é substituição explícita: version, name, phone, description e address são
obrigatórios, com strings vazias para limpar os campos opcionais. A versão evita
lost update: uma edição concorrente vence e a outra recebe 409. Nome/contato e
endereço são atômicos. Falha de commit não informa sucesso. Após resposta incerta,
GET e revisão do estado, não retry automático com uma versão nova. Não há outbox
nem efeitos externos que demandem uma nova chave idempotente neste fluxo.

Todas as edições pela API usam o protocolo de versão; SQL administrativo direto
continua sendo uma operação privilegiada fora do contrato de concorrência da API.
Testes PostgreSQL comprovam isolamento, locks de autorização, rollback por falha
do endereço e duas escritas simultâneas com a mesma versão. Sem mudanças em RLS ou
grants existentes para abrir acesso direto ao browser; logs não incluem PII.

O painel /admin/{slug}/configuracoes só monta o formulário para owner. Essa
restrição visual não substitui a autorização da API. A integração HTTP valida
unidade, slug, formato e versão da resposta; o hook controla carregamento,
edição, escrita pendente, conflito e confirmação de resultado incerto. Dados
comerciais do rascunho ficam em memória, sem persistência em storage do browser.

Após 409 ou resultado incerto, a edição fica bloqueada até uma consulta explícita
à API. Se o estado atual corresponde ao enviado e sua versão avançou, a tela
confirma esse estado sem reenviar. Caso contrário, prepara revisão de três vias:
mantém mudanças remotas nos campos não editados localmente e propõe as mudanças
locais para decisão do proprietário. Adotar esse rascunho não escreve nada; exige
um novo clique em salvar usando a versão consultada. Uma falha na consulta mantém
o bloqueio. Não há retry automático de PUT nem atualização silenciosa da versão.

Cadastro e configurações compartilham validação de endereço. Slug e fuso são
somente leitura; nome confirmado pela API atualiza apenas a apresentação no menu,
sem modificar IDs, perfil ou vínculos. Testes de navegador usam Auth/API simulados
e cobrem permissões, validação, concorrência, recuperação e responsividade.
