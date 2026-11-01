# 0035 — Casos de uso de resumo e histórico autorizado

Data: 2026-10-31. Estado: casos de uso implementados e testados, incluindo
PostgreSQL real. Handlers, transporte do cursor e bootstrap da API ficam para
a próxima parte. Nenhum endpoint novo foi registrado nesta etapa.

## Coordenação na aplicação

ActivityQueries possui duas operações: Overview e History. Recebe somente as
portas ActivityReadTransactor e PhoneNormalizer. Não importa HTTP, pgx, identity,
booking, sales ou customers. Os IDs de ator/unidade devem vir do contexto
autenticado no futuro adaptador HTTP, nunca do corpo/parâmetros livres.

1. Validar identidade de entrada, origem e filtros. Origem vazia assume all.
   Normalizar telefone localmente, antes da transação de banco.
2. Abrir a leitura compartilhada e verificar o acesso atual pela porta Access.
3. Aplicar ProfessionalFor: owner pode selecionar um profissional ou consultar
   a unidade; barber fica restrito ao profissional ativo de seu vínculo atual.
4. Carregar a unidade ativa pela porta Shops e validar ID/fuso armazenados.
5. Construir Period com as datas solicitadas no fuso da unidade, nunca do
   navegador. Datas inválidas ou acima de 366 dias retornam ErrInvalidQuery;
   ID/fuso incoerentes no banco retornam ErrInconsistentData.
6. Executar o reader e verificar se a resposta respeita escopo, metadados,
   tipos, valores e contagens. Falha da leitura ou do commit retorna resultado
   zero: não divulgar uma página parcial ou cursor em uma resposta de erro.

O frontend não foi alterado nesta parte. Nenhuma migration nova foi criada nem
houve alteração no Supabase; os testes usam o banco descartável de reporting.

## Filtros de contato

Nome: texto literal sem controles, UTF-8 válido, até 201 caracteres (100 de nome,
espaço e 100 de sobrenome). Remover espaços externos. O reader continua dono
do escape de LIKE e da busca sem distinguir maiúsculas/minúsculas no PostgreSQL.
Não tentar reproduzir a collation do banco em Go para validar os nomes retornados.

Telefone: entrada de até 40 bytes, normalizada pela capacidade existente de
customers. Extração de função privada compartilha o parsing com o contato do
agendamento, preservando sua exigência de país. Na busca de relatórios, país é
necessário para formato nacional e pode ser omitido para número com +código.
Rejeitar extensões, números inválidos e país sem telefone. O adapter retorna
ErrInvalidQuery para erro de entrada, sem devolver o texto sensível no erro.
Não criar nem resolver um customer durante a busca. A consulta recebe E.164.

## Cursor e paginação

Página padrão 25, máximo 100. Ler N+1, validar inclusive o registro adicional,
retornar N e gerar Next a partir do último registro efetivamente retornado.
Não usar o registro adicional como posição, pois isso pularia uma linha.

Fingerprint SHA-256 de estrutura canônica explícita, protocolo 1: ator, unidade,
profissional efetivo, alcance de owner, limites UTC, fuso IANA, origem, nome/telefone
normalizados, serviço, produto e situações de atendimento/venda. Mudança de owner
para barber invalida o cursor mesmo se o profissional selecionado for o mesmo.
Alterar a forma de escrever o mesmo telefone não invalida sua versão E.164.

Tamanho da página e ObservedAt ficam fora do fingerprint: pode mudar o tamanho
dentro do limite, e toda página é uma nova leitura autorizada. Não existe snapshot
congelado entre requisições. Revalidar membership mesmo em replay de cursor.

Verificar também posição dentro do período, origem permitida, IDs e ausência/
presença de item conforme a origem. Exigir ordem estritamente decrescente por
instante/origem/registro/item, seguindo COLLATE C e UUIDs do PostgreSQL. Rejeitar
duplicatas, linhas fora do escopo e respostas acima do limite solicitado.

O hash vincula filtros; não é assinatura nem credencial. Uma posição modificada
não pode ampliar o escopo da consulta autorizada. A codificação/decodificação
opaca, limites do parâmetro e mapeamento HTTP serão implementados no adaptador
de transporte da próxima parte, sem retornar contato dentro do cursor.

## Validação das projeções e testes

ActivityHistoryRow.ValidateFor centraliza as invariantes usadas pelo reader e
pelo caso de uso. Não manter duas cópias de regras de quantidade, preço, recursos,
profissional e filtros. Isso não transforma uma projeção em entidade de booking
nem concede acesso. Overview valida metadados, medidas por moeda, grupos no
escopo, dias do período e coerência entre concluídos e seus contadores.

Testes de aplicação cobrem normalização, acesso antes da leitura, restrição de
barber, falhas de dependência/commit, rejeição de resultados inconsistentes,
revalidação entre páginas e vínculo do cursor a cada filtro/autoridade. Testes
do adapter de telefone cobrem formatos nacional/internacional e entradas inválidas.

Onze cenários PostgreSQL passaram, incluindo o novo fluxo pelos casos de uso:
resumo, telefone normalizado, escopo pessoal, paginação, tentativa de reutilizar
cursor em outra unidade autorizada, revogação do vínculo e anulação posterior.
Também passaram go vet, go test ./..., builds da API/worker e Compose.

Próxima parte: handlers de resumo/histórico, transporte do cursor, parâmetros
HTTP estritos, composição única no bootstrap, tratamento de erros e OpenAPI.
O painel gerencial permanece na entrega H, preservando o redesign do usuário.
