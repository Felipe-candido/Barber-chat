# Contrato com o frontend

## Autenticação e unidades

Login envia credenciais apenas ao Supabase. O frontend chama `GET /api/v1/admin/me` e depois `GET /api/v1/admin/shops` com Bearer, mostra as unidades elegíveis e abre `/admin/{slug}/servicos` após a escolha. Cada operação administrativa revalida usuário, unidade e vínculo ativos. Contas, unidades e memberships continuam manuais; cadastrar um profissional não cria uma conta Supabase.

Health/readiness, identidade e catálogo estão descritos no [OpenAPI](openapi.yaml). Readiness verifica conectividade PostgreSQL, não migrations nem RabbitMQ. Todas as rotas abaixo estão implementadas; WhatsApp e agenda visual administrativa não estão.

## Cadastro administrativo de profissionais

`GET/POST /api/v1/admin/shops/{slug}/professionals` exige Bearer e membership ativo. GET retorna array de `{id,name,active,service_ids}`. POST cria profissional, associações a serviços ativos da mesma unidade e calendário inicial em uma transação; retorna 201 com o mesmo formato.

```json
{
  "name": "João",
  "service_ids": ["11111111-1111-4111-8111-111111111111"],
  "schedule": {
    "interval_minutes": 60,
    "buffer_minutes": 0,
    "weekdays": [1, 2, 3, 4, 5, 6],
    "opens_at_minute": 570,
    "closes_at_minute": 1110
  }
}
```

Domingo = 0; minutos contam desde meia-noite no fuso da unidade. O exemplo atende segunda a sábado, 09:30–18:30. Intervalo: 1–1440; margem: 0–1440. É necessário pelo menos um serviço e um dia, sem duplicatas; o período deve comportar ao menos um bloco. O painel começa com intervalo 60 e margem 0, editáveis antes de cadastrar. Ainda não há edição de profissional/calendário, gestão de pausas ou folgas pela UI.

JSON administrativo tem limite de 64 KiB e rejeita campos extras, inclusive `shop_id`. POST de profissional não possui idempotência: numa falha de rede ambígua, atualizar a lista e conferir antes de iniciar novo cadastro.

## Fluxo público implementado

Chat/formulário em `/b/{slug}`, sem conta de cliente ou token administrativo. As rotas usam a base `/api/v1/public/shops/{slug}`:

| Método e sufixo | Resultado |
| --- | --- |
| GET /services | Serviços ativos |
| GET /professionals?service_id=UUID | `{shop_name,shop_timezone,today,last_booking_date,professionals:[{id,name}]}` |
| GET /availability?service_id=UUID&professional_id=UUID&date=YYYY-MM-DD | `{shop_timezone,slots:[{starts_at,ends_at}]}` |
| POST /appointments | Cria/reproduz reserva; exige Idempotency-Key |
| POST /appointments/lookup | Consulta uma reserva com `{"code":"..."}` |
| POST /appointments/cancel | Cancela uma reserva com `{"code":"..."}` |

Disponibilidade cobre hoje até 90 dias adiante no fuso da barbearia, omitindo inícios passados. Não há reserva temporária ao selecionar. Os horários seguem a abertura do expediente: 09:30, 10:30, 11:30 em grade de 60 minutos. A ocupação é `ceil((duração + margem) / intervalo) * intervalo`: serviço de 40 ocupa 60; de 80 ocupa 120. Todo o período deve estar livre e caber no expediente. Datas/horas civis ambíguas ou inexistentes por horário de verão são omitidas conservadoramente.

Corpo de criação (IDs ilustrativos; início deve vir da disponibilidade):

```json
{
  "service_id": "11111111-1111-4111-8111-111111111111",
  "professional_id": "22222222-2222-4222-8222-222222222222",
  "starts_at": "2026-10-03T12:30:00Z",
  "customer": {
    "first_name": "Ana",
    "last_name": "Silva",
    "phone": "+5511912345678",
    "phone_country": "BR"
  }
}
```

Nenhum preço, duração, fim, tenant ID, status ou consentimento é aceito do navegador. O servidor reconsulta as informações atuais. Telefone é normalizado para E.164 com metadados, sem comprovar titularidade. Cliente só é persistido ao confirmar, na transação da reserva.

