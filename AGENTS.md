# Instruções para trabalhar neste repositório

## Antes de editar

- Ler README.md, docs/architecture.md e os ADRs relevantes. Inspecionar arquivos, go.mod, git status e remote; preservar mudanças existentes.
- Nunca fazer commit/push sem pedido explícito. Não usar reset/clean, remover volumes, apagar dados ou reescrever histórico como parte de validações.
- Manter o module path derivado do remote: `github.com/Felipe-candido/Barber-chat`.
- O usuário usa Windows/PowerShell e está aprendendo Go, Chi, arquitetura e mensageria. Explicar decisões e preferir comandos diretos de Go/Docker.

## Arquitetura e escopo

- Um módulo Go, executáveis `cmd/api` e `cmd/worker`. Worker separado não é automaticamente microsserviço. Frontend é separado e ainda não existe.
- Usar Chi/net/http, pgx, amqp091-go e biblioteca padrão quando suficiente. Dependências novas exigem problema concreto, versão fixada e fonte oficial verificada.
- Organizar capacidades futuras por negócio, com camadas pequenas e ports/adapters só em fronteiras efetivamente utilizadas. Não criar interfaces genéricas, pastas vazias nem dezenas de stubs.
- A estrutura solicitada pelo usuário está em `internal/modules`: shops, catalog, customers, booking, notifications, identity e reporting. Cada módulo tem domain, application e infra; ler seus doc.go antes de implementar fluxos. Esses arquivos mapeiam o escopo; não indicam funcionalidade já executável.
- Domain receberá regras/tipos de negócio; application receberá casos de uso e portas pequenas; infra receberá adaptadores concretos do módulo. Domain e application nunca importam infra. Não gerar uma pasta por caso de uso nem adaptadores/interfaces vazios. Novos subpacotes de infra só com implementação concreta.
- A raiz dos módulos é um diretório organizador, sem outro go.mod. Conexões/pools genéricos continuam em internal/platform, servidor em internal/httpapi e composição em cmd. Não duplicar pool por módulo: injetar recursos ou transações compartilhadas nos adaptadores.
- Booking é dono do expediente e bloqueios; catalog dos profissionais/serviços; shops do slug/timezone; customers do contato/consentimento; identity do acesso administrativo. Relatórios são leitura derivada. Fluxos entre módulos precisam preservar a transação de agendamento.
- Domínio não deve importar HTTP, driver SQL, AMQP ou SDK externo. Composição fica em cmd. Runtime do worker pode conhecer transporte; caso de uso de envio deve depender de porta própria.
- Código, nomes e comentários em inglês; documentação pode ser em português.
- Não há portfólio ou galeria. Serviços realizados são agendamentos concluídos usados em relatórios.
- Não antecipar autenticação, CRUD completo, frontend, relatórios ou integração WhatsApp quando a tarefa pedir somente fundação.

## Invariantes para próximas funcionalidades

- Toda consulta/escrita de negócio deve estar vinculada à barbearia; tenant vem de slug resolvido ou membership autenticado, nunca de confiança em ID enviado pelo cliente. Usar FKs compostas e testar isolamento.
- Não persistir cliente incompleto durante o formulário público. Normalizar telefone, separar consentimento transacional/marketing e não registrar PII/segredos em logs.
- Disponibilidade anterior ao INSERT não garante reserva: usar transação e exclusion constraint por profissional/tenant/range, além do protocolo comum de lock para bloqueios/expediente.
- Instantes em UTC/timestamptz, timezone IANA por barbearia; preço em centavos com moeda; preservar snapshots de serviço/preço/duração.
- Agendamento, intenção de mensagem e jobs/outbox devem ser atômicos. Não publicar no broker antes do commit nem fazer envio externo no handler.
- Projetar delivery at-least-once, confirms, roteamento obrigatório, ACK após persistência, retries limitados, DLQ e deduplicação. Não prometer exactly-once externo.
- Cancelamento/reagendamento invalidam jobs; consumidor revalida estado/versão/consentimento. Scheduler persistido no PostgreSQL, sem timer em memória como fonte de verdade.
- Não adicionar plugin RabbitMQ de atraso sem justificar operação e portabilidade. Não escolher provedor pago ou separar repositórios sem decisão explícita do usuário.

## Validação e documentação

- Rodar `gofmt -w cmd internal tests`, `go mod tidy`, `go vet ./...`, `go test ./...`, builds da API/worker e `docker compose --env-file .env.example config --quiet`.
- Executar migrations e testes `go test -tags=integration ./tests/integration/...` quando as dependências estiverem disponíveis. Não declarar teste pulado/bloqueado como aprovado.
- Não editar migration já aplicada; criar outra. Migrations são executadas separadamente da inicialização da API/worker. Validar no PostgreSQL, não só no parser.
- Usar testes que comprovem comportamentos e falhas relevantes. Testes concorrentes devem usar PostgreSQL real; mock não prova integridade do banco.
- Manter OpenAPI dos endpoints reais e proposta de contrato consistentes. Registrar decisões significativas em ADRs e separar comportamento implementado de intenção futura.
- Não versionar `.env`, `.tools`, binários, logs ou credenciais. Não substituir valores locais do usuário ao atualizar `.env.example`.
- O usuário pediu a remoção de `.tools`. Não recriar a pasta nem instalar Go portátil: usar o Go instalado e os caches padrão retornados por go env. Ferramentas adicionais, como Goose, são externas ao código da aplicação.
- Ao concluir, relatar arquivos, decisões, validações, limitações e próximos passos. Não esconder erros de compilação.
