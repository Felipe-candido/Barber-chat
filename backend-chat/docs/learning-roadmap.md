# Trilha prática — atendimentos, relatórios, IA e AWS

Atualizada em 12/10/2026. Este é um roteiro de estudo/implementação, não uma lista de recursos já funcionando.

## Como usar a trilha

Faça uma entrega por vez. Em cada sessão: leia os arquivos indicados, explique a regra com suas palavras, escreva exemplos, implemente uma camada, rode testes e só depois conecte a próxima camada. Ao pedir revisão, indique a etapa e envie o código/resultado sem credenciais. Não precisa acompanhar um prazo em semanas: avance pelo critério de conclusão.

Quatro trilhas: produto (`P`), nuvem (`C`), IA (`I`) e mensageria opcional (`W`). Ordem recomendada: **P0 → P1 → P2 → P3 → P4 → I1 → I2**. **C0 → C1 → C2 → C3** pode ocorrer em paralelo desde P0, sem esperar a API ficar completa. W1 pode seguir P4 e não é pré-requisito dos relatórios. Recebimentos efetivos (`P5`) são opcionais, após decisão explícita.

Regras gerais: não criar CRM; não duplicar agendamentos; não editar migration aplicada; não testar contra o Supabase real; não publicar dados/segredos nem contratar AWS/IA sem definir orçamento. Não usar `down`, apagar volumes ou recriar contas para atualizar o sistema.

## Mapa de responsabilidades

| Parte | O que controla |
| --- | --- |
| `booking` | Reserva, calendário, snapshots, cancelamento e futuras transições de atendimento |
| `reporting` | Histórico e indicadores por consultas somente leitura |
| `identity` | Login local, unidade autorizada e proprietário/barbeiro |
| `customers` | Contato local já usado ao reservar, sem painel de cadastro/CRM |
| `notifications`/worker | Futuro envio e processamento durável; não gera relatório financeiro |
| IA | Interpretação/explicação; nunca autoridade sobre dinheiro, agenda ou acesso |

O módulo reporting já existia com `doc.go`. A base agora contém:

```text
internal/modules/reporting/
  domain/
    errors.go              erro de período inválido
    period.go              datas da unidade → intervalo UTC validado
    status.go              projeção dos status, sem transições
  application/
    dto.go                 entradas, filtros, cursores e resultados
    ports.go               contratos somente leitura e normalização de busca
    errors.go              erros previstos para os próximos casos de uso
  infra/
    doc.go                 existente, não modificado
  tests/domain/
    period_test.go         datas inválidas, limites, fusos e DST
    status_test.go         status conhecidos e desconhecidos
```

`application` não importa booking, pgx, HTTP ou SDK. Não há casos de uso, queries ou endpoints de relatório ainda. Subpastas `infra/http`, `infra/postgres`, testes de application e queries serão criadas junto das implementações concretas, sem stubs “não implementado”.

## P0 — Definir exatamente o que o painel mede

**Aprenda:** snapshot, comando versus consulta, centavos, moeda, intervalo semiaberto e diferença entre reservado, realizado e recebido.

**Leia:** `internal/modules/booking/domain/appointment.go`, `db/migrations/00005_booking_flow.sql`, os arquivos novos de reporting e [ADR 0014](adr/0014-reporting-read-models-and-learning-path.md).

**Faça:** escreva cinco reservas fictícias: duas concluídas de R$50 e R$80, uma agendada de R$60, uma cancelada de R$40 e uma ausência de R$70. Resultado esperado: dois realizados, R$130 de serviços realizados, R$60 previsto e ticket de R$65. Caixa recebido ainda é desconhecido, não R$130 automaticamente. Troque o preço atual do catálogo e verifique que o resultado histórico não muda.

**Decisões iniciais:** período pela data agendada (`starts_at`), fuso vindo da unidade, até 366 dias, proprietário apenas, filtros por nome/telefone/profissional/serviço e status no histórico. Nomes de serviços/profissionais são snapshots; contato é o registro atual. Telefone normalizado ajuda a buscar, mas pode ser compartilhado ou trocado: não identifica uma pessoa com certeza nem libera acesso público.

