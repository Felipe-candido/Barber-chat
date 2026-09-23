# ADR 0005 — Domain, application e infra em cada módulo

Status: adotada a pedido do usuário. Data: 2026-09-17.

## Decisão

Explicitar três pacotes em cada um dos sete módulos: domain, application e infra. A decisão anterior usava a raiz como domínio; mover os contratos existentes para domain e preservar os casos de uso em application. Adicionar contratos de infraestrutura específicos para cada módulo, sem repositórios falsos nem interfaces sem consumidor.

Domain contém conceitos e invariantes de negócio. Application coordena ações e define portas pequenas para os recursos externos necessários. Infra implementa adaptadores de entrada (HTTP, AMQP, webhook) e saída (PostgreSQL, provider), conforme cada fluxo exigir. Infra pode depender de application/domain; o contrário não é permitido. Application pode depender do próprio domain. Pacotes domain não dependem uns dos outros.

O formato não é exigência de Go ou definição completa de Clean Architecture. É uma convenção explícita para facilitar o aprendizado e a localização de código neste monólito modular. Nomes iguais de pacote em módulos distintos são válidos: o import path os distingue. Usar aliases como bookingdomain quando o contexto precisar de clareza.

## Infraestrutura compartilhada

`internal/platform` mantém pools e conexões genéricos, sem SQL de negócio. `internal/httpapi` mantém servidor/roteador global; `internal/worker` mantém ciclo de vida do worker. `cmd` continua montando essas peças. Os adaptadores dos módulos recebem pool/transação/conexão por composição e não duplicam clientes globais.

Exemplo futuro: booking/application/CreateAppointment utiliza portas definidas pela aplicação; booking/infra implementa o acesso SQL e coordena uma transação concreta compartilhada com as capacidades de cliente e notificação. O domínio nunca recebe pgx.Tx. A separação de diretórios não muda os requisitos de atomicidade, idempotência ou isolamento por tenant.

## Estado da implementação

Há um doc.go em cada camada (21 arquivos nos sete módulos). A finalidade imediata é registrar ownership, regras de dependência e trabalho esperado para a estrutura solicitada. Não existem entidades, casos de uso ou adaptadores de negócio executáveis. Não criar subpastas adicionais de HTTP/PostgreSQL/provider antes de implementar seus consumidores reais. Nenhuma mudança nos binários, endpoints, migrations ou dependências é necessária nesta etapa.
