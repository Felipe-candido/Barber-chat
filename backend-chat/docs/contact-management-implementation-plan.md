# Contatos da barbearia — plano de implementação

Data: 2026-11-02. Estado: planejado; este documento não implementa endpoints,
interface ou migration. Não altera README, doc.go, configuração ou banco.

Decisão do usuário: proprietário e barbeiros podem consultar e cadastrar todos
os contatos da unidade. O painel deve continuar simples.

## 1. Resultado da primeira versão

Adicionar **Contatos** ao painel administrativo, em `/admin/{slug}/contatos`:

- Lista com nome completo, telefone formatado e data do cadastro.
- Busca por nome ou telefone, paginação de 25 itens e botão Novo contato.
- Cadastro manual com nome, sobrenome opcional e telefone com DDD.
- Clientes de agendamentos novos aparecem automaticamente, sem ação adicional.
- Contatos já persistidos por agendamentos anteriores aparecem na mesma lista.
- Proprietário e barbeiros utilizam a mesma base da barbearia selecionada.

Um contato não tem login, senha, assinatura nem cadastro Supabase Auth. Sua
inclusão não cria agendamento, venda, intenção de mensagem ou autorização de WhatsApp.
Pode existir sem nunca ter agendado; a tela se chama Contatos por esse motivo.

Não incluir inicialmente CPF, aniversário, endereço, tags, campanhas, importação,
exportação em massa, programa de fidelidade ou um dashboard adicional. Edição,
exclusão/arquivamento e mudança de telefone ficam para uma extensão com regras
próprias; a primeira entrega solicitada é consultar e cadastrar.

## 2. Base existente e decisão de arquitetura

Não criar outro módulo `contacts`, tabela paralela ou serviço. Expandir o módulo
`internal/modules/customers`, dono de nomes, telefones e permissões de mensagens.
Contatos é o nome de apresentação no frontend; customers continua sendo o nome
de negócio usado no Go e nas tabelas.

O projeto já possui:

- `customers`, com ID, shop_id, first_name, last_name, phone_e164 e created_at.
- `UNIQUE (shop_id, phone_e164)` para impedir duplicatas por unidade.
- `appointments.customer_id` com FK composta por unidade.
- `BookingContacts.ResolveForBooking`, executado na transação da reserva.
- `InsertBookingCustomer ... ON CONFLICT DO NOTHING`, seguido de busca do ID.
- Normalização com metadados de telefone e saída E.164, como +5511999999999.
- RLS e bloqueio de acesso direto das roles públicas às tabelas do negócio.

Assim, clientes históricos persistidos pelo booking não precisam de importação
ou backfill para aparecerem. Só existem contatos quando a reserva realmente foi
persistida; abandonar o formulário ou perder a disputa pelo horário não deve
criar um contato parcial. Essa atomicidade será preservada.

| Módulo | Responsabilidade |
| --- | --- |
| customers | Cadastro, consulta, validação e deduplicação dos contatos |
| booking | Agendamento, vínculo com contato e transação da reserva |
| identity | Verificar usuário e vínculo ativo com a unidade |
| notifications | Usar autorização por agendamento e snapshot protegido, sem inferir consentimento pela existência do contato |
| reporting | Histórico e valores derivados, preservando regras de acesso |
| cmd/api e httpapi | Composição, rotas e middlewares |

Permanecem um go.mod, o pool PostgreSQL compartilhado e chamadas síncronas.
Não há necessidade de fila, worker ou nova dependência para cadastrar um contato.

## 3. Regras de dados e concorrência

### Identificação e nomes

- ID do contato é o UUID local à aplicação, sempre consultado junto de shop_id.
- Telefone é um dado de contato e uma chave de deduplicação, não prova de identidade.
- O mesmo telefone em duas barbearias produz dois contatos independentes.
- Na primeira versão, duas pessoas usando o mesmo telefone na mesma unidade
  compartilham o registro. Não deduplicar por nome nem prometer identificar
  pessoas distintas com base só no número.
- Nome obrigatório: 1..100 caracteres; sobrenome opcional: até 100. Aparar espaços,
  rejeitar controles e preservar acentos. Formar exibição sem espaço sobrando.
- Telefone obrigatório, validado e normalizado no servidor. País BR no formulário,
  mantendo contrato que possa aceitar outro país; E.164 é a forma persistida.

### Resolução automática e criação manual

Booking continua resolvendo o contato dentro da própria transação. Agendamento
público com telefone existente não deve sobrescrever o nome já cadastrado nem
revelá-lo ao visitante. O formulário público não recebe busca/autocomplete da agenda
de contatos da barbearia.

Cadastro manual usa a mesma tabela e a mesma normalização. Se o número já existir,
retornar 409 contact_exists e permitir abrir/conferir o contato existente,
sem alterar seu nome silenciosamente. Não usar um upsert que substitua nomes.

