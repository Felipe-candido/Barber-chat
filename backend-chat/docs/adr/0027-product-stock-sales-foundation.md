# 0027 — Fundação de produtos, estoque e vendas

Data: 2026-10-30. Estado: entrega A implementada; banco, casos de uso transacionais e HTTP ainda planejados.

## Fronteiras

Catalog continua dono das ofertas e passa a definir Product, StockBalance e
StockMovement. Sales é um novo módulo de negócio, com Sale/Item e portas próprias.
Reporting amplia suas projeções para serviços/produtos. Permanece um monólito,
um go.mod e o pool existente. Domain/application não importam outros módulos;
os adaptadores futuros preencherão as portas na transação compartilhada.

Não foram criados adapters/pastas de infra vazios. Não há novas rotas, migrations
ou acesso ao Supabase nesta entrega. READMEs e doc.go permanecem preservados.

## Regras executáveis

- Product: nome UTF-8 de 1–100 caracteres, descrição até 1000, preço positivo em
  centavos inteiros, BRL, versão e datas UTC. Update exige versão esperada;
  alteração sem diferença não incrementa versão. Desativação é mudança de estado,
  sem apagar histórico. O saldo não é campo editável de Product.
- StockBalance: começa em zero, com quantidade/mínimo inteiros de 0 a MaxInt32.
  Apply calcula o resultado de um movimento do mesmo produto/tenant e recusa
  saldo negativo ou acima do limite. SetMinimum muda só o alerta, com versão.
- StockMovement: abertura, entrada, ajuste, venda ou reversão, com sinal/referências
  compatíveis e motivo de 1–500 caracteres. Uma reversão é construída a partir
  do movimento original de venda, copiando tenant, produto, item e quantidade.
- Sale: 1–50 produtos distintos; IDs de itens também distintos, quantidade positiva
  até MaxInt32 e snapshot BRL do próprio tenant. Soma/multiplicação são verificadas
  antes do cálculo. Total e nomes/preços nascem dos snapshots do servidor, não
  do corpo HTTP. O draft é copiado para não compartilhar a coleção com o chamador.
- Anulação: motivo, ator, versão esperada, data e decisão explícita de repor estoque.
  Repetição com mesmas instruções retorna a anulação armazenada, sem nova versão.
  Instrução de reposição diferente é conflito. Domínio não grava movimentos nem
  autoriza o proprietário: esses passos pertencem ao caso de uso transacional.

Centavos de produto/venda e soma do relatório são limitados a 2^53−1, preservando
inteiros exatos ao atravessar JSON/JavaScript. Não usar float para valores. Esse
é um limite técnico de representação, não uma regra comercial de preço sugerido.

## Autoridade e repetição

SalesAuthority recebe projeção confiável do acesso atual. ProfessionalFor obriga
barber a usar seu profissional; owner pode atribuir a outro ou deixar sem atribuição.
O mesmo escopo deve conter ator/tenant da projeção. RequireOwner restringe futuras
ações exclusivas. Não confiar em papel/profissional vindos de JSON ou JWT metadata.

DTOs distinguem preço esperado, que serve para revisão, de ProductForSale, lido
pelo servidor. As portas exigem locks ordenados, origem única de movimentos,
revalidação de acesso e conclusão idempotente no mesmo commit. Chave/fingerprint
são digests ligados a shop/actor/operação. O replay concluído recupera o ID salvo;
estado atual pode ser consultado antes de responder, sem repetir a baixa.

A proposta de exigir cobertura comercial para novas vendas self_service foi
representada pela porta SaleEligibility. Sua integração ainda não existe e não
modifica as permissões/assinaturas atuais. Gestão e replay não usam esse guard.

## Leituras gerenciais

ActivitySource distingue all/services/products. All é filtro e nunca identidade
de uma linha. Em all, filtro de serviço restringe o ramo de atendimentos e filtro
de produto restringe itens vendidos. Selecionar somente um ramo recusa filtro
incompatível. Cada bucket de moeda separa valor de serviços, valor de produtos,
quantidade de atendimentos, comprovantes de venda distintos e unidades vendidas.

CombinedValueCents valida o bucket e calcula a soma sem mudar esses contadores.
R$100 de serviços + R$90 de produtos = R$190 registrados; isso não comprova caixa
recebido ou lucro. ActivityAudience limita o relatório pessoal ao profissional
do acesso atual; projeção incoerente não pode virar consulta da unidade inteira.

As portas de leitura pedem snapshot coerente e agregação das origens antes de
combiná-las. Histórico propõe paginação por (instante, origem, registro, item),
vinculada ao tenant, filtros e autoridade efetiva. Não retorna código de reserva.

## Limites e próxima entrega

Os structs de domínio seguem o estilo existente de valores exportados para
mapeamento. Criações/comandos devem passar pelos construtores/transições e os
repositórios deverão manter os snapshots imutáveis na persistência. Esses tipos
não devem ser decodificados diretamente do HTTP.

Apply é uma regra pura: não controla concorrência, não deduplica a origem e não
comprova reposição única. A entrega B implementará AFTER INSERT, FKs, CHECKs,
unicidade, privilégio mínimo e caminhos de escrita de saldo/movimentos. A entrega
C juntará venda, itens, movimentos e idempotência em uma transação. Só os testes
PostgreSQL dessas entregas poderão comprovar concorrência e rollback.

Foram adicionados testes puros para validação, versões, snapshots, saldo/limites,
referências de reversão, soma segura, escopo e exemplo dos R$190. Os testes de
fronteiras de imports existentes também inspecionam automaticamente sales.

Roteiro completo: [plano de produtos/vendas](../product-sales-implementation-plan.md).
