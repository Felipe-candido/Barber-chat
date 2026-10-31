# ADR 0016 — Cadastro próprio com sessão confirmada e criação atômica

Status: API e interface de cadastro implementadas. Data: 2026-10-29.

## Fluxo

O frontend continuará enviando senha apenas ao Supabase. Após confirmar e-mail e
obter sessão, chamará POST /api/v1/onboarding/shops com Bearer, Idempotency-Key e
dados da barbearia. A API verifica JWT pelo verifier compartilhado e consulta
/auth/v1/user com o token do usuário e uma chave publicável/anon, sem privilégios
administrativos. O UUID precisa coincidir e email_confirmed_at deve estar definido.
Metadata editável não concede identidade confirmada nem permissão de owner.

O contexto de cadastro é distinto da identidade administrativa local. Não altera
Authenticate nem autoriza /admin/me antes do provisionamento. Dentro de uma única
transação: provisiona usuário se ausente, bloqueia a linha do usuário, encontra
repetição ou cria unidade + membership owner + registro durável de criação.
Não sobrescreve perfil nem reativa usuário/membership. Auth não participa dessa
transação; a conta Auth pode existir enquanto o usuário corrige dados de cadastro.

## Repetição e conflitos

A interface em /cadastro envia nome, e-mail e senha apenas ao Auth e exige senha
de 12–128 caracteres, sem substituir a política do provedor. A confirmação PKCE
retorna à rota fixa /cadastro/barbearia; o SDK troca o código uma única vez.
O formulário da unidade coleta nome, slug sugerido/editável, fuso e endereço
brasileiro obrigatório: CEP, logradouro, número (ou S/N), bairro, cidade e UF;
complemento opcional. Metadata do nome serve só como preenchimento inicial.

Migration 00011 cria shop_addresses, com campos completos, constraints e RLS.
O adapter grava o endereço na mesma transação; ele participa do fingerprint.
Unidades antigas sem endereço permanecem válidas: não inventar dados. O banco
valida as linhas de endereço existentes, mas não exige endereço para todo shop
legado; novos cadastros são obrigados pelo caso de uso. Pedidos antigos sem
endereço recebem 422, não são refeitos automaticamente.

A tentativa enviada fica no sessionStorage da aba, vinculada ao usuário: chave e
dados comerciais, sem senha/e-mail/token adicional. Reload recupera corpo/chave.
Sucesso/saída explícita limpam a tentativa; troca de conta invalida a tela. Erro
ambíguo bloqueia edição; rejeição definitiva de dados/slug permite correção.
Ao fechar a aba esse armazenamento termina: conferir as unidades no painel antes
de recomeçar sem a tentativa. Configurações e Stripe são próximas etapas.

No Supabase, habilitar cadastro e confirmação de e-mail e permitir o redirect
exato da origem utilizada + /cadastro/barbearia. Localhost e 127.0.0.1 são origens
distintas; PKCE precisa do navegador que iniciou o fluxo. Após confirmar em outro
dispositivo, fazer login e continuar. Não aceitar destinos via query string.

Chave aleatória de 32 bytes em hexadecimal minúsculo. Persistem somente hash da
chave e fingerprint HMAC da entrada normalizada. O lock do usuário serializa
requisições concorrentes da mesma conta. Mesma entrada/chave devolve a unidade
atual com 200; conflito de entrada ou slug retorna 409. Falha de commit não informa
sucesso; repetir a mesma chave resolve resultado incerto. Replay revalida acesso.
Não apagar registros para liberar reuso. Dados financeiros permanecem separados.

## Publicação e compatibilidade

Novas unidades recebem provisioning_source=self_service, active=true para permitir
configuração e publication_requested=false. O guard das rotas públicas de catálogo,
profissionais, horários e criação exige intenção de publicação E cobertura local
confirmada em billing. Não há endpoint de publicação nem integração Stripe ainda.
Lookup/cancelamento existentes não recebem esse guard. A criação pública revalida
publicação e cobertura dentro da transação, após recuperar um eventual replay:
repetição de uma reserva já confirmada continua recuperável após suspensão.
O fluxo comercial da criação manual permanece nas próximas entregas.

Unidades provisionadas manualmente preservam o comportamento público atual;
essa compatibilidade não é plano gratuito e exige rollout comercial explícito
antes do lançamento. Cliente JSON não escolhe origem, UUID, perfil ou publicação.

SUPABASE_PUBLISHABLE_KEY é opcional: ausência desabilita onboarding (503), sem
derrubar login/API existentes. Não editar .env real automaticamente. Antes de
lançamento, configurar confirmação de e-mail, SMTP, redirects, CAPTCHA/rate limits
do Auth e proteção de abuso do onboarding; esta etapa não é autorização de deploy.

Referência: [objeto de usuário Supabase](https://supabase.com/docs/guides/auth/users).