Uma busca antes de salvar pode melhorar a interface, mas não garante unicidade.
A constraint continua sendo a proteção final para dois cadastros simultâneos
ou para cadastro manual concorrente com agendamento. O adaptador traduz a
violação específica de unicidade para erro de negócio, sem expor SQL bruto.

Após resposta perdida, frontend faz leitura pelo telefone normalizado e pede
conferência do registro encontrado. Não envia POST automaticamente e não presume
que um registro criado por outra requisição tenha os mesmos dados do formulário.
Não é necessária outra tabela de idempotência para esse efeito limitado: a
unicidade do contato evita duplicata; recuperar a resposta é uma leitura explícita.

### Permissões e assinatura

Lista e cadastro exigem usuário local ativo, membership ativo e loja ativa.
Owner e barber têm acesso completo a esses dois recursos da unidade. Revalidar
o acesso no caso de uso/adaptador transacional, além do middleware.

O tenant vem do slug autorizado no contexto, nunca de shop_id informado no body.
UUID de contato de outra unidade não deve dar acesso aos seus dados. Respostas
administrativas usam no-store; logs registram rota/resultado, sem nomes, telefones
ou corpos. Não expor a lista no site público ou liberar leitura via Supabase Data API.

Essa autorização não amplia a agenda de barbeiros: eles continuam limitados aos
próprios atendimentos para consultar detalhes, alterar estado ou enviar lembretes.
Não criar novos papéis/permissões granulares para esta primeira versão.

Permitir preparar contatos no painel antes da assinatura, como outros cadastros
de configuração. Cadastro de contato não contorna a exigência de assinatura para
agendar manualmente nem libera a página pública da barbearia.

### WhatsApp e informações históricas

Contato cadastrado não significa permissão de mensagem, marketing ou comprovação
de posse do telefone. Autorização continua sendo coletada por agendamento.
Cadastrar manualmente não chama Evolution e não cria jobs.

Agenda e relatórios atuais consultam nomes/telefones de customers; não possuem
snapshot independente do nome do cliente. Não descrever esse nome como histórico
imutável. Nesta primeira versão, não há edição do contato; preços/serviços/código
dos agendamentos e conteúdo cifrado das notificações continuam com seus contratos.

## 4. Entregas em ordem

### A — Contratos e regras do módulo customers

Criar durante a implementação, sem antecipar stubs:

```text
internal/modules/customers/
  domain/contact.go
  domain/errors.go
  application/dto.go
  application/ports.go
  application/list_contacts.go
  application/get_contact.go
  application/create_contact.go
```

DTOs próprios: scope autenticado, entrada manual, filtros, item da lista e página.
Portas pequenas para acesso à unidade, normalização e persistência usada pelos
casos de uso. Não obrigar customers/application a importar os DTOs de booking.

A normalização existente em infra deve compartilhar a implementação de parsing
com o novo fluxo, mantendo adaptadores de erros/DTOs separados para booking,
reporting e customers. Validar o domínio sem importar pgx, HTTP ou Supabase.
Não modificar o contrato público atual do booking para exigir menos campos
apenas porque o cadastro manual admite sobrenome opcional.

Aceite: regras de nome/telefone e conflitos definidas; exemplos com nome único,
acentos, formatos nacionais/internacionais e telefone inválido.

### B — SQL e compatibilidade com o agendamento

Adicionar `db/queries/customer_contacts.sql` e adaptador concreto em
`customers/infra/postgres/contacts.go`, usando pgx/sqlc existentes.

Criar migration com o próximo número disponível no momento da implementação:

- Ajustar a constraint de last_name para aceitar sobrenome vazio no cadastro manual,
  mantendo NOT NULL e permitindo que reservas já existentes continuem válidas.
- Revisar projeções de nome em agenda, lifecycle e relatórios para não retornar
  espaço final quando faltar sobrenome.
- Acrescentar índice de paginação por (shop_id, created_at DESC, id DESC).
- Preservar as constraints de telefone, FKs e RLS existentes.
- Não criar tabela duplicada, reescrever reservas ou alterar contatos históricos.

Paginação por cursor opaco com created_at/id, limite padrão 25 e máximo 100;
ordenar por cadastros mais recentes, sem cursor baseado só em timestamp.
Cursor inválido resulta em erro, não em listagem global. Cada busca continua
incluindo shop_id autorizado, mesmo quando o cursor veio de outra unidade.

Busca inicial: nome parcial e telefone exato normalizado, com parâmetros limitados
e SQL parametrizado. O frontend identifica consulta por número e explica quando
um telefone completo é necessário. Não adicionar extensão de busca antes de
medir necessidade. Incluir cadastro manual que aparece mesmo sem appointments;
a listagem não pode usar INNER JOIN obrigatório com agendamentos.

Aceite: migration up/down/up local, sqlc gerado, contatos antigos visíveis,
normalização equivalente e unicidade comprovada com concorrência real.
Down deve recusar restaurar a antiga exigência de sobrenome se houver registros
que não a atendam; não apagar contatos para fazer rollback passar.

