# ADR 0014 — Histórico e indicadores como leituras derivadas

Data: 12/10/2026. Status: arquitetura proposta e fundação implementada; consultas, mudanças de status e endpoints ainda não implementados.

## Contexto e decisão

O módulo `reporting` já existe. Utilizá-lo para histórico administrativo e indicadores, sem novo módulo de atendimentos, segunda tabela de serviços realizados, CRM, banco analítico ou microsserviço. `booking` permanece dono do agendamento e das transições; `customers` mantém o contato já necessário à reserva; `identity` mantém autorização. Reporting possui seus DTOs/projeções e portas de leitura, sem importar application/domain de outros módulos.

Os adaptadores SQL poderão fazer joins revisados sobre tabelas desses módulos, sempre somente leitura, com `shop_id` e filtros antes da paginação/agregação. Usam o pool compartilhado. Isso é separação de comandos e consultas sem adicionar infraestrutura de CQRS distribuído. Não criar materialized views/cache antes de medir consultas reais.

## Regras propostas para a primeira entrega

- Histórico inclui todos os status, com filtro opcional, nome, telefone normalizado, serviço, profissional e período. Nome/telefone atuais vêm do contato local; nomes/preço/duração/moeda do serviço e nome do profissional vêm dos snapshots da reserva. Mudanças de contato podem refletir no histórico, e isso precisa estar claro no contrato.
- Datas `from` e `through` são inclusivas no fuso da unidade. A application obtém esse fuso do servidor e usa o domínio para gerar `[from,until)` em UTC. Limite inicial: 366 datas civis por consulta; 25 itens por página, máximo 100; keyset descendente `(starts_at,id)`. O cursor não autoriza e deve ser validado contra período/filtros na implementação.
- O marco temporal inicial é `appointments.starts_at`, inclusive para agrupar por dia. Não usar `created_at`, `updated_at` ou fingir que representam data de pagamento. Uma conclusão tardia corrige o indicador da data agendada. Mudança posterior para data de conclusão deve ser explícita/versionada.
- Somente `completed` conta como realizado. `scheduled` e `confirmed` são valores previstos; `cancelled` e `no_show` não geram valor realizado. Data passada não muda estado automaticamente.
- Preços históricos são snapshots em centavos; não atualizar com o catálogo atual. Moedas permanecem separadas. Ticket médio = valor de concluídos / quantidade de concluídos da mesma moeda; divisor zero gera ausência de ticket, não erro ou número inventado.
- Painéis de contagem, valores, dia, profissional e serviço usam o mesmo recorte e snapshot de leitura: uma query ou transação curta read-only/repeatable-read. Não carregar agregados completos nem contar multiplicações causadas por joins muitos-para-muitos.
- Agrupamento por profissional/serviço usa ID, não nome mutável. Rótulo: snapshot mais recente no período, ordenado por `(starts_at,id)`, sem divisão artificial em grupos ao renomear.
- Primeiro acesso ao histórico/indicadores: proprietário apenas. Contatos/telefone são privados; telefone não autentica uma pessoa, não libera histórico público e não é um identificador único de pessoa. Código da reserva continua limitado a uma reserva. Não expor código/hash/recibo cifrado em reporting.

## Pré-requisito em booking

O schema e enum já aceitam `completed`/`no_show`, mas o código executa criação e cancelamento. Primeiro implementar transições administrativas explícitas, idempotência, controle de versão, auditoria com ator/data/status anterior/novo, revalidação de membership na transação e o lock comum do profissional. Não inferir conclusão por data nem criar endpoint público para concluir atendimento.

Definir com o usuário quando concluir/registrar ausência e como corrigir enganos. Manter estados finais fechados inicialmente; uma correção futura deve registrar auditoria, não SQL manual silencioso. Nova migration deve ser criada quando essa implementação começar; não editar migrations aplicadas nem dar baixa em reservas antigas automaticamente.

## Serviços realizados não são caixa

`CompletedServiceValueCents` representa o valor reservado de serviços concluídos. Não comprova pagamento, descontos, taxas, estornos, lucro ou apuração fiscal. Primeiro painel deve usar “Valor dos serviços realizados”, não “Recebido”. Se o usuário pedir recebimentos reais, decidir antes um pequeno modelo de pagamentos/ajustes, com valores efetivos e `paid_at`, separado da projeção de reporting. Ainda não criar módulo financeiro.

## IA e cloud: direção, não contratação

Primeiro caso de IA sugerido: explicar indicadores agregados já calculados pelo SQL, somente leitura e sem contatos. Depois, interpretar pedidos no chat e propor escolhas usando ferramentas restritas. IA não decide disponibilidade, preço, permissões ou confirmação de reserva; o usuário confirma e booking executa com as mesmas proteções existentes.

Começar com adaptador de um provedor de inferência na aplicação Go; não criar módulo genérico `ai` nem microsserviço sem consumidor. Reporting pode receber uma porta específica de explicação quando o caso de uso existir. Trabalho pesado/assíncrono pode usar worker. Python só se treinamento, processamento local ou bibliotecas específicas justificarem; extração exige contrato, autenticação interna, observabilidade e orçamento próprios.

AWS pode começar antes dos relatórios, em staging isolado. Direção educacional inicial: EC2 + containers da API/Next + HTTPS, IAM role/Session Manager, logs e Supabase separado para dados/Auth. Ainda não foi criado recurso nem Dockerfile de deploy. S3 é opcional para objetos/exportações futuras, não substitui PostgreSQL/Go/SSR; RDS/ECS vêm depois de medir e aprender. Migrar DB para RDS exige redesenhar o vínculo `public.users -> auth.users` existente, não apenas trocar a URL.

## Estado desta entrega

Criados `domain/{errors,period,status}.go`, `application/{dto,ports,errors}.go` e testes de domínio. Somente período/status têm comportamento executável. Nenhum handler, SQL de relatório, migration, provedor de IA, pacote Python, worker de negócio ou infraestrutura cloud foi conectado. `doc.go` e READMEs foram preservados. A trilha detalhada está em [learning-roadmap.md](../learning-roadmap.md).
