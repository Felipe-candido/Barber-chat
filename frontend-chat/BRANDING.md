# Identidade visual Palma

Paleta baseada na imagem fornecida: azul-marinho #10243F, azul #315B92, branco e fundos claros #F6F8FB. Tipografia DM Sans nas interfaces e serifada nos destaques do chat.

- `public/brand/palma-original.png`: imagem original fornecida pelo usuário, preservada.
- `public/brand/palma-logo.png`: reconstrução em PNG com definição maior, utilizada nas telas.
- `src/app/icon.png`: mesma marca como ícone do aplicativo.

A imagem foi recriada com a ferramenta integrada de geração/edição de imagens do Codex, referenciando a logo original. É uma reconstrução raster, não um arquivo vetorial; pode haver pequenas diferenças tipográficas. A imagem original permanece disponível para comparação.

## Prompt utilizado

```text
Use case: precise-object-edit. Asset type: production-ready high-resolution barbershop logo for website.
Input image 1 is the edit target, the existing PALMA BARBEARIA logo. Reconstruct this exact logo faithfully at 2048x2048 with very sharp clean typography and smooth concentric circular outlines. Keep the same proportions: navy blue circular center with a restrained brighter blue at top left, narrow white inner ring, black outer ring and pale silver-gray outside border. Center the exact large white serif word "PALMA" with the same distinctive thin barber-tool detail integrated near the first A, and the exact small widely spaced uppercase "BARBEARIA" beneath it. Preserve this brand identity, letter spacing and positions. Text must be spelled exactly P A L M A and B A R B E A R I A. Make the ring perfectly circular and all letters readable without pixelation or blur. Flat graphic logo, no photograph, no mockup, no bevel or 3D, no extra words, no new symbols. Keep a very small clean margin around the complete round badge. Outside the round badge use a transparent background, preserving true alpha if possible. The blue circular center stays opaque. Deliver only the single restored logo.
```
