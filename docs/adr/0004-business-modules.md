# ADR 0004 — Módulos de negócio com casos de uso explícitos

Status: limites de negócio mantidos; organização das camadas substituída pelo [ADR 0005](0005-explicit-module-layers.md). Casos de uso ainda não implementados. Data: 2026-09-17.

## Contexto e decisão

O usuário pediu o início da estrutura física de módulos e casos de uso, após o scaffold executável. Manter a arquitetura mista: monólito modular, camadas pequenas por capacidade e ports/adapters introduzidos quando houver um fluxo real. Preservar API e worker no mesmo módulo Go.

Criar `internal/modules/{shops,catalog,customers,booking,notifications,identity,reporting}`. Cada módulo começa com `doc.go` na raiz e `application/doc.go`: o primeiro define domínio, ownership e invariantes; o segundo mapeia casos de uso e sua coordenação. Esses arquivos têm a finalidade imediata de registrar os limites solicitados e podem ser consultados com `go doc`. Não fingem implementar funcionalidades.

| Módulo | Dono de | Casos de uso previstos |
| --- | --- | --- |
| shops | Barbearia, slug, estado ativo, dados públicos e timezone | Criar/editar barbearia, resolver página pública |
| catalog | Profissionais, serviços e associações | Manter catálogo, associar serviços, listar opções reserváveis |
| customers | Contatos locais ao tenant e consentimentos | Localizar/criar cliente na confirmação, registrar/revogar consentimento, consultas administrativas |
| booking | Agendamentos, snapshots e regras de agenda | Disponibilidade, criação/consulta, confirmação/cancelamento, conclusão/ausência, expediente/intervalos/folgas/bloqueios |
| notifications | Intenções, jobs, tentativas e resultados | Planejar/inutilizar lembretes, materializar vencidos, enviar, retry, callback e resposta do cliente |
| identity | Identidade administrativa, membership e autorização | Resolver identidade verificada, autorizar ação, conceder/revogar acesso e mudar papel |
| reporting | Consultas derivadas, sem escrita nos outros módulos | Resumos de atendimento, cancelamentos/ausências, receita de concluídos por período/serviço/profissional |

Casos de uso futuros são arquivos de ações dentro de `application`, não classes obrigatórias nem diretórios separados para cada ação. A raiz do módulo receberá os tipos/regras que precisarem existir. Não criar pasta domain repetitiva enquanto o pacote raiz desempenha bem esse papel. Reporting pode usar modelos de leitura simples.

## Coordenação e limites

Composição em cmd; HTTP, banco e mensageria permanecem adaptadores. Application depende de regras do próprio módulo e define portas pequenas para capacidades externas. Domínio não depende de Chi, pgx, AMQP, SDKs ou de outro domínio. `internal` não substitui revisão dos limites entre os módulos.

Transação de criação de agendamento também inclui cliente, consentimento, intenções/outbox e lembretes. A futura implementação concreta deve compartilhar uma transação; separar módulos não permite transformar cada etapa em commit independente. Reporting pode consultar tabelas de outros módulos com joins revisados, mas não modificá-las.

Não foram adicionadas dependências, tabelas, endpoints de negócio, autenticação ou fornecedores. Adapters e interfaces serão introduzidos quando tiverem uso, com testes reais. Essa decisão não altera a separação de deploy nem requer outro repositório.
