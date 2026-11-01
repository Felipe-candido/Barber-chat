# ADR 0040 — Conexão WhatsApp por barbearia e reconciliação ativa

Data: 2026-11-01. Status: implementado no backend; pareamento de telefone pendente.
Complementa os ADRs 0038 e 0039, entrega C.

## Contexto

O proprietário precisa conectar um remetente por unidade antes de habilitar
confirmações e lembretes. Um QR é credencial temporária, e operações na Evolution
podem terminar depois que a requisição ou o processo local falha. Webhooks podem
ser perdidos, repetidos ou chegar fora de ordem. Conexão não comprova entrega de
mensagem nem confirmação de presença.

## Decisão

Manter gestão de conexão em notifications: ela atende ao transporte de mensagens,
não à identidade Supabase nem à agenda. Identity implementa a porta consumidora
ConnectionOwnerAccess; sua consulta exige usuário, vínculo owner e unidade ativos,
sob locks compartilhados na mesma transação do repositório. HTTP também exige
owner, mas essa verificação não substitui a autorização na aplicação/persistência.

Migration 00022 cria uma linha por unidade, com nome remoto opaco único, intenção
de conectar, estado observado, geração, lease, token da operação e datas. RLS e
revogação de grants impedem acesso pela Data API do navegador. Não gravar QR,
sessão, telefone, perfil ou chave Evolution nessa tabela.

Uma transação curta autoriza e reserva uma operação por 90s. A chamada HTTP ocorre
depois do commit, sem segurar locks. A gravação do resultado exige a mesma unidade,
geração, token e lease ainda válido. Resultado atrasado não sobrescreve uma decisão
nova. Antes de devolver QR, reler autorização e geração. Falha de transporte vira
unknown com categoria sanitizada; não afirmar logout/conexão sem observação válida.

Um monitor independente do RabbitMQ consulta trabalho vencido com SKIP LOCKED,
até dez unidades por ciclo de 5s e deadline total de 12s. Cada resultado agenda
nova consulta em 60s. Depois de 2min, ou diante de erro, estado conectado deixa de
ser apresentado como conexão atual. Uma intenção de desconectar é reaplicada
quando a observação comprova que ainda há sessão; não recriar instância nem gerar
QR automaticamente pelo monitor.

Webhook usa token HMAC derivado do segredo servidor e nome da instância em
X-BarberChat-Evolution. É autenticação por token compartilhado, não assinatura do
corpo. Não confiar no campo apikey ou no estado do JSON. Evento autenticado apenas
marca uma reconsulta durável, coalescida por unidade; polling recupera evento perdido.
Não aplicar eventos de entrega de mensagens nesta fase.

Rotas owner sob /api/v1/admin/shops/{slug}: GET /whatsapp/connection,
POST /whatsapp/connect e POST /whatsapp/disconnect. Connect devolve metadados e QR
opcional em resposta no-store, com prazo local de descarte de 20s (o provedor pode
rotacioná-lo antes). Não criar endpoint GET que inicia pareamento. Requests não
aceitam nome remoto, URL ou segredos do navegador. Webhook tem corpo limitado e
rota separada, sem Bearer Supabase.

EVOLUTION_* é configuração opcional e exclusivamente servidor. Instalação ausente
não impede agendamentos. Um listener opcional só de webhook permite callback do
Docker para Windows sem mudar o binding administrativo existente em loopback.
Segredos não aparecem em erros, logs, OpenAPI, URL de callback ou frontend.

## Ambiente do provedor e compatibilidade

Compose isolado em infra/evolution mantém PostgreSQL, Redis e sessões próprios,
preserva volumes e publica Evolution somente em 127.0.0.1:8081. Inicializador cria
um .env ignorado com chaves aleatórias, sem alterar o .env do backend.

Fixar fonte oficial em e273b904d53f5726970fd6a244ed9caa61dfeb9a, incluindo lockfile
com Baileys 7.0.0-rc13. Esta é uma revisão de desenvolvimento para teste local,
não certificação de prontidão para produção. Dockerfile copia prisma.config.ts,
preserva o client gerado e executa Prisma diretamente sem imprimir DSN.

Nessa revisão, a rota de criação recebe parâmetros vazios como instância e ignora
o DTO validado. compatibility.cjs altera somente a chamada dessa rota para usar
o DTO já validado, verificando correspondência exata antes do build. Não remove
sanitização, autenticação ou proteção das demais rotas. Atualização da revisão
exige revisar/remover esse patch e repetir os testes de contrato reais.

O guard CORS do provedor rejeita inclusive chamadas sem Origin com allowlist
restrita. Usar seu padrão '*' apenas no ambiente local em loopback, sem cookies e
com apikey obrigatório. Não copiar essa configuração como arquitetura de exposição
pública. A ativação exigida pelo fornecedor foi feita manualmente pelo usuário;
seu estado é persistido no ambiente local.

## Verificação e limites

Testes unitários/HTTP cobrem owner/barber/outro tenant, revogação durante I/O,
QR privado, erro de ativação, redirecionamento, criação reutilizável e logout.
PostgreSQL descartável comprova busy, CAS, lease expirado, dois monitores, RLS e
webhook que não altera estado diretamente. Migration validada up/down/up local.

Teste opt-in com Evolution real ativa gerou QR e recebeu callback autenticado de
Docker para listener temporário no host. Instância aleatória removida ao final,
sem parear celular nem enviar mensagens. Pareamento e desconexão de telefone real
continuam pendentes. Não aplicar migration no Supabase nem sobrescrever .env local
como consequência desses testes. Não foram implementados frontend, sender, outbox
ou consumer; próximos passos permanecem D–G do plano.

Passaram gofmt, go mod tidy, go vet ./..., go test ./..., builds de API/worker,
validação Goose/Compose e parser YAML do contrato. Parte da suíte reutilizou
cache; o teste Evolution real foi executado sem cache. Evolution permanece
rodando para o usuário; o PostgreSQL descartável é parado sem remover volumes.

Guia operacional e fontes: [evolution-local.md](../evolution-local.md).
