# ADR 0041 — Núcleo de entrega compartilhado e resultado incerto

Data: 2026-11-01. Estado: D1 implementado e validado localmente com remetente
simulado. Complementa ADRs 0038–0040 e seção 10.1 do plano de notificações.

## Contexto

A primeira hospedagem pretendida é a API na Render Free, sem worker executando.
A confirmação será tentada após o commit de reserva e o lembrete partirá de um
comando na agenda. Na fase posterior, um consumer automático chamará a mesma
lógica. Esta entrega constrói e testa o núcleo, sem conectar o fluxo HTTP de
booking, expor rota de envio ou efetuar chamadas de mensagem à Evolution.

Não é possível obter atomicidade entre PostgreSQL e WhatsApp. Queda após início
da tentativa pode significar mensagem enviada com resposta perdida. Um lease
vencido não comprova ausência de efeito externo.

## Decisão

DeliverNotification recebe referência interna (shop_id, job_id), transactor,
PayloadOpener e NotificationSender. Não recebe telefone, código ou texto arbitrário
do navegador. Endpoint administrativo futuro deve autorizar proprietário ou
barbeiro responsável antes de entrar nesse núcleo; ele ainda não existe.

O caso de uso usa três transações curtas:

1. Obter referência, bloquear/levar em conta dados atuais, reclamar job com token
   aleatório e lease de 90s. Só então obter snapshot decifrado fora da transação.
2. Repetir locks e verificações, confrontar snapshot com reserva/contato atuais,
   validar o token/lease, registrar tentativa sending e fazer commit. Só depois
   chamar o sender. Erro de commit não autoriza envio, mesmo que tenha persistido.
3. Persistir resposta normalizada da chamada externa, com contexto de limpeza
   independente do cancelamento do navegador e limitado a 5s.

O relógio de decisão é clock_timestamp(), consultado depois dos locks, sem
depender da hora do dispositivo ou usar o instante de começo da transação.
A ordem é shop → profissional → reserva → permissão/contato → política → conexão
→ job → tentativa. Primeiro encontrar o profissional sem lock e depois bloquear
sua agenda mantém o protocolo de booking. Se a referência mudar durante a espera,
não continuar com uma reserva/profissional diferente do bloqueado.

Shops, Customers e Booking implementam portas consumidoras de notifications
com as mesmas Queries da transação. A aplicação não importa seus domínios ou
infraestruturas. Revalidar unidade ativa, política habilitada, autorização válida,
cliente correto, reserva ativa/futura, janela de envio, telefone/snapshots e
observação de conexão connected recente (menos de 2min), sem erro e com intenção
de conectar. O núcleo não considera que estar conectado garante sucesso do envio.

## Estados e persistência

Migration 00023 preserva notification_jobs.state como intenção pending/invalidated
e adiciona delivery_state, token, lease, contador e razão sanitizada. Misturar
cancelamento da reserva com confirmação de entrega apagaria informação relevante.

| delivery_state | Significado |
| --- | --- |
| ready | Sem tentativa externa; pode reclamar se elegível e vencido |
| leased | Operação reservada; não chamou sender; pode recuperar após lease |
| sending | Início persistido; pode ter efeito externo; lease vencido vira unknown |
| accepted | Provedor aceitou e retornou ID válido; não comprova entrega/leitura |
| failed | Resposta explícita comprova que não foi enviado; sem retry automático nesta entrega |
| unknown | Sem evidência suficiente; nunca repetir automaticamente |
| expired | Prazo encerrado antes do início |
| invalidated | Intenção revogada, dados incompatíveis ou payload inutilizável antes do início |

notification_attempts guarda token, unidade/job, número, instância opaca, horários,
resultado, ID do provedor quando conhecido e categoria de erro. Não guarda nome,
telefone, código, texto, QR ou resposta bruta. FKs compostas impedem vínculo entre
tenants; UNIQUE por job/número e por mensagem/instância impedem histórico duplicado.
RLS e grants protegem o histórico contra leitura pelo navegador.

Trigger na invalidação da intenção também invalida ready/leased, limpando o
claim. Não altera sending/accepted/unknown: cancelamento depois da decisão de
início não consegue retirar uma mensagem externa. O resultado dessa tentativa
continua sendo persistido, inclusive depois do cancelamento.

Lease antigo não inicia tentativa nova. Expiração de sending registra unknown
no job e tentativa, sem reenviar. Evidência conclusiva tardia pode resolver
unknown → accepted somente para o token exato da tentativa. Não permitir
accepted → unknown nem unknown → failed por resposta atrasada. A origem/verificação
de evidência de provider será implementada no adaptador real; este port não é rota
pública e um token não deve ser aceito como prova de entrega do navegador.

Respostas ausentes, inválidas ou contraditórias viram unknown. Erro não tipado
do sender também vira unknown, descartando texto privado. Somente resultado
explícito not_sent sem erro permite failed. D1 não implementa retries; a política
limitada de recuperação de falhas comprovadas chegará com o transporte posterior.

Cancelamento da requisição durante preparação libera o claim ainda não iniciado.
Falha de integridade/abertura do payload com contexto ativo bloqueia o trabalho,
sem chamada externa. Falha de persistência após sender não libera outra tentativa:
a recuperação posterior vê sending e conclui unknown. Não prometer exactly-once
externo; preferir revisão de um resultado incerto a duplicação automática.

## Escopo executável e próximos passos

infra/simulation/Sender usa resultados determinísticos sem rede ou retenção de
conteúdo. Não há fallback simulado na composição de produção. API e worker não
instanciam DeliverNotification ainda, logo esta migration não habilita envios.
Não há nova variável de ambiente, endpoint ou mudança frontend/OpenAPI nesta fase.

O modo manual precisará planejar janela e validade do snapshot próprias, conforme
seção 10.1. D1 consome os jobs/janelas existentes, sem ampliar a retenção de um
ciphertext já autenticado. Reservas antigas não recebem backfill.

Próximo passo E1: sender Evolution real e template, com telefone de teste. Depois
F1: conexão/configuração e consentimento no frontend, integração pós-commit e
lembrete manual autorizado. D2: scheduler/outbox/AMQP, inicialmente fora do deploy
Render. O executável worker permanece na fundação existente.

## Validação

Migration validada up/down/up em banco novo, local e descartável, sem auth schema,
Supabase ou .env do usuário. Testes PostgreSQL reais exercitam cliques concorrentes,
ausência de locks durante sender, cancelamento antes/durante I/O, revogação,
política, mudança de telefone, desconexão, expiração e isolamento.

Injeção de falha diferencia commit com resposta perdida antes do envio de rollback
após o provedor aceitar. Job iniciado não é reenviado, claim não iniciado pode ser
recuperado e evidência tardia é persistida no token correto. Testes cobrem erro de
payload e cancelamento do contexto; testes de domínio/formatadores protegem a
classificação conservadora e a privacidade. Nenhuma mensagem real foi enviada.

Passaram gofmt, go mod tidy, go vet ./..., go test ./..., builds de API/worker,
Goose validate, Compose e diff check. Integrações de delivery e regressões de
planejamento passaram sem cache; parte da suíte unitária reutilizou cache.
Supabase e .env do backend não foram alterados. O PostgreSQL descartável foi
parado ao final, sem apagar bancos/volumes; a Evolution do usuário ficou ligada.
