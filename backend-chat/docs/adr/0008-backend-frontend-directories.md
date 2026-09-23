# ADR 0008 — Backend e frontend em diretórios próprios

Status: adotada. Data: 2026-09-22.

## Decisão

O usuário reorganizou o repositório em `backend-chat` e `frontend-chat`. O backend reúne `go.mod`, `cmd`, `internal`, configuração, migrations, scripts e testes. O frontend tem seu próprio package.json e permanece uma demonstração independente da API.

Mantemos o nome do módulo Go `github.com/Felipe-candido/Barber-chat`, conforme a convenção existente do projeto. Mover toda a raiz do módulo não altera os caminhos relativos de seus pacotes; portanto, os imports `github.com/Felipe-candido/Barber-chat/internal/...` continuam válidos. Não adicionamos outro go.mod, go.work ou replace.

## Consequências

Go, Compose, sqlc e migrations devem ser executados de `backend-chat`. O carregamento automático de `.env` continua relativo ao diretório de execução, agora `backend-chat`. As referências do sqlc e os links entre documentos do backend continuam relativos à mesma raiz.

Essa organização serve ao desenvolvimento e build local do aplicativo. Caso futuramente o módulo precise ser distribuído com `go install` ou consumido remotamente como biblioteca, revisar a correspondência entre o nome do módulo e seu subdiretório no repositório. Isso não faz parte desta mudança.
