# 0049 — Pacote Docker para migração gradual à EC2

Data: 04/11/2026. Estado: pacote implementado; recursos AWS ainda não criados.

## Decisão

Manter frontend na Netlify e banco/autenticação no Supabase. A EC2 executará os
containers do backend, sem criar outro módulo Go ou microsserviço. A imagem
multi-stage usa Go 1.27.3 fixado por digest, CGO desabilitado, timezone embutido,
certificados CA e usuário numérico sem privilégios. API/worker não têm shell e
usam filesystem somente leitura. O contexto permite apenas fonte e migrations;
nenhum arquivo de ambiente, dump, chave ou binário local entra na imagem.

`infra/aws/compose.yaml` inicia somente API e Caddy. `compose.notifications.yaml`
acrescenta a integração Evolution e seu PostgreSQL/Redis privados. O perfil
`worker` acrescenta worker e RabbitMQ. A API segue independente do broker.
O PostgreSQL da Evolution não substitui o banco comercial no Supabase.

O Caddy foi ajustado para publicar somente a API: não existe container frontend.
Somente 80/443 são públicos. O IP fixo do Caddy na rede edge origina o /32
confiável da API. As três configurações de subnet/IP podem mudar em conjunto
caso haja colisão com redes existentes. Nenhum outro container usa esse IP.
Callback Evolution chega diretamente em api:8082; Caddy bloqueia esse caminho
e as sondas de saúde. A rede de dados do provedor e a do broker são internas.
API, worker e Evolution conservam saída à internet conforme suas dependências.

O manager Evolution fica ligado somente a 127.0.0.1:8081 no host. O acesso
administrativo na EC2 será via encaminhamento SSM; não publicar seu painel.
O CORS amplo do fornecedor permanece atrás desse isolamento e exige API key,
sem cookies. Licença/ID de instalação são persistidos em RuntimeConfig no
PostgreSQL do fornecedor; sessões e Redis também recebem volumes nomeados.

API é sondada por um pequeno executável HTTP local; worker continua expondo
readiness de negócio pelo heartbeat PostgreSQL existente (scheduler + AMQP).
Container iniciado não equivale a remetente saudável ou telefone conectado.
Restart policy atua na saída do processo; healthcheck sozinho não reinicia um
container unhealthy. Logs Docker têm rotação, sem habilitar logs de payload.

## Segredos, migrations e publicação

`Initialize-Environment.ps1` cria `.env` ignorado com quatro segredos aleatórios
próprios da infraestrutura. Recusa sobrescrever arquivo existente. Não importa
as credenciais locais, não ativa cobrança real e não gera outra keyring de
notificações: ao reutilizar banco, copiar a keyring COMPLETA e o ID ativo, para
continuar decifrando jobs antigos. Atualizar de forma coordenada API/worker;
somente gerar novas chaves torna os payloads anteriores ilegíveis.

Segredos são injetados apenas nos serviços consumidores, não em build args.
Worker não recebe chaves Supabase de administração nem Stripe. `docker compose
config` revela os valores interpolados: usar `config --quiet` para checagem.
O `.env` do host precisa de permissões restritas; Docker Compose não é um cofre.
A fase AWS definirá IAM/SSM e armazenamento de parâmetros com acesso mínimo.

Target Docker `migrate` contém Goose 3.28.0, só driver PostgreSQL, e SQL da mesma
revisão do código. É uma operação explícita via perfil tools, nunca um efeito
de cada restart. Verificar backup/compatibilidade antes de executar contra
Supabase. Downgrade financeiro live e rollback destrutivo continuam proibidos.

Publicar imagens com tags imutáveis/digests no ECR antes de usá-las na EC2; os
nomes aws-local são somente para construção/validação local. Preservar volumes
e backups, inclusive banco/sessões do fornecedor. Um volume EC2 não substitui
backup externo e uma única máquina é um ponto único de falha.

## Etapas e validação

1. Gerar ambiente, preencher conexão, domínio, origem Netlify e chaves públicas.
2. Construir API e migrate; aplicar migrations explicitamente no alvo correto.
3. Iniciar API/Caddy e validar HTTPS, identidade, autorização e Stripe em teste.
4. Adicionar arquivo notifications, importar keyring e conectar/ativar WhatsApp
   no manager privado. Confirmar envio controlado antes de habilitar automação.
5. Habilitar planning/delivery/automatic e perfil worker, verificar heartbeat.
   Conferir conexão/instâncias e callbacks: não executar dois provedores com
   a mesma sessão durante a troca, nem presumir que um volume novo contém a antiga.
6. Só após validação, atualizar URL da API na Netlify, destino webhook Stripe e
   encerrar a Render. Migrar nuvem não exige trocar para Stripe live.

`Test-Containers.ps1` cria projeto Docker local único, banco vazio sem Supabase,
chaves fictícias e TLS interno apenas nos testes. As URLs SQL da fixture são
literais; variáveis herdadas são substituídas e restauradas para evitar usar
um `.env` real. Verifica migrations, HTTPS/CORS, callback privado, heartbeat,
reconexão AMQP e persistência do PostgreSQL Evolution após recriação. Containers
de teste são encerrados ao final, volumes preservados, sem pareamento/envio real.
Certificado público, DNS, ativação e recebimento num telefone serão verificados
na fase operacional; não confundir health HTTP do fornecedor com essas evidências.
