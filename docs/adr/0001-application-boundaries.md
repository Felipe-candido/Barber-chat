# ADR 0001 — Um módulo Go, dois executáveis

Status: adotada para o scaffold. Data: 2026-09-17.

## Contexto

Um desenvolvedor, MVP pequeno e objetivo de aprender Go e processamento assíncrono. O remote existente define `github.com/Felipe-candido/Barber-chat`. Não há frontend nem código anterior.

## Decisão

Um módulo Go e uma base de código, com `cmd/api` e `cmd/worker` implantáveis separadamente. Evoluir como monólito modular organizado por negócio, usando camadas curtas e ports/adapters nos limites externos quando houver caso de uso real. API não chama provider de mensagens nem depende do RabbitMQ na transação de agendamento. Worker é outro runtime da mesma aplicação, não um microsserviço.

## Consequências

Refatorações e versões são coordenadas; escala de processos pode ser independente. Schema e releases permanecem compartilhados. Não criar interfaces por tabela nem camadas vazias. Múltiplos módulos, repositórios ou serviços passam a fazer sentido com equipes autônomas, contratos estáveis, ciclos de release distintos ou necessidade comprovada de isolamento. Comparação detalhada em [architecture.md](../architecture.md).
