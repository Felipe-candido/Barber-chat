# Tarefas de backend: comprador na venda e assinatura no cadastro de produto

Pedidos do dono do produto (15/11/2026). Escritos para o Codex: exigem migration,
sqlc e testes Go.

## 1. Comprador opcional na venda

Hoje `sales` não guarda quem comprou. O dono quer registrar o cliente ao criar
uma venda, de forma opcional.

- Migration: `sales.customer_id UUID NULL`, com FK composta
  `(shop_id, customer_id) -> customers(shop_id, id)` e índice por
  `(shop_id, customer_id, created_at DESC)`.
- API `POST /sales`: campo opcional `customer_id` (contato existente da mesma
  barbearia) **ou** `customer {first_name, last_name, phone}` para criar/reusar
  pelo telefone, com a mesma regra do agendamento
  (`ON CONFLICT (shop_id, phone_e164) DO NOTHING`). Nunca aceitar os dois.
- A chave de idempotência da venda deve cobrir o comprador.
- Respostas de venda (lista, detalhe, recibo): `customer {id, name}` ou `null`.
  Barbeiro só vê o comprador das vendas que ele próprio pode ver.
- Relatórios não mudam nesta etapa.
- Frontend (depois da API): no formulário de venda, um campo "Cliente
  (opcional)" com busca nos contatos por nome/telefone e opção de cadastrar
  novo; mostrar o cliente no detalhe e no recibo; filtro por cliente na lista.
- Testes: venda sem cliente continua igual; cliente de outra barbearia é
  recusado; retry idempotente não duplica contato; cancelamento preserva o
  vínculo.

## 2. Exigir assinatura para cadastrar produto

O painel passou a conferir a assinatura antes de abrir "Adicionar produto" e a
oferecer o pagamento quando falta. É só uma conferência de tela: a API ainda
aceita `POST /products` sem cobertura.

- Em `POST /products`, usar o mesmo guard das vendas
  (`billing/infra/postgres/sales.go`, `CanOperateShop`) e responder
  `402 shop_subscription_required`.
- Decidir e documentar: editar produto e movimentar estoque continuam liberados
  sem assinatura? (O painel hoje só bloqueia a criação.)
- O frontend já trata `shop_subscription_required` em `CommandFeedback`.