**Valide agora, em `backend-chat`:**

```powershell
go test ./internal/modules/reporting/tests/... -v
go test ./tests/architecture/...
```

**Concluído quando:** você consegue explicar por que reporting não altera agendamentos e por que passado não significa realizado. Fundação: implementada; testes devem ser reexecutados após mudanças.

## P1 — Registrar conclusão e ausência no booking

**Aprenda:** máquina de estados, idempotência, versionamento, auditoria e transação.

**Leia:** `booking/domain/appointment.go`, `application/manage_appointment.go`, `application/ports.go`, `infra/postgres`, `identity/infra/postgres/booking_access.go` e os testes de cancelamento/concorrência.

**Faça em fatias:**

1. Decida quando permitir conclusão/ausência: proposta mínima, somente após início e ação explícita. Não interpretar fim do bloco reservado como hora exata do corte. Defina correção de erro separadamente; inicialmente não reabrir estados finais.
2. Escreva testes puros para `scheduled/confirmed → completed/no_show`; negar antes do início, negar cancelado, repetir mesmo resultado sem nova versão, negar troca entre estados finais.
3. Implemente regras no domínio de booking e um caso de uso administrativo. Proprietário pode operar a unidade; se habilitar barbeiro, somente o profissional do escopo autorizado, nunca ID que ele escolher livremente.
4. Crie nova migration para auditoria/metadata decidida: ator, timestamp, estado anterior/novo e versão; FKs tenant-scoped. O valor antigo `updated_at` não reconstrói uma auditoria.
5. Escreva SQL parametrizado. Mantenha lock do profissional, revalidação do acesso dentro da transação e update por versão/estado esperado. Status e auditoria gravam juntos ou nada grava. Use ordem de locks compatível com booking.
6. Rode `sqlc generate`, implemente adaptador e só então handler/rota/bootstrap/OpenAPI. Rota sugerida: `POST /api/v1/admin/shops/{slug}/appointments/{id}/outcome`, ainda inexistente.

**Teste:** cancelamento versus conclusão concorrentes; duas conclusões; outro tenant; barbeiro de outro profissional; usuário revogado; relógio antes do início; rollback da auditoria. Não atualizar reservas antigas em massa para “completed”.

**Comandos após criar a migration, somente em banco local descartável:**

```powershell
goose -dir db/migrations validate
# Carregue uma DATABASE_URL do banco descartável, não o .env do Supabase.
goose -dir db/migrations postgres "$env:DATABASE_URL" up
sqlc generate
go test ./internal/modules/booking/tests/...
```

**Concluído quando:** uma reserva pode ser concluída/registrada como ausência, com regra, autorização, replay seguro e auditoria comprovados. Só enum/coluna já aceitar status não basta.

## P2 — Histórico privado com busca

**Aprenda:** projeções SQL, join tenant-scoped, keyset pagination, normalização de telefone e índices.

**Faça:** crie primeiro `application/list_appointment_history.go` com leitores falsos de teste. Valide tenant não nulo, datas, status, limite e cursor. Normalize telefone pelo contrato próprio, adaptado à capacidade existente de customers; normalização não cria contato. Depois crie `db/queries/reporting.sql`, gere sqlc e implemente o leitor SQL.

**Regras SQL:** `a.shop_id` obrigatório, joins de customers com `shop_id` + ID, histórico inclui recursos hoje inativos; não usar filtro de serviço ativo para apagar o passado. Nome pode usar busca textual parametrizada e com `%/_` tratados conforme o contrato. Telefone: igualdade E.164, não busca arbitrária por últimos dígitos. IDs estrangeiros nunca retornam outra unidade. Ordene `(starts_at,id) DESC`, limite máximo 100 e busque um item extra para emitir próximo cursor. Cursor só vale para o mesmo recorte; mudar filtro reinicia paginação.

