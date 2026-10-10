# ADR 0013 — Dois perfis fixos, sem painel de permissões

Status: implementada em 2026-10-10. Substitui a igualdade de poderes da ADR 0009.

## Decisão

O produto mantém somente `owner` e `barber`, definidos no vínculo com a unidade, não no JWT nem em `user_metadata`. Proprietário administra; barbeiro consulta catálogo, sua própria agenda e cria reservas para seu profissional. Serviços e preços são globais à unidade e permanecem sob gestão do proprietário. Não há gerente, recepcionista, papéis customizados ou matriz configurável de permissões.

O proprietário inicial continua provisionado manualmente. A migration 00006 converte todos os vínculos existentes em proprietários, conforme decisão do usuário, e remove o default: cada vínculo novo precisa declarar seu perfil. Barbeiro exige associação a um profissional da mesma unidade, com FK composta e índice único. O profissional e o vínculo são conceitos diferentes: cadastrar um profissional não cria uma conta de login.

## Fluxo e camadas

`Authenticate` valida a identidade Supabase e o usuário local. `AuthorizeShopAction` relê usuário, unidade e vínculo; perfis desconhecidos e barbeiros sem profissional são negados. O adaptador SQL calcula acesso efetivo do barbeiro incluindo a atividade do profissional. `ListMembershipsByUser` permanece uma leitura do estado armazenado do vínculo; não é um método de autorização.

`AuthorizedShopScope` contém perfil e profissional. `OwnerOnly`, após autenticação e autorização da unidade, protege os POSTs administrativos de serviços e profissionais e as rotas de contas. As leituras do catálogo servem também à escolha das opções de agendamento. `GET /admin/shops/{slug}/access` informa o perfil à interface, sem conferir autorização permanente.

O adaptador HTTP da agenda traduz o escopo de barbeiro em uma restrição explícita de profissional para a application de booking. Ela não importa identity. O repositório aplica a restrição no SQL, junto ao tenant, antes de ordenar/paginar. Proprietário não recebe essa restrição. O navegador não pode substituí-la por parâmetro de consulta.

## Contas e reservas administrativas

`identity` possui os contratos de acesso e provisionamento. O adaptador Supabase usa uma chave administrativa somente no servidor, com timeout, sem seguir redirects ou repetir convites automaticamente. O cadastro do profissional continua em catalog; não há import de application de catalog em identity.

A migration 00007 registra uma solicitação única por profissional antes da chamada externa. A transação local revalida proprietário, profissional e usuário; não sobrescreve vínculos de proprietário. Auth e PostgreSQL não compartilham transação: falha ambígua deixa a solicitação pendente/em revisão, impedindo novo envio automático. Recuperação exige UUID conferido no Auth e e-mail correspondente. Desativar acesso altera só o membership desta unidade, não a conta global Supabase.

O barbeiro define a própria senha por convite na rota dedicada `/definir-senha`, enviando-a apenas ao Supabase. O frontend mantém PKCE nos fluxos normais; a rota de convite usa implicit, compatível com convites administrativos abertos em outro navegador. Menus e formulários são simples, mas não constituem autorização.

POST administrativo de reservas usa o mesmo núcleo de booking público, com uma autoridade separada do DTO público. Revalida membership na transação, inclusive no replay. A ordem é chave de idempotência, lock do profissional, lock/leitura de acesso. Não muda o fingerprint público nem permite ao barbeiro trocar o profissional. Restrições de sobreposição e gravação atômica existentes permanecem.

## Limites

A migration 00008 permite corrigir e-mail com “Substituir conta”. Preserva histórico com `replaced_at` e unicidade parcial da conta atual. Só remove o membership armazenado como inativo, com perfil barbeiro e profissional correto, nesta unidade. Nunca altera/deleta Auth global ou usuário local; outras unidades e reservas permanecem intactas. Substituição cria UUID novo na mesma transação, sob lock comum do profissional. Vínculo e ativação releem o registro depois desse lock, impedindo ressurreição por requisição antiga. Auth é chamado após commit; falha exige revisão do novo registro, sem restaurar acesso antigo ou reenviar automaticamente. Rollback da migration recusa descartar histórico existente.

A equipe não compartilha agendas por padrão. O fluxo público continua independente do login. Convite aceito pelo provedor não garante entrega do e-mail; SMTP e redirects precisam estar configurados no Supabase. Não há reenvio automático, transferência de conta entre profissionais nem editor de perfis. Relatórios/histórico gerencial e notificações permanecem entregas posteriores.
