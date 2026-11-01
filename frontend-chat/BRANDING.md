# Identidade visual Barber-chat

Visual genérico de "barbearia clássica", sem logo de nenhuma barbearia específica. Os tokens ficam em `src/styles/tokens.css`; as telas usam apenas os aliases semânticos (`--color-*`, `--space-*`, `--radius-*`).

## Cores

| Papel          | Claro                    | Escuro            |
| -------------- | ------------------------ | ----------------- |
| Fundo          | `#F4F4F2` (papel frio)   | `#121415`         |
| Superfície     | `#FFFFFF`                | `#1A1D1F`         |
| Texto          | `#1C1F22` (grafite)      | `#ECEAE4`         |
| Ação principal | `#1C1F22` sólido         | `#C9A463` (latão) |
| Acento         | `#B08A4A` (latão)        | `#C9A463`         |
| Links          | `#7A5C2A` (latão escuro) | `#D8BD86`         |

O latão aparece com moderação: o traço curto acima dos títulos de página, o dia atual, o item ativo da navegação e a borda dos agendamentos. O modo escuro segue o tema do sistema (`prefers-color-scheme`).

## Tipografia

- Títulos de página, de janelas e preços: **Fraunces** (serifada, `@fontsource-variable/fraunces`).
- Interface e textos: **DM Sans**.

## Marca

A marca do produto é o ícone de tesoura em um quadrado grafite com o nome "Barber-chat". As barbearias aparecem pelo nome cadastrado, sem logo. Os arquivos em `public/brand/` são do protótipo anterior e não são usados nas telas.
