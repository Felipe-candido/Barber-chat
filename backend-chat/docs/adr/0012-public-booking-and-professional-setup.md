# ADR 0012 — Agendamento público e cadastro inicial de profissionais

Status: aceito e implementado em 02/10/2026. Atualiza o plano dos ADRs 0010/0011; WhatsApp permanece adiado.

## Decisão

- Booking implementa disponibilidade, criação, consulta e cancelamento. Catalog mantém profissional/serviço; customers normaliza/resgata contato; shops resolve a unidade. Domain/application só dependem das próprias regras/portas; cmd compõe adapters sobre o mesmo sqlc transacional.
- Um lock `FOR UPDATE` na linha do profissional serializa criação/cancelamento/alterações de calendário. A leitura feita antes da confirmação não reserva nada. Após o lock, disponibilidade é recalculada.
- PostgreSQL impõe `appointments_no_overlap`: exclusão GiST por unidade/profissional e `tstzrange(...,'[)')`, para todo status diferente de cancelled. FKs compostas impedem associações entre tenants. A exclusão é proteção independente contra reservas sobrepostas; SQL manual continua responsável por respeitar expediente e bloqueios.
- A política arredonda duração + margem para blocos inteiros ancorados na abertura. Datas são civis no fuso IANA da unidade; instantes são persistidos em UTC. Horários DST ambíguos/inexistentes são omitidos conservadoramente. Horizonte público inicial: hoje até 90 dias.
- Confirmação gera código opaco aleatório de 160 bits, armazenado com SHA-256. O código permite consultar/cancelar apenas uma reserva na unidade; não autentica pessoa nem libera histórico. Não vai na URL/logs. Não expira automaticamente nesta etapa; cancelar só antes do início. Cancelamento repetido é idempotente.
- Criação exige chave de 256 bits aleatórios do frontend, em 64 caracteres hexadecimais minúsculos. Banco guarda hash da chave, fingerprint HMAC da entrada e recibo cifrado AES-GCM derivado do segredo da tentativa. Mesmo corpo/chave recupera resultado por 24 horas; outra entrada ou chave expirada resulta em conflito. Registros expirados não são removidos automaticamente.
- O frontend mantém contato/tentativa apenas em memória. Falha ambígua preserva chave/corpo e bloqueia edição; uma recarga forçada perde essa recuperação. Persistência segura de tentativas poderá ser avaliada separadamente, sem gravar PII por padrão.
- Cadastro administrativo de profissional cria associações e um período inicial nos dias escolhidos atomicamente. Catalog consome uma porta de calendário inicial, implementada por booking/infra e montada no bootstrap. Não cria login Supabase nem atribui membership. UI de edição, pausas, folgas e agenda administrativa permanece futura.
- Migration 00005 habilita RLS e revoga acesso de PUBLIC/anon/authenticated a tabelas de negócio, incluindo shops/services. O Go usa conexão servidor autorizada; uma role restrita requer grants/políticas próprios. Nunca expor DATABASE_URL ou credenciais de servidor ao navegador.

## Consequências e limites

Não há PII no comprovante público nem autorização por telefone/UUID. Cliente e reserva não ficam parcialmente gravados se qualquer etapa falhar. APIs públicas têm limite de corpo e rate limit básico por processo; produção requer políticas de edge, TLS e revisão de privilégios. O código é uma credencial: qualquer pessoa que o possua pode consultar/cancelar dentro das regras.

No futuro, mensageria deve acrescentar consentimento explícito e intenção/outbox na transação, sem envio externo antes do commit. Nada nesta entrega afirma que mensagens são enviadas. O cadastro administrativo de profissional ainda não tem idempotência; falhas ambíguas exigem conferir a lista antes de nova tentativa.

Testes PostgreSQL usam banco descartável separado: requisições concorrentes ao mesmo intervalo, repetição com a mesma chave, bloqueio concorrente, violação SQL direta, FKs, rollback e fluxo profissional→agendamento. Testes de navegador usam HTTP simulado e não substituem essa integração.
