# Barber-chat

O repositório reúne duas aplicações independentes:

- [backend-chat](backend-chat/README.md): backend Go com API, worker, migrations e testes.
- [frontend-chat](frontend-chat/README.md): Next.js e Tailwind, com catálogo integrado à API e estrutura de agenda/chat em evolução.

## Backend

No PowerShell, a partir desta pasta:

```powershell
cd backend-chat
go run ./cmd/api
```

O backend lê o `.env` de `backend-chat`. Para iniciar o worker em outro terminal, entre nessa mesma pasta e execute `go run ./cmd/worker`. Execute também os comandos de Go, Docker Compose, sqlc e migrations dentro de `backend-chat`.

## Frontend

Em outro terminal, a partir da raiz do repositório:

```powershell
cd frontend-chat
npm.cmd install
npm.cmd run dev
```

## Organização e imports Go

```text
Barber-chat/
├── backend-chat/
│   ├── go.mod
│   ├── cmd/
│   ├── internal/
│   ├── db/
│   ├── tests/
│   └── compose.yaml
└── frontend-chat/
    ├── package.json
    ├── src/
    └── public/
```

O módulo Go continua sendo `github.com/Felipe-candido/Barber-chat`. Os imports são esse nome mais o caminho relativo ao `go.mod`, por exemplo `github.com/Felipe-candido/Barber-chat/internal/config`. A pasta física `backend-chat` não precisa entrar nos imports. Não há outro módulo Go nem necessidade de `go.work` para executar o backend a partir de sua pasta.
