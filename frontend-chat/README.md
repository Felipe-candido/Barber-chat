# Palma Barbearia — frontend

Frontend independente em Next.js (App Router), React, TypeScript e Tailwind CSS. Interface em português, com a identidade azul-marinho da Palma.

## Rodar no Windows

Requer Node.js 22 ou superior e npm. No PowerShell, a partir da raiz do repositório:

```powershell
cd frontend-chat
npm.cmd install
npm.cmd run dev
```

Abra [Agenda](http://127.0.0.1:3000/admin), [Serviços](http://127.0.0.1:3000/admin/servicos) ou [Chat](http://127.0.0.1:3000/chat).
Não é necessário carregar .env, iniciar Go, Docker ou Supabase. Se a porta 3000 estiver ocupada, use `npm.cmd run dev -- --port 3001`.
O uso de npm.cmd evita depender da política de execução de scripts do PowerShell.

## O que funciona

- Agenda diária, semanal (segunda a sábado) e mensal; navegação entre períodos, busca e filtro por profissional.
- Criação e edição de agendamentos manuais; confirmação de pendentes, conclusão de atendimentos até hoje e cancelamento com confirmação.
- Catálogo com busca, categorias, inclusão, edição, exclusão e ativação/desativação.
- Chat guiado: serviço → profissional → data/horário → contato → revisão → confirmação.
- Dados compartilhados entre as telas e persistidos no navegador. Ao salvar ou abrir a agenda pelo chat, o calendário navega até a data reservada.
- Layout responsivo, menu móvel e diálogos nativos com foco contido e fechamento por Escape.

## Demonstração e dados

Os dados são fictícios, gerados para a semana atual. Os profissionais são exemplos. O expediente de demonstração é de segunda a sábado, 9h–19h, no horário local do navegador. Os preços são armazenados em centavos; cada agendamento preserva nome, preço e duração do serviço mesmo se o catálogo mudar.

Não existe integração com a API, autenticação, envio de mensagens, pagamento ou reserva real. O chat é um fluxo de escolhas, sem IA ou WhatsApp. Os formulários só salvam contato ao confirmar.

A chave `palma-barbearia-demo-v1` no localStorage guarda serviços e agendamentos. Use dados fictícios. Para restaurar a demonstração, remova apenas essa chave no DevTools e recarregue. Abas da mesma origem sincronizam alterações pelo evento storage. Se o armazenamento estiver indisponível, a interface informa que os dados ficarão na sessão.

Conflitos são verificados localmente por profissional e intervalo. Isso facilita testar a interface, mas não garante concorrência entre dispositivos/abas. A integração futura deverá delegar identidade, timezone, disponibilidade e persistência transacional ao backend.

## Arquitetura

- `src/app`: rotas e layouts do Next. A raiz redireciona para a agenda.
- `src/components/admin-shell.tsx`: navegação e estrutura administrativa.
- `agenda-page.tsx` e `appointment-modal.tsx`: visualizações e interação com agendamentos.
- `services-page.tsx`: catálogo e formulários de serviços.
- `booking-chat.tsx`: fluxo guiado de agendamento.
- `demo-provider.tsx`: estado compartilhado, armazenamento, validações de escrita e notificações.
- `src/lib/model.ts`: tipos, dados iniciais e operações de data/disponibilidade.
- `src/components/ui.tsx`: marca, diálogo e estados de carregamento/vazio.
- `src/app/globals.css`: tema Tailwind, estilos e adaptações de tela.
- `public/brand`: logo original e reconstruída. Detalhes em [BRANDING.md](BRANDING.md).
- `tests/frontend.spec.ts`: testes reais no navegador com Playwright.

Não há cliente HTTP nem credenciais do Supabase no frontend. Ao integrar, substitua a fonte de dados do provider e adapte os contratos reais; não use validações do navegador como autorização.

## Validar

```powershell
npm.cmd run typecheck
npm.cmd run format:check
npm.cmd run build
npm.cmd test
```

Os testes usam Microsoft Edge instalado no Windows (`channel: msedge`). O Playwright inicia a versão de produção na porta 3000 quando não há servidor ativo. Portanto, gere o build antes. A configuração permite reutilizar um servidor local.

Os testes cobrem ciclo de serviços e persistência, agendamento manual/edição/cancelamento, horários ocupados, chat e confirmação, filtros do calendário, navegação móvel e erros de runtime. Capturas ficam em test-results (ignorado pelo Git). Isso não valida integração com PostgreSQL, isolamento de tenant ou concorrência real.

Para execução de produção local: `npm.cmd run build` seguido de `npm.cmd run start`.
Para formatar os arquivos: `npm.cmd run format`.

## Dependências e fontes

Versões exatas registradas em package.json e package-lock.json:
[Next.js](https://nextjs.org/docs/app/getting-started/installation), [Tailwind CSS](https://tailwindcss.com/docs/installation/framework-guides/nextjs), [Lucide](https://lucide.dev/guide/react), [Fontsource DM Sans](https://fontsource.org/fonts/dm-sans/use), [Playwright](https://playwright.dev/docs/intro) e [Prettier](https://prettier.io/docs/install).
A fonte é servida localmente pelo pacote Fontsource, sem buscar Google Fonts em runtime.

## Acesso administrativo (prévia)

A rota `/login` apresenta e-mail/senha, alternância de visibilidade da senha, explicação de acesso por convite e recuperação demonstrativa. O link administrativo do chat abre essa página; o avatar do painel também permite acessá-la.

O formulário apenas abre a demonstração: não chama a API, não valida credenciais, não cria sessão, não protege `/admin` e não salva e-mail/senha. Não use credenciais reais. A recuperação não envia mensagens. Os dados de catálogo/agendamento de demonstração continuam separados desses formulários.

A proposta de arquitetura, entidades, permissões, pacotes e próximos passos está em [Autenticação por barbearia](../backend-chat/docs/authentication-proposal.md).
