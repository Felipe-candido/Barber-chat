# ADR 0042 — Texto versionado e envio pela Evolution

Data: 2026-11-01. Estado: E1 implementado; contrato e persistência testados com
servidor HTTP controlado. Recebimento em telefone pareado ainda pendente.

## Decisão

application/message.go renderiza versão 1 para booking_confirmation e
booking_reminder a partir do snapshot protegido. Usa data absoluta e fuso IANA
da reserva, com offset do instante, e preço inteiro em centavos. Não depende de
catálogo atual, horário do servidor ou palavras relativas como amanhã. Conteúdo
inclui endereço, profissional, serviço e código privado; não repete telefone.
Normaliza separadores Unicode/controles de direção nos campos de exibição, mas
exige preservar o código exatamente. Objeto renderizado oculta conteúdo em
formatação/log comuns. Texto livre, marketing e URLs arbitrárias não são aceitos.

Migration 00024 acrescenta template_version ao histórico. Tentativas antigas
continuam NULL, sem atribuição retroativa fictícia. Start grava a versão antes
da chamada externa e o sender exige versão suportada. Não guardar texto legível
em notification_attempts para conseguir identificar qual versão foi utilizada.

infra/evolution/sender.go implementa NotificationSender no Client existente.
POST /message/sendText/{instance} envia somente number (E.164 sem +), text,
messageId, linkPreview=false e delay=0, com apikey no header. messageId é o UUID
da tentativa sem hífens em maiúsculas. O protocolo verificado aceita esse campo
e o encaminha ao Baileys; isso é correlação, não garantia de idempotência externa.
Não realizar retries ou seguir redirects; manter timeout de 8s do cliente.

Resposta aceita exige HTTP 200/201, JSON limitado a 64KiB, ID correlacionado,
fromMe=true, destinatário compatível e ausência de erro/status contraditório.
PENDING, SERVER_ACK ou até READ continuam significando apenas accepted no estado
interno dessa chamada. Não implementar comprovante delivered/read com esse POST.

Destinatário deve corresponder ao telefone E.164 esperado. LID só é aceito quando
remoteJidAlt também fornece o telefone esperado; grupo/broadcast não é aceito.
Mudanças de canonicalização (por exemplo, número brasileiro com/sem nono dígito)
não são presumidas equivalentes. Mapeamento sem prova retorna unknown; validar
esse comportamento com o telefone real antes de ativar a operação externa.

Somente envelopes reconhecidos de autenticação, instância ausente ou ativação,
originados nos guards da revisão fixada e sem evidência de mensagem, permitem
not_sent/provider_unavailable. Corpo inesperado, conexão perdida, timeout,
redirect, 429, 5xx genérico e HTTP 400 permanecem unknown. Na revisão conferida,
um catch depois do send pode produzir 400; status HTTP sozinho não comprova que
nada foi enviado. Erros/corpos privados nunca são repassados ou registrados.

## Verificação e limites

Testes de texto cobrem fuso, mudança de data/DST, centavos sem float, estrutura,
versão e privacidade. Contratos HTTP cobrem ID/destinatário/LID, envelopes de erro,
limites, corpo truncado, redirecionamento e perda da conexão após leitura do POST.
Testes com PostgreSQL real usam o sender HTTP e gravam template/ID/resultado;
repetição de resultado aceito ou incerto não faz outro POST.

Migration validada up/down/up somente em PostgreSQL descartável. Supabase,
.env do backend, README/doc.go e frontend preservados nesta entrega. Sender não
é ligado ao bootstrap até a integração pós-commit e as rotas/configurações de F1.
Nenhuma mensagem foi enviada a um telefone real nesta validação.

## Fontes verificadas

- [DTO na revisão fixada](https://github.com/evolution-foundation/evolution-api/blob/e273b904d53f5726970fd6a244ed9caa61dfeb9a/src/api/dto/sendMessage.dto.ts).
- [Rota sendText](https://github.com/evolution-foundation/evolution-api/blob/e273b904d53f5726970fd6a244ed9caa61dfeb9a/src/api/routes/sendMessage.router.ts).
- [Implementação e catch após envio](https://github.com/evolution-foundation/evolution-api/blob/e273b904d53f5726970fd6a244ed9caa61dfeb9a/src/api/integrations/channel/whatsapp/whatsapp.baileys.service.ts).
