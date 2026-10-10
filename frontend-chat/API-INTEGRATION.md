# Integração de identidade, unidades, catálogo e agendamento com a API Go

## Agendamento e barbeiros

O painel `/admin/{slug}/barbeiros` usa GET/POST `/api/v1/admin/shops/{slug}/professionals`, com Bearer e autorização da unidade. Cria profissional, associações aos serviços ativos e expediente inicial atomicamente. Intervalo/margem/dias/abertura/fechamento são configuráveis no cadastro; não há edição de calendário ainda. Um profissional não é automaticamente uma conta Supabase.

O chat `/b/{slug}` consome `/api/v1/public/shops/{slug}/professionals?service_id=...`, `/availability`, `/appointments`, `/appointments/lookup` e `/appointments/cancel`, sem Bearer. Os corpos e respostas completos estão no [contrato do backend](../backend-chat/docs/api-contract.md) e no [OpenAPI](../backend-chat/docs/openapi.yaml).

`lib/api/booking.ts` valida respostas e gera a chave criptográfica da tentativa. `booking-form.tsx` coordena seleção, contato, revisão e confirmação; `appointment-receipt.tsx` exibe comprovante/código e consulta/cancela. `lib/api/professionals.ts` e `professionals-page.tsx` integram o cadastro administrativo.

Disponibilidade não segura horário. Na confirmação, o servidor revalida tudo, grava em transação e pode retornar 409 se outra pessoa reservou antes. O formulário volta à escolha com mensagem clara. Falha ambígua mantém corpo/chave em memória e bloqueia edição; repetir recupera o resultado em vez de duplicar. A tentativa não sobrevive a uma recarga forçada. Código de consulta fica fora de URL/localStorage/logs; guardá-lo privadamente. Cancelamento só antes do início. Não há envio de WhatsApp nem consentimento de marketing neste fluxo.

## Fluxo implementado

1. /login envia e-mail/senha diretamente ao Supabase Auth.
2. O SDK mantém e renova a sessão. O frontend não copia tokens para seu contexto.
3. AdminAccessProvider chama GET /api/v1/admin/me e depois GET /api/v1/admin/shops, com Authorization: Bearer.
4. /admin mostra as unidades elegíveis, inclusive um estado de lista vazia. Não escolhe a primeira automaticamente.
5. A escolha abre /admin/{slug}/servicos. ShopPanel fornece nome/slug da unidade aos componentes.
6. O painel consulta GET /api/v1/admin/shops/{slug}/services. Cada GET/POST administrativo revalida usuário, unidade e vínculo no Go.
7. Trocar de unidade remonta componentes e cancela consultas anteriores. Logout usa scope local, limpa o estado da interface e volta ao login.

Uma conta válida no Supabase não implica perfil local ativo ou vínculo com uma
barbearia. Usuários, unidades e vínculos continuam cadastrados manualmente,
conforme [o guia de autenticação](../backend-chat/docs/authentication-proposal.md).
Não há endpoint de signup, convite, criação de barbearia ou gestão de vínculos.

## Contratos

### GET /api/v1/admin/me

Bearer válido e usuário local ativo. Retorna somente:

```json
{ "user_id": "11111111-1111-4111-8111-111111111111", "display_name": "Felipe" }
```

### GET /api/v1/admin/shops

Bearer válido e usuário local ativo. Retorna um array direto, ordenado por nome e ID:

```json
[
  {
    "shop_id": "33333333-3333-4333-8333-333333333333",
    "name": "Barbearia Centro",
    "slug": "barbearia-centro"
  }
]
```

Somente usuário/vínculo/unidade ativos qualificam. Nenhum vínculo retorna 200
com []. O ID do usuário vem do token, nunca de query/body. O resultado não é
um passe permanente; o Go verifica o membership novamente em cada operação.

### GET /api/v1/admin/shops/{slug}/services

Bearer e vínculo atual obrigatórios. A consulta recebe o ShopID do contexto
autorizado, não resolve o slug outra vez nem recebe tenant do body. Atualmente
retorna somente serviços ativos; incluir inativos/paginação é trabalho futuro.

### GET /api/v1/public/shops/{slug}/services

Sem Authorization, mesmo com uma sessão administrativa existente. O slug vem de
/b/{slug}. /chat orienta a obter o link da barbearia, sem escolher uma unidade.
Barbearia pública ausente/inativa retorna 404; barbearia ativa sem serviços retorna [].

Ambas as leituras retornam um array de serviços no mesmo formato:

```json
[
  {
    "id": "55555555-5555-4555-8555-555555555555",
    "name": "Corte",
    "description": "Máquina e tesoura",
    "duration_minutes": 30,
    "price_cents": 4500,
    "currency": "BRL",
    "active": true
  }
]
```

Os exemplos são documentais. Nenhum seed/fallback é enviado com a aplicação.

### POST /api/v1/admin/shops/{slug}/services

Mesmo Bearer/vínculo da leitura. Content-Type: application/json, máximo 64 KiB:

```json
{ "name": "Corte", "description": "Máquina e tesoura", "duration_minutes": 30, "price_cents": 4500 }
```

Retorna 201 com um serviço. O servidor gera o ID e define BRL/active=true.
Nome normalizado: 1–100 pontos de código Unicode. Duração inteira: 1–2147483647.
Preço inteiro não negativo e obrigatório, inclusive zero. O cliente converte
o decimal em centavos com precisão e limita preços a Number.MAX_SAFE_INTEGER.

Não enviar user_id, shop_id, slug, category, active ou currency no JSON. A URL
solicita a unidade; autorização no Go é que decide seu ID. Campos desconhecidos,
null, múltiplos objetos e conteúdo extra são rejeitados. A rota antiga sem
unidade foi removida; DEV_SHOP_SLUG/NEXT_PUBLIC_SHOP_SLUG não são usados.

