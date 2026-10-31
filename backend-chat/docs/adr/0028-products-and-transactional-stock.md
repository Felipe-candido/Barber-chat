# 0028 — Catálogo de produtos e estoque transacional

Data: 2026-10-30. Estado: entrega B implementada na API; testes em PostgreSQL descartável. Sem frontend de produtos ou registro de vendas nesta etapa.

## Caminho da requisição

Bearer → Authenticate → ShopAccess → OwnerOnly nas escritas/auditoria → ProductHandler
→ ProductCommands/ProductQueries → ProductTransactor → ProductGuard + ProductStore
na mesma transação → commit → resposta sem cache.

cmd/api injeta o pool já existente. O middleware verifica acesso inicial; o guard
de identity revalida usuário, membership, perfil e unidade dentro da transação.
Barber consulta apenas produtos ativos; owner também pode consultar inativos.
Entrada, ajuste, edição, mínimo e histórico detalhado exigem owner.

Para barber, o guard trava primeiro o profissional ativo, depois membership/user
e shop. Uma troca de papel/vínculo entre a leitura inicial e os locks é recusada.
Escritas de produto seguem por produto e saldo, preservando uma ordem conhecida.
Consultas são curtas, sem rede externa, e mantêm autorização até a resposta SQL.

## Schema e invariantes

00015 cria products, product_stock, product_stock_movements e product_commands.
Produtos têm preço BRL positivo em BIGINT, versão e timestamps finitos. Saldo é
INTEGER não negativo; nasce em zero. Quantidade inicial positiva gera movimento
opening no mesmo commit do cadastro. Edição do mínimo não altera quantidade.

Movimentos desta entrega são apenas opening/receipt/adjustment. Sale/reversal
continuam somente no domínio: a futura migration de vendas criará as origens com
FKs completas antes de liberar esses tipos no banco, evitando UUIDs sem referência.

As FKs incluem a unidade; o ator precisa ter membership nessa unidade. A aplicação
revalida também atividade e papel: uma FK histórica não substitui autorização.
Uma única abertura por produto é permitida. UPDATE/DELETE de movimentos são
recusados por trigger; correção de quantidade usa novo movimento, com motivo.

O trigger AFTER INSERT calcula e aplica o delta com a linha de saldo bloqueada.
Se o resultado for negativo ou ultrapassar MaxInt32, a inserção inteira falha.
O trigger de saldo impede inicialização diferente de zero e mudança direta da
quantidade fora do caminho dos movimentos. AFTER INSERT evita efeitos quando
uma inserção duplicada é ignorada por ON CONFLICT DO NOTHING.

O saldo materializado permanece igual à soma dos movimentos no caminho normal.
Os testes conferem essa igualdade depois das mutações, falhas e corridas. Papel
restrito recebe INSERT de movimentos e não UPDATE de quantidade; a função do
trigger tem SECURITY DEFINER, referências qualificadas e search_path fixo.
Seu EXECUTE público foi revogado. O teste comprova que o trigger funciona sem
conceder ao chamador UPDATE de saldo.

RLS está habilitado e grants de PUBLIC/anon/authenticated foram revogados. A
role servidor autorizada precisa de política/privilégios apropriados; no teste,
uma role NOLOGIN/BYPASSRLS exercita os grants mínimos. A migration não cria roles
globais nem troca a conexão atual. Proprietário/superusuário continua uma fronteira
administrativa: pode alterar schema/desabilitar triggers; não é credencial de navegador.

## Correção de locks — migration 00016

O teste com duas inserções SQL diretas encontrou uma conversão de locks na 00015:
ambas as FKs adquirem KEY SHARE e FOR UPDATE tenta promovê-las, causando deadlock.
Uma migration posterior substitui o lock do trigger por FOR NO KEY UPDATE. Esse
lock serializa os escritores de quantidade e é compatível com KEY SHARE. A query
do repositório de saldo adota o mesmo modo. Não foi reescrita a migration já testada.

Aplicar o conjunto até a versão 16, nunca parar o rollout na 15. Rollbacks de 15/16
recusam produtos/comandos existentes; não remover registros reais para contorná-los.
Referência: [locks de linha PostgreSQL](https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-ROWS).

## Idempotência e versões

POST de produto/movimento exige Idempotency-Key com 64 caracteres hex minúsculos.
Gerar 32 bytes aleatórios no cliente. Não registrar a chave. Banco recebe digest
SHA-256 e fingerprint HMAC da entrada normalizada, vinculados a shop/ator/operação.

Claim, criação/movimento e conclusão da chave pertencem ao mesmo commit. Um
constraint trigger diferido recusa commit de claim incompleto. Requisições
concorrentes com mesma chave são serializadas pela unicidade e lock do comando;
mesma entrada recupera IDs, entrada diferente gera 409. Um resultado incerto exige
repetir a chave e a entrada originais, sem criar outra movimentação.

Replay revalida autoridade antes da leitura e acontece antes de testar o estoque
para uma operação nova. A resposta contém IDs originais e estado ATUAL do produto,
com replayed=true: uma edição posterior não é desfeita para repetir um cadastro.
Chaves não expiram nem são apagadas automaticamente nesta primeira versão.

PUT de detalhes exige todos os campos editáveis e expected_version. PUT do mínimo
exige expected_stock_version. Conflito devolve 409 e pede consulta/revisão; no-op
preserva a versão. PUT foi adotado no lugar do PATCH proposto para deixar explícita
a substituição completa dos detalhes, sem interpretar campos omitidos como remoção.

## Leituras e HTTP

Produtos: busca literal sem diferenciar maiúsculas por nome, include_inactive
owner-only, paginação por (created_at,id) descendente; 25 por padrão, máximo 100.
Histórico: página por (created_at,id), limitado ao produto/unidade e owner.
Cursor não autoriza: sua posição é ligada ao ator/unidade/endpoint/filtros, e o
SQL aplica a unidade antes da posição. Respostas privadas usam no-store.

Corpos JSON até 8 KiB, campos desconhecidos e conteúdo adicional recusados. Não
aceita shop_id, actor_id, saldo final, papel ou tipo de venda no corpo. Razões,
chaves, fingerprints, payloads e erros SQL brutos não entram nos logs. OPTIONS
fica fora da autenticação e permite Idempotency-Key na origem configurada.

Contrato completo: [products-openapi.yaml](../products-openapi.yaml).

## Validação e próxima etapa

Testes com PostgreSQL real: criação/abertura, replay normalizado/concorrente,
retirada da última unidade pela API e por SQL direto, falha depois do trigger,
limites inteiros, FKs de tenant, histórico imutável, claims incompletos, role
restrita, revogação de acesso, versões e contrato HTTP/CORS/paginação.

Próxima entrega é C: registrar venda com itens/snapshots e baixa de todos os
produtos em uma transação. SQL direto de múltiplos produtos também deverá seguir
ordem de locks; esta entrega exercita operações de estoque de um produto por comando.
Frontend e relatórios continuam nas etapas posteriores do plano.