Resposta 201 (primeira criação) ou 200 (repetição): `{appointment,code,replayed}`. O objeto `appointment` contém `id,shop_name,status,service_name,duration_minutes,price_cents,currency,professional_id,professional_name,starts_at,ends_at,shop_timezone`. Início/fim são instantes RFC3339; `ends_at` é o fim reservado, não necessariamente o término estimado do corte. Snapshots preservam serviço/preço/duração/nome do profissional/fuso. Nome e telefone do cliente não aparecem no comprovante público.

Consulta/cancelamento retornam `{appointment,can_cancel}`. O código aleatório de 160 bits permite consultar e cancelar apenas aquela reserva naquela unidade; não dá acesso à conta/histórico. Ele não vai na URL nem nos logs; a tabela de agendamentos armazena só SHA-256. Quem possuir o código poderá usá-lo: guardar em local privado. Cancelamento permitido estritamente antes do início; repetir cancelamento já concluído não altera novamente a versão. Não há expiração automática do código de consulta nesta etapa.

## Repetição segura e concorrência

O frontend gera `Idempotency-Key` com 32 bytes criptograficamente aleatórios, representados por 64 caracteres hexadecimais minúsculos. É uma credencial secreta para recuperar a resposta daquela tentativa. Repetir mesma chave e entrada normalizada devolve o comprovante original, sem criar outro atendimento; entrada diferente retorna 409. Janela de repetição: 24 horas; chave expirada retorna 409 e não é reutilizada automaticamente. Registros expirados permanecem como proteção contra reuso; ainda não existe limpeza automática.

A chave não é gravada em texto puro. O banco guarda seu hash e um comprovante cifrado AES-GCM, cuja chave deriva do segredo da requisição. O fingerprint do corpo é HMAC com a chave da tentativa. A resposta original pode ter status antigo após cancelamento; o frontend consulta o código novamente para mostrar o estado atual.

Após falha ambígua de rede/servidor, o formulário mantém chave e entrada em memória, impede edição e permite repetir a confirmação. Não fechar/recarregar nessa situação: há aviso, mas não persistência da tentativa após recarga forçada. Sem comprovante e sem a tentativa, consultar a barbearia antes de reservar novamente.

Criação, cancelamento e alterações de calendário compartilham bloqueio da linha do profissional. Depois de obter o lock, o servidor reconsulta disponibilidade. A restrição PostgreSQL `appointments_no_overlap` impede qualquer sobreposição de intervalos não cancelados para o mesmo tenant/profissional, inclusive em escritas SQL diretas. Conflito vira 409 `slot_unavailable`. Uma transação que falha não deixa cliente novo ou reserva parcial.

A migration 00005 habilita RLS e revoga acesso de PUBLIC/anon/authenticated às tabelas de negócio, inclusive shops/services. O Go necessita uma conexão servidor autorizada; o navegador nunca acessa essas tabelas diretamente. Regras de expediente/bloqueios exigem o fluxo Go e o protocolo comum de locks: a exclusão SQL protege sobreposição entre reservas, não substitui toda validação de negócio.

## Erros e limites

Erros: `{"error":{"code":"...","message":"..."}}`; sem SQL, dados pessoais ou detalhes internos.

- 400: JSON inválido, campos desconhecidos ou objeto adicional.
- 401/403: autenticação/acesso administrativo ou origem CORS não permitida.
- 404: unidade/opção/reserva não encontrada no escopo solicitado.
- 409: horário ocupado, conflito/expiração de idempotência ou cancelamento encerrado.
- 413/415: corpo excessivo ou Content-Type diferente de application/json.
- 422: campos de entrada inválidos.
- 429: limite de requisições; Retry-After 60.
- 500/503: falha inesperada ou indisponibilidade/deadline.

Booking limita corpos a 8 KiB. As respostas não devem ser armazenadas em cache. CORS permite a origem exata FRONTEND_ORIGIN e headers Authorization, Content-Type e Idempotency-Key; OPTIONS não autentica. Limites básicos são locais ao processo: 20 POST/IP/minuto, 120 GET/IP/minuto e 1000 requisições/minuto, usando RemoteAddr sem confiar em X-Forwarded-For arbitrário. Produção com proxy/múltiplas instâncias precisa política de edge e proteção contra abuso.

WhatsApp, consentimentos, outbox/lembretes, administração completa da agenda e novos estados operacionais permanecem próximos incrementos, não funcionalidades deste contrato.
