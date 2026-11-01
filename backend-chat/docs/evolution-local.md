# Evolution local e conexão por barbearia

Entrega C, 2026-11-01. Integração de conexão implementada; mensagens de agendamento
continuam nas entregas D/E. O painel de configuração está previsto na entrega F.

## Ambiente separado

Executar a partir de backend-chat:

```powershell
.\infra\evolution\Initialize-Local.ps1
docker compose --env-file .\infra\evolution\.env -f .\infra\evolution\compose.yaml up -d --build --wait
```

O script cria apenas infra/evolution/.env, com credenciais aleatórias locais, se
ele ainda não existir. Preserva arquivos existentes e não imprime segredos.
Não altera o .env do backend. Os dois arquivos .env permanecem ignorados no Git.

Compose tem projeto/rede/volumes próprios: Evolution, PostgreSQL e Redis.
Somente a porta 127.0.0.1:8081 é publicada. O PostgreSQL da Evolution não é o do
Barber-chat e nunca deve receber suas migrations Goose. O Redis atende o provider;
os jobs do produto continuam no banco do backend. Não usar down -v para reiniciar.

O build fixa o commit oficial e273b904d53f5726970fd6a244ed9caa61dfeb9a,
com Baileys 7.0.0-rc13 no lockfile. Node/Redis usam digests fixos. É uma revisão
de desenvolvimento para homologação local, sem declaração de prontidão para AWS.
O build completo inclui prisma.config.ts e mantém o gerador oficial e o alias
Prisma do projeto. Não substitui o mecanismo de licenciamento.

compatibility.cjs corrige uma regressão concreta dessa revisão: POST /instance/create
precisa repassar o DTO já validado, pois a sanitização de parâmetros de rota
remove instanceName e essa rota não tem parâmetro de instância. O patch confere
o trecho esperado antes de alterar; falha se o contrato upstream mudar. Os guards
de autenticação, licença e proteção das outras rotas ficam preservados.

A configuração CORS padrão '*' do fornecedor é usada somente neste serviço local,
com porta em loopback, autenticação por chave e sem cookies. A allowlist restrita
do código upstream rejeita também requisições servidor-servidor sem Origin e
navegação ao gerenciador. Não reutilizar essa instalação como exposição pública.

## Ativação e gerenciamento

Abrir http://127.0.0.1:8081/manager/login e seguir a ativação no fornecedor. O
cadastro/condições do fornecedor pertencem ao operador. Não realizar cadastro
automaticamente. A chave local no arquivo infra/evolution/.env autentica esta
instalação; ela não substitui a licença. A ativação fica no banco próprio da
Evolution e deve sobreviver à recriação do container com volumes preservados.

Consultar GET /license/status retorna status da instalação. Inactive não é
desconexão do telefone. Durante a preparação inicial, a instalação retornou
LICENSE_REQUIRED; após a ativação feita pelo usuário, retornou status active.

As instâncias do Barber-chat serão criadas pelo backend, com nome opaco ligado
à unidade. A API gera o QR depois de um comando explícito do proprietário.
Pareamento só ocorre quando o usuário escaneia o QR no WhatsApp. Criar uma
instância arbitrária no gerenciador não a vincula automaticamente à barbearia.

## Configurar o backend quando for testar a conexão

Aplicar as migrations separadamente:

```powershell
. .\scripts\Load-Env.ps1
goose -dir .\db\migrations postgres "$env:DATABASE_URL" up
```

A entrega C criou 00022_shop_whatsapp_connections.sql. O desenvolvimento validou
up/down/up em banco local descartável; não aplicou migrations ao Supabase.

Configuração servidor, no .env do backend, preservando os valores existentes:

```dotenv
EVOLUTION_BASE_URL=http://127.0.0.1:8081
EVOLUTION_API_KEY=<valor EVOLUTION_API_KEY da instalação local>
EVOLUTION_WEBHOOK_SECRET=<outro segredo aleatório de pelo menos 32 caracteres>
EVOLUTION_WEBHOOK_URL=http://host.docker.internal:8082/api/v1/webhooks/evolution
EVOLUTION_WEBHOOK_ADDR=0.0.0.0:8082
```

O endereço 0.0.0.0:8082 é um listener opcional exclusivo de webhook, necessário
para Docker Desktop alcançar o host Windows nessa configuração. Ele aceita só
POST /api/v1/webhooks/evolution, autenticado por segredo derivado por instância.
Não expõe rotas administrativas. HTTP_ADDR pode continuar 127.0.0.1:8080.
Escolher outra interface/porta conforme a rede e as regras locais do operador.
Em deploy com proxy privado, o callback pode apontar para a rota do listener
principal e EVOLUTION_WEBHOOK_ADDR pode ficar vazio.

