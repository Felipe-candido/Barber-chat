# 0036 — API administrativa de resumo e histórico

Data: 2026-10-31. Estado: entrega G concluída na API; painel gerencial fica na H.
Complementa os ADRs 0033–0035. Sem nova migration nesta etapa.

## Rotas e composição

- GET /api/v1/admin/shops/{slug}/reports/activity — resumo e agrupamentos.
- GET /api/v1/admin/shops/{slug}/reports/activity/history — histórico paginado.

server.go registra GET e preflight OPTIONS, com os middlewares de autenticação e
acesso à unidade. Não aplicar OwnerOnly: barber consulta o próprio profissional,
conforme a autoridade atual revalidada pelo caso de uso dentro da transação.
Não usar gate de assinatura/publicação para consultar histórico administrativo.

cmd/api compõe uma instância de ActivityQueries e seu handler, usando o pool
compartilhado. O transactor injeta o mesmo Queries transacional no guard de
identity, no reader de shops e nas projeções de reporting. O normalizador local
de customers é reutilizado. Não criar outro verifier, pool ou serviço externo.

## Transporte e privacidade

handler.go extrai ator/unidade do requestctx verificado. O cliente fornece apenas
datas/filtros e posição da página; actor_id, shop_id, role e quaisquer parâmetros
desconhecidos são erro. Limitar RawQuery a 4096 bytes e rejeitar parâmetros
repetidos, vazios e escapes inválidos. from/through são obrigatórios. Situações,
limit e cursor são exclusivos do histórico; o resumo rejeita esses parâmetros.
IDs de filtro devem ser UUIDs não nulos. O caso de uso continua dono da validade
dos valores e da compatibilidade entre origem, recursos e situações.

response.go mapeia DTOs próprios em snake_case, sem serializar tipos de domínio
diretamente. period contém datas locais inclusivas, limites UTC [from_at,until_at)
e fuso IANA; observed_at identifica o início da leitura. Coleções vazias são [].
Valores/contagens são inteiros seguros para JSON, com moedas separadas. O total
combined_value_cents exclui previsão e resultados cancelados/ausências; não
representa dinheiro recebido. Quantidades de atendimentos/vendas/unidades são
medidas distintas. completed_automatically é subconjunto de completed.

No histórico, IDs/situações não aplicáveis são null. Contato é privado e atual;
venda avulsa anônima tem nome/telefone vazios. value_cents mantém o valor original;
recorded_value_cents explicita sua contribuição, inclusive zero após anulação.
Não expor códigos de consulta pública, hash, ator/motivo da revisão ou tokens.
Respostas JSON administrativas, inclusive erros, usam Cache-Control no-store.

400 invalid_report_query: revisar datas/filtros/limite/cursor. 401 exige sessão
Bearer válida; 403 nega acesso atual ou seleção de outro profissional por barber.
404 shop_not_found representa unidade não encontrada pelo caso de uso. 500
report_unavailable indica projeção incoerente; 503 temporarily_unavailable indica
falha de dependência ou prazo. Não devolver resultados parciais. Logs registram
mensagem genérica e request_id; não registrar erro bruto, URL/query, contatos,
cursor ou Bearer. O middleware de logs já usa o padrão da rota, sem query string.

## Cursor opaco

cursor.go transporta a posição e o fingerprint da aplicação em envelope JSON
versão 1, codificado em base64url sem padding. Não inclui nome nem telefone.
Aceitar apenas a representação canônica emitida pelo servidor, até 1024
caracteres: rejeitar versão desconhecida, campos extras/duplicados, nulls,
trailing JSON, CRLF e formas não canônicas de base64. Validar origem/IDs/instante
e binding de 32 bytes. O caso de uso confere o vínculo ao ator/unidade/filtros e
à autoridade atual. Não tratar hash/base64 como assinatura ou autenticação.

next_cursor vazio indica fim. Reutilizar o cursor com os mesmos filtros
normalizados; tamanho da página pode mudar entre 1 e 100. Uma nova página
revalida o acesso e observa uma nova transação, sem snapshot congelado entre GETs.

## Verificação e próximos passos

Testes do adaptador verificam parâmetros, contexto, JSON, privacidade, cursor,
erros e timeout. Testes do router usam assinaturas ES256/JWKS reais com
persistência substituída, cobrindo owner/barber, acesso negado, GET/OPTIONS e
ausência de informações sensíveis nos logs. O teste PostgreSQL usa o verifier
de fixture já existente, com autenticação local, membership, casos de uso e
consultas reais: totais por perfil, páginas, mudança de filtros/ator, revogação e
anulação posterior. Doze cenários PostgreSQL de reporting passaram no banco
descartável; não usar esses fixtures como acesso de produção.

Também passaram go vet, go test ./..., builds API/worker e Compose. Sintaxe YAML,
referências locais e propriedades required do OpenAPI foram verificadas com a
biblioteca YAML já disponível no cache, sem novas dependências no projeto.

docs/reporting-openapi.yaml e as referências em docs/openapi.yaml descrevem as
rotas reais. A interface do usuário foi preservada. Próximo incremento: integrar
o painel gerencial da entrega H aos endpoints, mantendo o redesign existente.