**HTTP proposto:** `GET /api/v1/admin/shops/{slug}/history?from=2026-10-01&through=2026-10-31`. Sufixos ainda não registrados. Bearer → usuário → unidade → proprietário → application → leitor SQL → projeção privada. Query string com telefone pode acabar em logs de proxy: redigir query strings/logs/APM e nunca registrar filtros ou resposta. Se não conseguir garantir isso, use endpoint de busca com corpo protegido e mesmas regras de logging.

**Teste:** duas barbearias com o mesmo telefone, página seguinte sem repetição, datas de fronteira, nomes com apóstrofo/percentual, status desconhecido, profissional inativo, zero resultados e filtro alterado. Nome/telefone ou UUID não autorizam histórico público.

**Concluído quando:** proprietário busca e pagina sem vazamento entre unidades; código de reserva não aparece na resposta. Testes ficam em `reporting/tests/application`, `reporting/tests/infra/http` e `tests/integration`, criados com os adapters.

## P3 — Indicadores gerenciais determinísticos

**Aprenda:** `COUNT`, `FILTER`, `SUM`, agrupamento, `COALESCE`, `EXPLAIN` e consistência de leitura.

**Faça:** implemente `application/get_management_overview.go`, usando os contratos já criados. Retorne contagens de status, valores previstos/realizados, série diária, agrupamento por serviço/profissional. Primeiro uma query ou uma transação curta read-only/repeatable-read para não devolver cartões e gráficos de estados diferentes. Não somar linhas multiplicadas por associações profissional-serviço.

**SQL/resultado:** `SUM(price_cents)` somente de snapshots; realizados só `completed`; previstos `scheduled/confirmed`; agrupamento por moeda e IDs estáveis; dia civil usando o fuso selecionado do servidor; labels de grupos vêm do snapshot mais recente do período. Ticket médio é calculado por moeda; sem realizados, exibir “—”. Não agrupar entidades pelo nome atual nem somar moedas diferentes.

**HTTP proposto:** `GET /api/v1/admin/shops/{slug}/reports/overview?from=...&through=...`, proprietário apenas. Cancelamentos/ausências são indicadores, não receita. Indicadores seguem os mesmos filtros de contato/serviço/profissional; status não deve esconder realizados arbitrariamente nos totais.

**Teste:** conjunto manual da P0, preços atuais alterados, duas unidades, duas moedas em teste de projeção mesmo que o schema hoje só aceite BRL, datas locais/UTC, zero/divisor zero, conclusão tardia e renomeação sem duplicar grupo. Compare soma dos grupos com total. Use `EXPLAIN (ANALYZE, BUFFERS)` somente no banco de teste; criar índices por plano/carga, não por superstição.

**Concluído quando:** cada número tem definição, origem e teste reproduzível. Não é necessário IA, worker, Redis ou warehouse.

## P4 — Ligar o painel sem aumentar a complexidade

**Aprenda:** estado assíncrono, contratos JSON, gráficos com dados reais e testes de navegador.

**Faça:** duas entradas do proprietário: “Histórico” e “Relatórios”. Histórico tem período, busca por contato, status e paginação. Relatórios tem três destaques (realizados, valor realizado, ticket), curva por dia e filtros; agrupamentos podem ficar abaixo. Evite dez gráficos, painel de permissões e cadastro de clientes. Barbeiro continua com a própria agenda; menu escondido não substitui autorização no backend.

**Arquivos futuros:** `frontend-chat/src/lib/api/reporting.ts`, componentes de histórico/relatórios e rotas dentro de `src/app/admin/[slug]`. Leia o `frontend-chat/AGENTS.md` e o guia Next instalado antes de editar.

**Teste:** loading/erro/vazio distintos, troca de unidade, token vencido, barbeiro negado, resposta atrasada não preencher unidade nova, centavos/BRL, período civil correto, mobile e estado vazio sem dados fictícios.

