# ADR 0006 — Carregamento automático de .env

Status: adotada a pedido do usuário. Data: 2026-09-22.

## Contexto

Iniciar API e worker exigia importar o `.env` manualmente em cada terminal. A política de execução do PowerShell bloqueou esse passo no ambiente do usuário. O usuário pediu inicialização direta com `go run`.

## Decisão

`config.Load()`, já utilizado pelos dois executáveis, lê o `.env` opcional do diretório de execução com `github.com/joho/godotenv` v1.5.1. A dependência resolve o parsing de dotenv, incluindo aspas e comentários, sem manter um parser próprio. Fonte e versão verificadas no [projeto oficial](https://github.com/joho/godotenv) e na [release v1.5.1](https://github.com/joho/godotenv/releases/tag/v1.5.1).

Usar `godotenv.Read` e consultar primeiro `os.LookupEnv` preserva o ambiente do processo, inclusive valores explicitamente vazios. O mapa do arquivo serve como fallback somente para variáveis ausentes. A validação existente continua obrigatória. Não usar autoload por efeito de importação nem substituir valores do ambiente.

Somente arquivo inexistente é ignorado. Erros de acesso ou parsing interrompem a inicialização sem incluir o erro original, que pode conter credenciais. Não procurar arquivos em diretórios ancestrais. Em produção, fornecer variáveis pela plataforma; o arquivo não é obrigatório nem deve ser incluído na imagem ou no Git.

## Consequências e validação

Na raiz do projeto, `go run ./cmd/api` e `go run ./cmd/worker` passam a carregar a configuração automaticamente. A leitura ocorre uma vez no início: alterações exigem reiniciar o processo. Variáveis antigas importadas no terminal continuam tendo prioridade; abrir um novo terminal elimina as importações daquela sessão.

Goose e testes de integração continuam recebendo variáveis separadamente; não usam `config.Load()`. O script PowerShell continua disponível para eles. O parser dotenv permite interpolação em valores sem aspas ou entre aspas duplas; usar aspas simples para `$` literal. O script PowerShell mantém seu formato simples sem interpolação.

Testes cobrem leitura do arquivo, prioridade do ambiente, valores vazios explícitos, ausência do arquivo, validação de campos obrigatórios, falhas de leitura/parsing e proteção contra exposição de conteúdo em erros. Esta decisão atualiza a escolha inicial de configuração sem parser do ADR 0003.