### C — Casos de uso e API administrativa

Contrato proposto, a ser publicado no OpenAPI junto da implementação:

| Método/rota após /api/v1/admin/shops/{slug} | Comportamento |
| --- | --- |
| GET /contacts | Lista paginada; filtros name/phone/phone_country, limit e cursor |
| GET /contacts/{contactID} | Dados do contato na unidade, sem histórico de outros barbeiros |
| POST /contacts | Criação manual com first_name, last_name opcional, phone e phone_country |

Usar contacts no transporte para combinar com a interface, mantendo customers
como módulo/tabela. Input não contém shop_id, user_id, flags de consentimento ou
identificadores de provider. Criar DTOs HTTP separados dos tipos de aplicação.

Respostas: 201 criação; 409 telefone existente; 422 dados inválidos; 401/403
autenticação/acesso; 404 contato inexistente no escopo; 503 indisponibilidade.
OPTIONS/CORS devem incluir as rotas novas, com a mesma origem permitida existente.
Nenhuma requisição cria usuário Supabase, assinatura ou vínculo de acesso.

Aceite: autenticação, owner/barber, isolamento entre unidades, validação, conflito
e indisponibilidade cobertos sem depender só da ocultação de botões no frontend.

### D — Tela Contatos e cadastro manual

Arquivos previstos no frontend:

```text
src/app/admin/[slug]/contatos/page.tsx
src/components/contacts/contacts-page.tsx
src/components/contacts/contact-form.tsx
src/components/contacts/contacts.module.css
src/lib/api/contacts.ts
tests/contacts.spec.ts
```

Adicionar item ao menu para owner e barber, preservando o design, navegação mobile
e componentes existentes. Lista/tabela no desktop e apresentação legível no celular.
Separar carregamento, nenhum contato, busca sem resultados e falha de API.

Formulário com nome, sobrenome opcional e telefone. Campos vinculados às mensagens
de erro; envio desabilitado enquanto pendente. Não salvar PII em localStorage.
Após 201, atualizar a lista e confirmar cadastro. Após 409, oferecer conferência
do existente. Após falha ambígua, preservar rascunho e exigir leitura antes de
repetir; nunca apresentar erro de rede como lista vazia ou cadastro confirmado.

Troca de barbearia/logout limpa estado e cancela requisições; resposta atrasada
da unidade anterior não pode preencher a tela atual.

Aceite: proprietário e barbeiro cadastram/consultam, busca e paginação funcionam,
lista inclui clientes de reservas e manuais, sem alterar o layout das outras telas.

### E — Validação final e homologação

- Dois cadastros manuais concorrentes com variantes do mesmo telefone: uma linha.
- Cadastro manual versus booking concorrente: mesmo contato e reserva consistente.
- Repetir booking com a mesma chave: sem contato/reserva/mensagem duplicados.
- Booking mal sucedido: nenhum contato parcial deixado pelo novo fluxo.
- Número igual em duas unidades: isolamento, sem nomes/histórico compartilhados.
- Público não consulta contato existente nem sobrescreve seu nome.
- Contato manual sem agendamento aparece; cliente antigo também aparece.
- Manual não grava consentimento, plano de mensagens ou chamada Evolution.
- Verificar que consulta geral de contatos por barber não amplia leitura da agenda.
- Testar nome sem sobrenome e formato de exibição nas projeções afetadas.
- Playwright: owner/barber, formulário, conflito, resposta perdida, paginação,
  troca de unidade, erros de leitura e tela estreita.

Rodar Go tests/vet/builds, integração com PostgreSQL descartável, typecheck/build
e testes de navegador. Aplicação no Supabase será uma etapa explícita após
validação; não há migration a executar apenas por criar este plano.

## 5. Extensões posteriores, fora da primeira entrega

1. Selecionar contato já cadastrado ao criar agendamento administrativo, com
   preenchimento do formulário e validação no servidor. Público continua sem
   autocomplete da lista privada. A opção de WhatsApp continua independente do
   cadastro do contato e pode ser desmarcada antes de confirmar a reserva.
2. Abrir histórico a partir de um contato, usando reporting e autorização própria;
   visualizar um contato não libera atendimentos de outro profissional.
3. Corrigir nome com controle de versão; definir se relatórios mostram nome atual
   ou snapshot antes de prometer preservação histórica do cliente.
4. Alterar telefone com regras explícitas de conflito, permissão por reserva e
   invalidação de mensagens pendentes; não atualizar destinos em snapshots antigos.
5. Arquivar, corrigir duplicatas ou tratar telefones compartilhados somente após
   definir retenção, reativação e referências de agendamentos. Sem exclusão em cascata.

Ordem inicial recomendada: A → B → C → D, com validação em cada parte e encerramento
pela entrega E. Explicar arquivos e fluxo ao concluir cada bloco. As extensões
acima exigem outra decisão de escopo e não atrasam a lista/cadastro solicitados.