```powershell
cd C:\Users\felip\Barber-chat\frontend-chat
npm run typecheck
npm run build
npm test
```

**Concluído quando:** fluxo criar → concluir → localizar no histórico → reconciliar indicador funciona com dados reais de teste.

## P5 — Recebimentos reais, somente se você escolher

Antes de usar o rótulo “Recebido”, decidir pagamento integral/parcial, desconto, estorno, método e data. Modelo mínimo pode ser um registro de pagamento vinculado à reserva, sem gateway nem contas de clientes. Status do atendimento não é status financeiro: concluído pode não ter sido pago, e agendado pode estar pré-pago. Reporting lê o modelo; não registra pagamentos.

Primeiro exercício: corte de R$50, recebido R$45 hoje; valor dos serviços e caixa não coincidem. Projete ajustes auditados/idempotentes e soma por `paid_at`, sem sobrescrever snapshot da reserva. Avalie módulo financeiro apenas quando as regras efetivas exigirem, não nesta fundação.

## C0 — Preparar AWS com segurança e orçamento

**Pode começar agora. Aprenda:** EC2 (computação), IAM (permissões), VPC/security groups (rede), S3 (objetos) e CloudWatch (observabilidade). IAM da AWS não substitui o login Supabase nem o membership da barbearia.

**Faça:** proteja root com MFA; acesso cotidiano federado/Identity Center e credenciais temporárias; escolha região e teto mensal antes de criar recursos; orçamento/alertas de custo e etiquetas `project`, `environment`, `owner`. [Boas práticas IAM](https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html). Budgets usa dados de faturamento atualizados periodicamente: alerta não é trava de gasto instantânea. [AWS Budgets](https://docs.aws.amazon.com/cost-management/latest/userguide/budgets-best-practices.html).

Inventarie EC2, EBS, snapshots, IPv4 público, tráfego e logs; não assuma Free Tier. IPv4 público também tem cobrança. [Documentação VPC](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-ip-addressing.html). Escolha tamanho pelo build/runtime medido de Next + Go; não assumir que uma microinstância suporta tudo. Comece sem NAT Gateway, ALB, EKS ou GPU, evitando recursos sem necessidade no laboratório.

**Exercício local, quando o perfil abaixo tiver sido configurado por você:**

```powershell
aws --version
aws sso login --profile barber-staging
aws sts get-caller-identity --profile barber-staging
```

O nome é um exemplo de perfil a criar, não perfil detectado. Não envie as credenciais ou o resultado identificável para logs públicos. Cloud ainda não foi configurada nesta entrega.

**Concluído quando:** você sabe quem pode criar recursos, quanto aceita gastar e quais recursos revisar ao encerrar o laboratório.

## C1 — Empacotar e separar staging

**Aprenda:** imagem versus container, multi-stage build, variáveis em build/runtime, healthcheck e segredos.

**Faça:** Dockerfile Go da API, Dockerfile Next compatível com a versão instalada e Compose de deploy distintos do Compose atual (hoje ele sobe somente PostgreSQL/RabbitMQ). Usuário não root, imagens/versionamento fixados, `.dockerignore` excluindo `.env/.git`, tzdata disponível ao Go e shutdown correto. API precisa escutar `0.0.0.0:8080` DENTRO do container; isso não significa publicar a porta ao mundo. Só proxy deve expor HTTP/HTTPS; banco/broker/API em rede interna.

Use projeto Supabase staging separado para Auth + banco, com dados fictícios. TLS PostgreSQL verificado, conexão suportada pelo ambiente AWS e configuração de pool apropriada devem ser testados. Não copie clientes reais. Segredos ficam no servidor via secret store e role mínima; nunca `NEXT_PUBLIC_*`. Valores públicos Next de API/Supabase são build-time e exigem rebuild ao mudar. Convites exigem redirect staging configurado.

**Comandos futuros, somente depois dos Dockerfiles/Compose existirem:**

```powershell
docker build -f backend-chat/Dockerfile -t barber-api:staging backend-chat
docker build -f frontend-chat/Dockerfile -t barber-web:staging frontend-chat
docker compose -f deploy/compose.staging.yaml config --quiet
```

Definir passagem das variáveis públicas do Next no build conforme o Dockerfile, sem colocar chaves de servidor em build args. Esses caminhos ainda não existem; não executar como se já estivessem prontos.

**Concluído quando:** containers rodam localmente, sem `.env` na imagem e sem depender da instalação Windows; `/health`, `/ready`, login/convite e reserva de teste passam.

## C2 — Primeiro deploy manual em EC2

**Aprenda:** Linux, IAM role da instância, rede, TLS, persistência e recuperação.

**Faça:** após aprovar orçamento/tamanho, crie EC2 staging com role mínima e [Session Manager](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/connect-with-systems-manager-session-manager.html), agente e conectividade necessários. Prefira acesso administrativo sem abrir SSH ao mundo. Libere apenas HTTPS e HTTP para redirect/validação TLS; 3000/8080/5432/5672/15672 não devem ficar públicas.

Instale runtime de containers, configure proxy reverso/HTTPS e DNS, API + Next e credenciais staging. Uma origem pode encaminhar `/api/v1/*` ao Go e restante ao Next, desde que configuração/frontend combinem; testar também health/readiness. Defina `FRONTEND_ORIGIN` HTTPS exata e allowlist Supabase. Não iniciar worker/broker de negócio só para consumir RAM: agendamento/reporting não dependem deles nesta fase.

**Compatibilidade:** o projeto usa Next 16.3.6. A documentação Amplify consultada lista SSR até Next 15; não pressupor suporte nativo à versão atual nem fazer downgrade apenas para hospedar. [Suporte Amplify](https://docs.aws.amazon.com/amplify/latest/userguide/ssr-amplify-support.html). EC2 com Node/container evita essa dependência de suporte do hosting. S3 sozinho não executa Go nem SSR; export estático exigiria avaliar as rotas dinâmicas do projeto.

**Teste:** navegador externo em HTTPS, CORS/preflight, login/convite, isolamento tenant, reserva/consulta/cancelamento, reinício da instância, segredos ausentes nos logs e rollback para imagem anterior. `go run`/`next dev` não são processos de deploy. Staging de uma EC2 é laboratório, não alta disponibilidade/SLA de produção.

**Concluído quando:** você consegue implantar uma versão, verificar e voltar à anterior sem perda de dados. Ao parar laboratório, revisar EBS, snapshots, IPv4 e logs que podem continuar cobrando; não apagar dados sem backup e validação de alvo.

## C3 — Automatizar, observar e depois evoluir

**Aprenda:** CI/CD, OIDC, registro de imagens, migrations compatíveis e infraestrutura como código.

**Faça:** pipeline Go/testes/Next/build → imagens imutáveis por commit → registro ECR → deploy staging com aprovação. Autenticação temporária OIDC/role restrita, não access key permanente no repositório. Migrations são uma etapa explícita do release, com backup, plano de rollback/compatibilidade e sem `goose up` concorrente no boot de réplicas. Pratique expand/contract: voltar binário não implica rodar migration Down no banco real.

Logs JSON com request ID, retenção/custos, métricas de latência/5xx/readiness e alerta de custo. Depois codifique o laboratório em CloudFormation ou outra ferramenta escolhida, testando criação/atualização e remoção controlada. Antes de clientes reais: revisão de privilégios/RLS, backup e restore, TLS/proxy/rate limits, abuso, operação de segredos, auditoria e política de acesso/retenção.

**S3 opcional:** artefatos/exportações com acesso privado, criptografia e retenção; não uma segunda fonte de dados dos relatórios. [Bloqueio de acesso público](https://docs.aws.amazon.com/AmazonS3/latest/userguide/access-control-block-public-access.html). URLs temporárias também são credenciais. Exportação de dados pessoais só depois de autorização, limites e cuidado com CSV injection.

**Evolução após medir:** ECS/Fargate para containers, RDS para banco e broker gerenciado podem ser estudos separados. Hoje existe FK de `public.users` para `auth.users` no Supabase: RDS com Auth ainda no Supabase requer estratégia de provisionamento/desativação/reconciliação sem FK remota, migration e testes. Não é apenas trocar `DATABASE_URL`. Escolher uma mudança por vez, sem microserviços/GPU/cluster simultâneos.

**Concluído quando:** CI identifica regressão, deploy é reproduzível, restore foi exercitado e você consegue inventariar custos/recursos.

## I0 — Decidir onde IA agrega valor

**Ordem recomendada:**

1. Explicar relatórios prontos: piloto pequeno, somente leitura, fácil conferir contra números reais.
2. Interpretar pedido no chat e ajudar a escolher serviço/dia/profissional: maior valor potencial ao cliente, mas maior risco e mais avaliações.
3. Resumir instruções/respostas sem alterar templates transacionais sensíveis; mensageria básica não precisa de IA.
4. Previsão de demanda/ausência: depois de volume/dados representativos e avaliação de viés/qualidade; não antes.

Nenhuma IA é necessária para somar valores, desenhar gráficos, proteger horários ou gerar código. Não treinar modelo nem criar embeddings/RAG para tabelas estruturadas pequenas por padrão. Começar em Go com inferência por API; [Bedrock tem exemplos oficiais Go v2](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/go_bedrock-runtime_code_examples.html). Provedor/modelo/região e uso de dados precisam ser escolhidos/verificados no momento da implementação, sem contratação agora.

Python passa a fazer sentido com treinamento, processamento local ou bibliotecas específicas. Um serviço Python separado só quando houver carga/release/dependências próprias que compensem deploy, rede, autenticação e monitoração adicionais. Um worker Go pode separar latência/processamento sem dividir o domínio em microserviços.

## I1 — IA somente leitura para explicar o relatório

**Aprenda:** inferência, saída estruturada, timeout, custo, prompt injection e avaliação.

**Faça:** crie `ExplainManagementOverview` quando P3 estiver correto, com porta específica e adaptador inicialmente falso. Se síncrono, deadline curto e limites; se demorado, job durável processado pelo worker. Envie só indicadores agregados autorizados, período e definições. Nada de nomes, telefones, UUIDs de clientes, códigos, tokens, SQL ou credenciais. Mesmo agregados muito pequenos podem revelar informações: limitar contexto/uso e nunca tratar agregação como anonimização garantida.

Separe resultado de métricas (SQL) do texto de IA, mantendo números do servidor. O modelo pode explicar “cresceu em relação ao período anterior”, mas precisa de comparativo calculado e fornecido; não inventar causas, descontos ou projeções. Ele não soma “de cabeça”, altera status ou escreve SQL livre. Falha/limite deixa dashboard utilizável sem IA. Texto deve aparecer como resumo assistido, não verdade contábil.

Defina orçamento por unidade/período, limites de entrada/saída/chamadas, circuit breaker/fallback e telemetria de tokens/latência sem prompts com PII. Reavalie política de dados/retenção/logs do provedor; [proteção de dados Bedrock](https://docs.aws.amazon.com/bedrock/latest/userguide/data-protection.html). Mesmo modelo hospedado fora da API é acessado por um adapter, não chamado pelo domínio ou navegador com chave privada.

**Teste antes de modelo real:** tabela de casos (vazio, cancelamentos, valores iguais, moeda distinta, instrução maliciosa em label, timeout e resposta inválida). Depois medir fidelidade numérica, nenhuma PII, ausência de alegação causal inventada, latência/custo e validação manual. Versão de prompt/modelo deve ser rastreável. Não exigir saída textual idêntica em teste probabilístico.

**Concluído quando:** explicação ajuda, não modifica os números e sua falha não quebra o produto. Implementar controle de gasto no servidor, não depender só do alerta cloud.

## I2 — Agendamento conversacional com confirmação humana

**Aprenda:** tool/function calling, estado da conversa, schemas e limites de autoridade.

**Faça:** ferramentas permitidas: listar serviços públicos da unidade, listar profissionais elegíveis e consultar disponibilidade. O servidor injeta slug/tenant/autoridade; modelo não escolhe uma unidade arbitrária. Nunca fornecer SQL livre, shell ou ferramenta para ler contatos/histórico privado a uma conversa pública. Tool calling pede uma ação; o código valida/executa. [Documentação Bedrock](https://docs.aws.amazon.com/bedrock/latest/userguide/tool-use.html).

Fluxo: “corte amanhã à tarde” → IA propõe serviço/data → servidor resolve data no fuso da unidade e consulta horários reais → cliente escolhe → formulário coleta/revisa contato → cliente confirma → booking transacional confirma ou retorna conflito → recibo/código. Não depender de memória textual do modelo para idempotência/dados confirmados; estado do fluxo é controlado pela aplicação. Cancelamento continua exigindo código e confirmação explícita.

IA não reserva no momento da consulta e não pode contornar exclusion constraint/locks, alterar preço ou promover usuário. Pedidos vagos exigem perguntar; múltiplos serviços/opções devem ser apresentados sem inventar disponibilidade. Fallback é o fluxo guiado existente. Limites por conversa/IP/unidade, sessões opacas, retenção mínima de texto/contato e defesa contra prompt injection são parte da entrega.

**Teste:** amanhã próximo de meia-noite, fuso diferente do navegador, horário ambíguo, serviço inexistente/inativo, outro tenant, consulta de dado privado, horário tomado entre proposta e confirmação, timeout, double click, instrução “ignore as regras” e cancelamento sem código.

**Concluído quando:** comparação com fluxo guiado mostra ajuda mensurável e nenhuma violação de regra/acesso. Futuro ganho de escala deve ser medido antes de extrair serviço.

## W1 — Worker/WhatsApp, quando você retomar essa entrega

Sem IA inicialmente. Aprenda outbox, jobs persistidos, ACK, retries, DLQ e efeitos duplicados. Primeiro provedor falso; depois escolha canal, consentimento e fornecedor/custo. Criar intenção/outbox junto da reserva, publicar após commit, deduplicar consumer e revalidar status/versão antes de enviar. Cancelamento invalida lembretes; não prometer exactly-once externo. Relatórios e histórico não dependem de mensagens entregues. Esta etapa segue o desenho existente de notificações e requer revisão própria, não foi implementada aqui.

## Rotina de revisão e próximos passos imediatos

Em cada entrega, anote: regra, entrada/saída, dono dos dados, casos inválidos, concorrência, autorização, migration necessária, teste e como voltar o deploy. Explique cada arquivo antes de iniciar a camada seguinte. Use este pedido para orientar as sessões:

> “Vamos fazer só a etapa P1, começando pelos testes e regras do domínio. Explique cada arquivo ao terminar e aguarde minha revisão antes de conectar PostgreSQL e HTTP.”

**Agora:** execute os testes da P0, confirme se o primeiro painel mede valor realizado ou também pagamento e escolha C0 se quiser iniciar cloud em paralelo. Não começar a integração de IA antes de existir um indicador confiável.

**Checklist Go por entrega, dentro de `backend-chat`:**

```powershell
go mod tidy
go vet ./...
go test ./...
go build ./cmd/api ./cmd/worker
docker compose --env-file .env.example config --quiet
git diff --check
```

Rode gofmt nos arquivos alterados. Testes de integração `go test -tags=integration ./tests/integration/...` exigem env/bancos descartáveis explícitos e dependências conforme a suíte. Não aplicar esse comando carregando automaticamente o Supabase real; conferir alvo e executar somente os testes cujo ambiente foi preparado. A fundação atual não criou schema ou novas rotas, então não há migration nova para rodar agora.
