# Barber-chat — frontend

Next.js, React, TypeScript e Tailwind. Login Supabase, identidade local no Go, seleção de unidades, catálogo, cadastro de barbeiros e agendamento público estão integrados. Usuários, barbearias e vínculos continuam cadastrados manualmente. O chat permite reservar, receber comprovante/código, consultar e cancelar sem conta de cliente; WhatsApp permanece adiado.

## Rodar

No PowerShell:

```powershell
cd frontend-chat
if (!(Test-Path .env.local)) { Copy-Item .env.example .env.local }
npm.cmd install
npm.cmd run dev
```

Configure NEXT_PUBLIC_API_BASE_URL com a URL HTTP da API, NEXT_PUBLIC_SUPABASE_URL com a URL do projeto Supabase e NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY com a chave pública/publishable. No backend, configure SUPABASE_URL para o mesmo projeto e FRONTEND_ORIGIN=http://127.0.0.1:3000. Não há slug fixo de ambiente.

Abra [Login](http://127.0.0.1:3000/login). O formulário envia e-mail/senha diretamente ao Supabase, que persiste e renova a sessão no navegador. Depois, o frontend chama /api/v1/admin/me e /api/v1/admin/shops com o access token. O usuário escolhe uma unidade em /admin e entra em /admin/{slug}/servicos. Leituras e escritas administrativas usam Bearer; nunca enviam senha à API Go. O catálogo público fica em /b/{slug}; /chat apenas orienta a obter o link da barbearia.

Variáveis `NEXT_PUBLIC_*` são incorporadas ao JavaScript do navegador. A URL e a chave publishable do Supabase são públicas por design; nunca coloque `service_role`, chave `sb_secret_*`, `DATABASE_URL` ou senha de banco no frontend. Reinicie `npm.cmd run dev` depois de alterar `.env.local`.

O Go valida JWT ES256 do projeto configurado, usuário local e vínculo ativo em cada operação por unidade. /me identifica o usuário, /shops lista unidades elegíveis e serviços administrativos revalidam o vínculo; a seleção visual não é prova de permissão. Uma conta no Auth sem perfil local ativo recebe 403; um perfil sem unidades recebe uma lista vazia. O catálogo público continua sem token.

## Integração

- Lista serviços ativos e cria serviços com os campos reais do handler Go.
- Exibe carregamento, lista vazia, erro, sucesso e envio pendente.
- Faz buscas locais sobre a lista recebida.
- Consome o slug da unidade escolhida no painel e o slug da URL no catálogo público.
- Não usa serviços/agendamentos simulados, nem fallback em localStorage.
- Cadastra barbeiro, serviços habilitados e expediente inicial pelo painel Barbeiros.
- Agenda pelo chat público, mostra comprovante/código e oferece consulta/cancelamento.
- Mantém a chave da confirmação em memória para repetição segura após falha de rede.
- Não permite editar/excluir/ativar serviços ou editar o calendário. A agenda visual administrativa ainda não consulta reservas.
- Autentica e-mail/senha com `@supabase/supabase-js`, restaura/renova a sessão e usa Bearer somente em chamadas administrativas.
- Oferece troca de unidade e logout deste navegador, limpando estado antigo e cancelando consultas pendentes.

[API-INTEGRATION.md](API-INTEGRATION.md) contém contratos, configuração e limites. [Autenticação](../backend-chat/docs/authentication-proposal.md) detalha provisionamento manual e o fluxo implementado.

Para testar: aplicar migrations até 00005 no backend, iniciar a API, entrar em /login, escolher unidade, cadastrar um serviço e depois um barbeiro vinculado a ele. No painel, abrir o chat público, escolher profissional/data/horário, revisar e confirmar. Guardar o código e testar “Já tenho um código” em outra aba. Cancelamento só antes do início. Em falha ambígua de confirmação, manter a aba aberta e repetir com a mesma tentativa; não criar outra reserva às cegas.

## Verificar

```powershell
npm.cmd run typecheck
npm.cmd run format:check
npm.cmd run build
npm.cmd test
```

Testes Playwright usam Edge instalado, servidor isolado na porta 3100 e respostas HTTP de teste, sem escrever no banco real. O build de produção e os testes de navegador são validações distintas. Para formatar: npm.cmd run format.

## Organização

src/lib/api centraliza HTTP, contratos e erros; src/hooks/use-services.ts compartilha o ciclo de consulta. AdminAccessProvider guarda os resultados de /me e /shops em memória, AdminAccessGuard aguarda validação e ShopPanel fornece a unidade selecionada. app define as rotas; FeedbackProvider cuida dos avisos. O token permanece gerenciado pelo SDK, sem cópia no contexto React. Detalhes em API-INTEGRATION.md.

A identidade Palma é uma referência visual; a API ainda não fornece marca/dados públicos da barbearia. Logo original e reconstruída em public/brand. [BRANDING.md](BRANDING.md) registra a origem.