Não há retry automático de POST. O formulário impede envio duplicado e só
confirma sucesso após a resposta, depois recarrega a lista. Falha na atualização
não desfaz o cadastro confirmado. Falha de rede/timeout ou cancelamento pode
ocorrer depois de persistir: conferir a lista antes de repetir.

## Responsabilidades e segurança

- AdminAccessProvider guarda identidade/unidades em memória. O SDK é dono da sessão.
- AdminAccessGuard não monta o painel antes da validação. 401 leva ao login; 403/503 apresentam erro recuperável.
- ShopPanel confere o slug para a navegação. Isso é proteção visual, não segurança do servidor.
- O listener de sessão é síncrono; não chama getSession/signOut dentro do callback.
- Mudança de conta/logout cancela leituras e descarta respostas antigas.
- O Go verifica ES256, issuer, audience, expiração, subject UUID e perfil/vínculo local.
- JWT previamente emitido não é imediatamente invalidado por logout. Não há consulta de revogação por requisição.
- A sessão persistida pelo SDK exige cuidado com XSS. SSR/cookies de autenticação não foram implementados.
- CORS permite apenas FRONTEND_ORIGIN exata, Authorization e Content-Type, sem cookies. OPTIONS não autentica.
- Não registrar tokens, senhas, DSNs ou erros brutos do servidor/provedor.

## Erros

Envelope: {"error":{"code":"...","message":"..."}}. O cliente preserva código/status
e apresenta mensagens locais.

| Status | Código                                         | Significado                                                   |
| ------ | ---------------------------------------------- | ------------------------------------------------------------- |
| 400    | invalid_body                                   | JSON desconhecido, inválido ou múltiplo                       |
| 401    | authentication_required / invalid_access_token | Sessão ausente ou inválida                                    |
| 403    | access_denied / origin_not_allowed             | Perfil/vínculo inelegível ou origem indevida                  |
| 404    | shop_not_found                                 | Unidade pública ausente/inativa ou desativada antes do INSERT |
| 413    | body_too_large                                 | Corpo acima de 64 KiB                                         |
| 415    | unsupported_media_type                         | Content-Type incorreto                                        |
| 422    | invalid_service                                | Campos inválidos                                              |
| 500    | internal_error                                 | Falha sanitizada                                              |
| 503    | temporarily_unavailable                        | JWKS/dependência indisponível ou timeout                      |

Falhas de rede, timeout do cliente (10 segundos), resposta incompatível e
configuração inválida têm mensagens específicas. 404/405 de rotas inexistentes
ainda usam as respostas padrão do Chi.

## Configurar e testar manualmente

No backend, configurar o .env sem alterar credenciais existentes:

```dotenv
SUPABASE_URL=https://SEU-PROJETO.supabase.co
FRONTEND_ORIGIN=http://127.0.0.1:3000
```

DATABASE_URL deve apontar ao banco que contém o perfil/vínculos provisionados.
Docker PostgreSQL puro não fornece Supabase Auth. Aplicar migrations é operação
separada, explicitamente revisada para o banco de destino; /ready verifica
conectividade, não schema nem contas. Ver [teste do catálogo](../backend-chat/docs/catalog-testing.md).

No frontend, .env.local:

```dotenv
NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8080
NEXT_PUBLIC_SUPABASE_URL=https://SEU-PROJETO.supabase.co
NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY=sb_publishable_...
```

Valores NEXT_PUBLIC_* entram no JavaScript. Nunca usar sb_secret_*, service_role,
DATABASE_URL ou senha de banco. Reiniciar processos após mudar configuração.

```powershell
# Terminal do backend, em backend-chat:
go run ./cmd/api

# Outro terminal, em frontend-chat:
npm.cmd run dev
```

Abrir /login, entrar, escolher a unidade e cadastrar um serviço. Recarregar a
página e confirmar que o serviço permanece. Conferir o catálogo público em
/b/{slug}. Testar outro usuário/unidade e verificar isolamento. 403 exige revisar
perfil/vínculo e atividade no banco, não adicionar um slug à .env.

## Estrutura e validação

src/lib/api centraliza fetch, timeout, contratos e validação; identity.ts chama
/me e /shops, services.ts chama as operações por slug. use-services compartilha
o ciclo de consulta pública/administrativa. app/admin/[slug] define o painel
da unidade; components contém a seleção, guards e telas. requestctx no Go
transporta identidade/escopo dentro de uma requisição, sem estado global.

```powershell
npm.cmd run typecheck
npm.cmd run format:check
npm.cmd run build
npm.cmd test
```

Playwright constrói uma versão de produção em .next-playwright, inicia um servidor
isolado na porta 3100 e usa respostas HTTP fictícias apenas nos testes. Não chama
o projeto Supabase real nem grava serviços no banco. Cobre login, ordem de chamadas,
seleção/troca, lista vazia, bloqueios, logout, restauração, respostas atrasadas,
contratos, erros, cadastro, cancelamento e telas móveis. Integração PostgreSQL e
criptografia Go são validações separadas; ver backend-chat/docs/validation.md.

## Pendências

Edição/exclusão/ativação, serviços inativos/paginação, edição de calendário,
notificações, convites e recuperação real ainda não estão integrados. A agenda
visual administrativa é um calendário sem consulta de reservas; o fluxo público
de agendamento, consulta e cancelamento está integrado. A identidade Palma continua
sendo referência visual. Consultar a documentação de autenticação para o
provisionamento e os requisitos antes de publicar.
