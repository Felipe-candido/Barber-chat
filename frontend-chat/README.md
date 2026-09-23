# Barber-chat — frontend

Next.js, React, TypeScript e Tailwind. O catálogo está integrado à API Go; agenda e autenticação ainda aguardam seus contratos.

## Rodar

No PowerShell:

```powershell
cd frontend-chat
if (!(Test-Path .env.local)) { Copy-Item .env.example .env.local }
npm.cmd install
npm.cmd run dev
```

Configure NEXT_PUBLIC_API_BASE_URL com a URL HTTP da API e NEXT_PUBLIC_SHOP_SLUG com a barbearia local. No backend, habilite DEV_SHOP_SLUG e DEV_FRONTEND_ORIGIN=http://127.0.0.1:3000. Slugs do frontend e backend devem corresponder para o cadastro local.

Abra [Serviços](http://127.0.0.1:3000/admin/servicos) ou o catálogo público em /b/{slug}. /chat usa o slug do ambiente. /admin mostra a estrutura da agenda sem dados inventados. /login continua uma prévia sem sessão, validação ou armazenamento de credenciais; acessar o painel não significa autenticar.

## Integração

- Lista serviços ativos e cria serviços com os campos reais do handler Go.
- Exibe carregamento, lista vazia, erro, sucesso e envio pendente.
- Faz buscas locais sobre a lista recebida.
- Consome o slug da rota pública.
- Não usa serviços/agendamentos simulados, nem fallback em localStorage.
- Não permite editar/excluir/ativar serviços ou reservar horários sem rotas disponíveis.

[API-INTEGRATION.md](API-INTEGRATION.md) contém contratos, erros, configuração local, arquivos, limites e requisitos dos endpoints futuros. [Proposta de autenticação](../backend-chat/docs/authentication-proposal.md) descreve o desenho futuro, ainda não implementado.

## Verificar

```powershell
npm.cmd run typecheck
npm.cmd run format:check
npm.cmd run build
npm.cmd test
```

Testes Playwright usam Edge instalado, servidor isolado na porta 3100 e respostas HTTP de teste, sem escrever no banco real. O build de produção e os testes de navegador são validações distintas. Para formatar: npm.cmd run format.

## Organização

src/lib/api centraliza HTTP, contratos e erros; src/hooks/use-services.ts compartilha o ciclo de consulta; components contém as telas; app define rotas. FeedbackProvider cuida apenas dos avisos. Detalhes em API-INTEGRATION.md.

A identidade Palma é uma referência visual; a API ainda não fornece marca/dados públicos da barbearia. Logo original e reconstruída em public/brand. [BRANDING.md](BRANDING.md) registra a origem.