Nunca colocar os segredos em NEXT_PUBLIC_, URLs, logs ou mensagens de chat.
EVOLUTION_WEBHOOK_SECRET é diferente da chave do provider e da cifragem dos
agendamentos. Rotacioná-lo exige reconfigurar os webhooks das instâncias por
POST /whatsapp/connect, que reutiliza a instância existente.

Reiniciar o backend depois de configurar:

```powershell
go run ./cmd/api
```

A configuração da Evolution não habilita o envio nem altera
NOTIFICATION_PLANNING_ENABLED. São capacidades distintas.

## API de conexão

As rotas administrativas usam Bearer Supabase e acesso owner à unidade:

| Método/caminho após /api/v1/admin/shops/{slug} | Efeito |
| --- | --- |
| GET /whatsapp/connection | Lê estado persistido, freshness, erro sanitizado e instantes |
| POST /whatsapp/connect | Inicia/reutiliza pareamento; devolve QR temporário quando disponível |
| POST /whatsapp/disconnect | Persiste intenção de desligar e solicita logout, sem apagar a instância |

POST aceita corpo vazio ou objeto JSON vazio, sem aceitar instanceName, ShopID,
URL ou chave do navegador. A identidade da instância é gerada no servidor e não
muda com o slug. Não há GET /whatsapp/qr: a criação/atualização do desafio é um
comando explícito de conexão. Respostas usam no-store e não expõem tokens/nomes
remotos. O QR não é salvo em banco ou logs; discard_after é um prazo local de
descarte de 20s e o fornecedor pode rotacionar o desafio antes dele.

O handler revalida contexto/role e o repositório revalida usuário, membership e
loja ativos. A autorização é verificada novamente depois do I/O externo antes
de devolver um QR. Revogação durante a chamada impede sua exposição.

## Estado e concorrência

Uma linha shop_whatsapp_connections por unidade guarda apenas metadados, com
instância remota única. Claim persiste intenção, geração e token de operação,
com lease de 90s. A transação é encerrada antes de chamar Evolution. Outra ação
da unidade recebe 409 enquanto o lease está válido. Resultado só é gravado se
token/geração ainda coincidirem e o lease estiver válido.

Após timeout ou queda, a instância opaca é reutilizada. Não criar nomes novos
como retry. O reconciliador recupera leases expirados e converge ao último
desired_connected, inclusive repetindo logout quando uma ação externa antiga
reabrir a sessão depois de uma intenção de desligamento.

API consulta até dez unidades vencidas por ciclo de 5s, com deadline de 12s.
Cada verificação completa agenda a próxima em 60s. Após dois minutos sem uma
verificação válida, connected é apresentado como unknown/stale. Falhas do
provider não renovam verified_at. Na entrega D esse trabalho pode migrar de
processo sem alterar a persistência/contrato.

Webhook recebe apenas evento de conexão: header X-BarberChat-Evolution com HMAC
derivado por instância. Não é assinatura do corpo. Não confiar em apikey ou
estado informados no JSON. O evento somente marca dirty para nova consulta
ativa, com coalescência/limite de frequência. Eventos atrasados ou repetidos não
regridem a projeção diretamente. Polling recupera eventos perdidos.

Estado da conexão, aceitação de mensagem, entrega/leitura e estado do atendimento
são informações diferentes. A entrega C não envia confirmações/lembretes.

## Testes

Go tests cobrem contrato HTTP, autenticação por chave, ausência de redirecionamento,
erros de licença/credencial/protocolo, criação única, logout e privacidade do QR.
Rotas usam tokens ES256 verificados em fixtures para owner/barber/tenant e CORS.
PostgreSQL real cobre autorização, busy, CAS, lease expirado, dois monitores,
evento que não altera estado diretamente, revogação e RLS.

TestEvolutionLocalQRAndWebhook é opt-in e exige a instalação local ativa. Cria
uma instância aleatória, solicita QR sem parear e recebe o callback autenticado
num listener temporário. Remove somente essa instância de teste ao terminar.
Não substitui validação com um telefone pareado na barbearia real.

Validação em 2026-11-01: instalação ativa, teste real de geração de QR e callback
autenticado aprovado. Instância de teste removida; nenhum telefone foi pareado e
nenhuma mensagem foi enviada. Migration 00022 validada apenas em PostgreSQL local.
Decisões e limites: [ADR 0040](adr/0040-owner-whatsapp-connection-management.md).

Fontes: [revisão oficial](https://github.com/evolution-foundation/evolution-api/tree/e273b904d53f5726970fd6a244ed9caa61dfeb9a),
[ativação na release](https://github.com/evolution-foundation/evolution-api/releases/tag/2.4.0-rc1),
[aviso de segurança corrigido no Baileys](https://github.com/WhiskeySockets/Baileys/security/advisories/GHSA-qvv5-jq5g-4cgg).
