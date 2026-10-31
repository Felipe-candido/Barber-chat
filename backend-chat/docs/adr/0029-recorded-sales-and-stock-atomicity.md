# 0029 — Venda com itens e baixa atômica de estoque

Data: 2026-10-30. Estado: entrega C implementada na API, validada em PostgreSQL descartável. Consulta/listagem/anulação ficam na entrega D; frontend continua posterior.

## Coordenação e fronteiras

POST /api/v1/admin/shops/{slug}/sales passa por Authenticate/ShopAccess, sem OwnerOnly:
barber também vende, limitado ao profissional atual. Handler aceita somente IDs,
quantidades e preço/versão revisados. Actor e shop vêm do contexto; nomes, moeda,
totais, status, instantes e snapshots vêm do servidor. JSON até 16 KiB, 1–50 produtos
distintos, centavos inteiros exatos e idempotência obrigatória.

RegisterSale recebe SalesTransactor. O bootstrap compartilha um pgx.Tx entre os
adaptadores de identity, catalog, booking, billing e sales. Domain/application
não importam outros módulos. As portas de escrita desta entrega são estreitas:
SaleRecorder e SaleStockRecorder; anulação/reposição não têm métodos falsos.

## Fluxo do caso de uso

1. Copiar/ordenar os itens por UUID e validar limites. Fingerprint HMAC da entrada
   normalizada, chave de 32 bytes em hex minúsculo e digest SHA-256, sem armazenar
   chave ou corpo bruto. Escopo, profissional/atendimento solicitados e condições
   revisadas compõem a identidade da entrada. O formato canônico privado inclui
   protocolo 1 e não serializa o DTO inteiro: adicionar campo ao DTO não altera
   silenciosamente as chaves persistidas. Ordem dos produtos não muda a chave.
2. Ler acesso preliminar e reivindicar a chave de shop/ator/operação na transação.
   Claim pertence ao mesmo commit; o banco recusa claim incompleto.
3. Replay: revalidar acesso atual e devolver a venda salva. Não consultar catálogo,
   disponibilidade, situação do atendimento ou cobertura como condição de uma
   venda nova. Owner pode recuperar recibo atribuído a profissional agora inativo;
   barber precisa manter seu próprio acesso ativo e correspondente à venda.
4. Venda nova: inferir profissional do atendimento opcional e impor o escopo do
   barber. Travar profissional antes de membership/user/shop, seguindo booking.
   Revalidar acesso após locks. Owner pode atribuir ou deixar venda avulsa sem barbeiro.
5. Travar atendimento e aceitar somente scheduled/confirmed/completed do mesmo
   tenant/profissional. Não concluir nem cancelar automaticamente o atendimento.
6. Verificar habilitação: unidades manuais mantêm compatibilidade; self_service
   requer cobertura. Usar o lock da conta compartilhado com reconciliação, sem
   chamar Stripe. Publicação não é necessária para vender administrativamente.
7. Travar produtos em ordem estável com FOR SHARE e saldos na mesma ordem com
   FOR NO KEY UPDATE. Comparar versão/preço revisados, atividade e quantidade.
8. Construir Sale com snapshots, gravar cabeçalho e itens, inserir saídas ligadas
   aos itens. Lotes via jsonb_to_recordset evitam um round-trip por item, preservando
   casts inteiros, limites e triggers. IDs/totais não são JSON arbitrário do navegador.
9. Completar claim e carregar o recibo persistido; timestamps têm a mesma precisão
   na primeira resposta e replay. Revalidar cobertura com o relógio atual ao fim
   do trabalho. Se o prazo expirou durante espera/escrita, tudo sofre rollback.
10. Commit confirma constraints diferidas. Só então o handler responde 201; replay
    responde 200 e replayed=true. Resultado de commit incerto exige a mesma chave.

O check final é uma decisão no fim da transação; não garante que a assinatura
continue vigente depois da resposta. Cobertura pode terminar normalmente em seguida.

## Proteções no banco — migration 00017

Sales guarda o ator, atribuição, vínculo, valor, moeda e data. Sale_items guarda
snapshots e quantidade; line_total_cents é calculado pelo PostgreSQL. Moeda BRL,
valores positivos e até 2^53−1; FKs incluem shop_id. Uma FK adicional liga o
atendimento ao mesmo profissional da venda.

Movimentos ganham origem sale_item_id e quantidade positiva derivada de sua saída.
FK (shop,produto,item,quantidade) obriga a saída exata do item. Índice único permite
uma baixa por item. Um trigger valida também que o ator é quem registrou a venda.
Movimentos manuais continuam sem origem de venda; a API manual não aceita kind=sale.

Constraints diferidas conferem no commit: 1–50 itens, total igual à soma e uma saída
para cada item. Cabeçalho sem itens, item sem baixa, total divergente e comando
incompleto não podem ficar persistidos. Uma falha em qualquer etapa desfaz também
os saldos atualizados pelo trigger. Trigger de saldo mantém o protocolo corrigido
da migration 00016, compatível com os locks das FKs.

Cabeçalhos e itens registrados não aceitam UPDATE/DELETE nesta entrega. RLS e
revogação de grants públicos/browser acompanham as tabelas novas. Triggers de
consistência que precisam ler tabelas usam SECURITY DEFINER, referências qualificadas,
search_path fixo e EXECUTE público revogado. Papel administrativo do banco permanece
uma fronteira de confiança, como nas entregas anteriores.

## Observação e limites

SaleStockItem é projeção interna de item validado, não payload público. Catalog
grava os movimentos; sales não atualiza quantity_on_hand diretamente. O histórico
de estoque preserva sale_item_id para rastrear a origem; o relatório futuro poderá
ler as tabelas sem duplicar agendamentos através dos joins com itens.

Esta rota registra uma venda e seu valor; não processa pagamento do comprador,
não presume recebimento e não usa Stripe Checkout para cobrar produtos. Não cria
conta de cliente. Logs não recebem chaves, payloads, erros SQL brutos ou contatos.

## Validação e sequência

Testes reais exercitam última unidade, chave simultânea, itens em ordem inversa,
rollback após primeira saída, preço revisado, snapshot após alteração do catálogo,
escopo de barber/owner, revogação, produtos de outro tenant, vínculo com atendimento,
expiração de cobertura após as escritas, HTTP/CORS e regressão de produtos.
Testes SQL diretos verificam cabeçalho incompleto, itens sem baixa, total divergente,
origem duplicada e histórico imutável. Migration testada up/down/up em banco vazio.

Próximo passo: D, consulta e anulação com motivo e reposição explícita, em nova
migration que evolua cuidadosamente os guards de histórico e consistência; não
desabilitar os triggers atuais para editar uma venda manualmente.

Contrato: [sales-openapi.yaml](../sales-openapi.yaml). Sem aplicação no Supabase nesta entrega.
