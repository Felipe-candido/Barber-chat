# Go para quem vem de Python, Django e FastAPI

Este guia explica o código executável atual do Barber-chat, usando Django e FastAPI como referência. Ele descreve o estado presente do repositório: health checks, configuração, PostgreSQL, RabbitMQ e ciclo de vida do worker. Ainda não há agendamento, ORM, modelos de negócio, CRUD, autenticação ou envio de mensagens.

Os links para os arquivos usam números de linha. Quando o código mudar, os números podem mudar; a ideia é abrir o arquivo e acompanhar os blocos citados.

## Mapa rápido: conceitos conhecidos e equivalências

| Ideia | Python/Django/FastAPI | Go neste projeto | Observação importante |
| --- | --- | --- | --- |
| Dependências do projeto | `pyproject.toml`, `requirements.txt` | [go.mod](../go.mod) e `go.sum` | `go.mod` fixa módulos diretos; `go.sum` verifica conteúdo baixado. |
| Pacote | diretório importável com `__init__.py` | diretório de `.go` com `package nome` | Go não precisa de `__init__.py`. Todos os arquivos do mesmo diretório normalmente usam o mesmo `package`. |
| Aplicação executável | `uvicorn app.main:app`, `manage.py runserver`, comando Django | `go run ./cmd/api` ou `go run ./cmd/worker` | Cada diretório em `cmd` cria um binário por ter `package main` e `func main()`. |
| ASGI/router | `FastAPI()` e decorators como `@app.get()` | `chi.NewRouter()` e `mux.Get()` | Chi usa a interface padrão `net/http`. |
| View/endpoint | função decorada, `APIView`, viewset | `http.Handler` ou função `(http.ResponseWriter, *http.Request)` | Não há injeção automática de request models ainda. |
| Settings | `django.conf.settings`, `BaseSettings`/Pydantic Settings | `config.Config` + `config.Load()` | Tipos e validação são escritos explicitamente. |
| Logger | `logging.getLogger(__name__)` | `slog.Logger` | Ambos permitem campos estruturados; `slog` é da biblioteca padrão. |
| Dependency injection | `Depends(...)`, container, fixtures | parâmetros de funções e composição em `cmd` | Go favorece passar dependências explicitamente. |
| Contexto/requisição cancelada | `request`, cancellation de ASGI, timeout de client | `context.Context` | `context` transporta cancelamento, deadline e valores por chamadas. |
| Pool de banco | pool do driver/SQLAlchemy/Django connection handling | `pgxpool.Pool` | O pool é compartilhado pelo processo, não um por módulo de negócio. |
| Worker | Celery/RQ/Dramatiq management command | `cmd/worker` + `internal/worker` | Ainda é um scaffold; não consome mensagens de negócio. |
| Fila RabbitMQ | Kombu/Celery transport | `amqp091-go` | O cliente AMQP é usado diretamente; não há abstração Celery equivalente. |
| Migrações | `python manage.py migrate` | Goose CLI em `db/migrations` | Migrations não executam automaticamente quando a API inicia. |

## Como Go organiza este projeto

### Módulo, pacote e executável

Em Python, um repositório pode ter vários pacotes e scripts de entrada. Em Go, o [go.mod](../go.mod) declara um módulo, aqui `github.com/Felipe-candido/Barber-chat`. É comparável ao nome de distribuição definido em um `pyproject.toml`, embora também faça parte do caminho de import.

Um **pacote Go** é um diretório. Por exemplo, `internal/config` contém arquivos que iniciam com `package config`; seu import é `github.com/Felipe-candido/Barber-chat/internal/config`.

`internal` é uma regra aplicada pelo compilador: código fora deste repositório não pode importar os pacotes abaixo dele. Isso se aproxima de um pacote Python privado por convenção, mas é efetivamente imposto pelo Go.

Um executável é um pacote chamado `main` que possui `func main()`. Os dois existentes são:

```text
cmd/api/main.go       -> go run ./cmd/api       -> API HTTP
cmd/worker/main.go    -> go run ./cmd/worker    -> processo de background
```

Isso lembra dois comandos Django, como `manage.py runserver` e um comando Celery worker, mas os dois são binários Go independentes. Eles compartilham pacotes internos e a mesma versão, sem serem microsserviços.

### Exportação e nomes

Python exporta tudo que não começa com `_` por convenção. Go usa maiúscula inicial:

```go
func Load()          // exportada: outro pacote pode chamar config.Load
func load(...)       // privada ao pacote config
type Config struct   // exportada
```

Não há classes obrigatórias. Métodos podem existir, mas composição por structs e funções é comum. Uma interface é satisfeita implicitamente: não existe uma declaração equivalente a `class X(Protocol)` ou `implements`.

### Erros em vez de exceções

Em Python, uma operação pode lançar `ValueError`, que sobe até ser capturada. Em Go, o padrão é retornar `error`:

```go
cfg, err := config.Load()
if err != nil {
    // lidar com a falha aqui
}
```

`err` é um valor comum. Isso torna o caminho de falha visível em cada chamada. Go tem `panic`, mas ele é reservado para erros de programação irrecuperáveis, não para fluxo normal de validação ou banco indisponível.

## A API: cmd/api/main.go

Arquivo: [cmd/api/main.go](../cmd/api/main.go).

### Linhas 1–14: pacote e imports

```go
package main
```

É o equivalente conceitual de um módulo Python executado como script, mas a convenção do compilador exige literalmente o nome `main` para gerar programa.

Os imports entre aspas simples, como `"context"`, são biblioteca padrão. Os caminhos com `github.com/...` são pacotes do módulo atual. Go agrupa imports e o `gofmt` organiza essa lista automaticamente.

| Import | Papel | Aproximação Python |
| --- | --- | --- |
| `context` | cancelamento/prazo propagado | cancellation scope de ASGI, `asyncio` timeout, mas explícito no argumento |
| `log/slog` | logger JSON estruturado | `logging` com formatter JSON/structlog |
| `net` | abre listener TCP | socket/server criado pelo Uvicorn/Gunicorn |
| `os` | stdout e código de saída | `sys.stdout`, `sys.exit` |
| `os/signal`, `syscall` | SIGINT/SIGTERM | signal handlers normalmente geridos pelo servidor/process manager |
| `config` | carrega e valida ambiente | `pydantic-settings`, `django.conf.settings` |
| `httpapi` | roteador/servidor HTTP | módulo que contém `FastAPI()` e routes |
| `postgres` | cria pool pgx | factory de engine/session pool SQLAlchemy |

### Linhas 16–32: função main

```go
func main() {
```

É o ponto em que o sistema operacional inicia o programa. Não recebe argumentos. Se precisarmos de argumentos de linha de comando, eles virão de `os.Args` ou de uma biblioteca de CLI no futuro.

Na [linha 17](../cmd/api/main.go:17):

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api")
```

O operador `:=` declara uma variável local e deixa o compilador inferir o tipo. A expressão ocorre de dentro para fora:

1. `os.Stdout` é a saída padrão, normalmente o terminal ou o coletor de logs do container.
2. `slog.NewJSONHandler(...)` decide o formato JSON.
3. `slog.New(...)` cria o logger.
4. `.With("service", "api")` devolve um logger derivado que sempre inclui `service=api`.

Em Python, a ideia seria aproximadamente:

```python
logger = logging.getLogger("api")
# com formatter JSON e contexto service="api" configurados
```

O logger inicial não usa ainda `cfg.LogLevel`, porque a configuração ainda não foi carregada. Assim podemos registrar uma falha de configuração.

Nas [linhas 18–22](../cmd/api/main.go:18):

```go
cfg, err := config.Load()
if err != nil {
    logger.Error("configuration failed", "error", err)
    os.Exit(1)
}
```

`Load` retorna dois valores. Isso é parte normal da linguagem, não um tipo especial como `Result` de Rust. O teste `err != nil` equivale ao ponto em que, em Python, você usaria `try/except` para uma exceção de configuração. `os.Exit(1)` é equivalente a `raise SystemExit(1)`.

Na [linha 23](../cmd/api/main.go:23), o logger é recriado com o nível configurado. `&slog.HandlerOptions{...}` cria uma struct e obtém seu ponteiro. Ponteiros em Go são referências controladas: não há aritmética de ponteiro normal no código de aplicação.

Nas [linhas 24–26](../cmd/api/main.go:24):

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
err = run(ctx, cfg, logger)
stop()
```

`context.Background()` é o contexto raiz da aplicação. `NotifyContext` cria outro contexto que é cancelado quando ocorre Ctrl+C (`os.Interrupt`) ou SIGTERM. O valor `stop` é uma função de limpeza; chamá-la para de observar sinais.

Em FastAPI/Uvicorn, esse ciclo é gerenciado pelo servidor ASGI. Em Django/Gunicorn, o process manager costuma receber SIGTERM. Em Go, a aplicação participa diretamente dessa gestão. O `ctx` é passado explicitamente; qualquer operação que aceite esse contexto pode parar cooperativamente.

Nas [linhas 27–31](../cmd/api/main.go:27), o programa registra erro e sai com código 1 ou registra término normal. `main` não deve usar `defer` para recursos que precisam sobreviver a `os.Exit`, porque `os.Exit` não executa funções adiadas. A próxima função resolve isso.

### Linhas 34–46: função run

```go
func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
```

`run` recebe dependências já preparadas. Isso é injeção de dependências explícita. Uma aproximação FastAPI seria uma função de lifespan recebendo settings e engine; em Django é parecido com preparar recursos no startup do processo. A diferença é que os parâmetros mostram todas as dependências diretamente.

Na [linha 35](../cmd/api/main.go:35), `postgres.Open` prepara `*pgxpool.Pool`. O `*` no tipo significa ponteiro para pool. A API manipula uma referência compartilhada, não copia o pool inteiro.

Na [linha 39](../cmd/api/main.go:39):

```go
defer pool.Close()
```

`defer` agenda uma chamada para a saída da função atual. É parecido com colocar limpeza em um `finally`, mas a chamada é registrada perto da aquisição do recurso:

```python
pool = open_pool()
try:
    ...
finally:
    pool.close()
```

Isso explica a separação entre `main` e `run`: quando `run` retorna por qualquer caminho, o pool fecha antes de `main` poder chamar `os.Exit`.

Na [linha 40](../cmd/api/main.go:40), `net.Listen("tcp", cfg.HTTPAddr)` reserva uma porta TCP. Uvicorn ou Gunicorn faz esse trabalho internamente em Python. O resultado é um `listener`, um objeto que aceita conexões.

Na [linha 45](../cmd/api/main.go:45):

```go
return httpapi.Serve(
    ctx,
    listener,
    httpapi.NewHandler(pool.Ping, cfg.DBTimeout, logger),
    cfg.ShutdownTimeout,
    logger,
)
```

`pool.Ping` é passado sem parênteses: é uma referência à função, como passar `pool.ping` em Python, e não chamar `pool.ping()`. O handler poderá executá-la ao receber `/ready`. Isso substitui uma forma pesada de mock ou container: os testes passam uma função simples no lugar de um banco real.

## Configuração: internal/config/config.go

Arquivo: [internal/config/config.go](../internal/config/config.go).

### Linhas 14–22: Config como dataclass tipada

```go
type Config struct {
    HTTPAddr        string
    DatabaseURL     string
    RabbitMQURL     string
    LogLevel        slog.Level
    DBTimeout       time.Duration
    ShutdownTimeout time.Duration
    WorkerInterval  time.Duration
}
```

É muito próximo de uma dataclass ou modelo Pydantic já validado:

```python
@dataclass(frozen=True)
class Config:
    http_addr: str
    database_url: str
    rabbitmq_url: str
    db_timeout: timedelta
```

Go não valida automaticamente tipos de variáveis de ambiente. A função `Load` faz essa conversão. Campos com maiúscula são acessíveis de outros pacotes; em Python seriam atributos públicos convencionais.

### Linhas 24–28: função pública e função testável

```go
func Load() (Config, error) {
    return load(os.Getenv)
}
```

`Load` é a API pública. `load` com letra minúscula é interna e recebe uma função `getenv func(string) string`. Isso é uma dependência injetada: produção entrega `os.Getenv`; testes entregam uma função que consulta um mapa.

Em Python, uma versão seria:

```python
def load(getenv: Callable[[str], str]) -> Config:
    ...

def load_from_os() -> Config:
    return load(os.getenv)
```

### Linhas 29–39: padrão e construção inicial

```go
value := func(key, fallback string) string { ... }
```

É uma função anônima guardada em uma variável local. Em Python:

```python
def value(key: str, fallback: str) -> str:
    return getenv(key) or fallback
```

O `if v := getenv(key); v != ""` declara `v` somente dentro do `if`. A struct nas [linhas 35–39](../internal/config/config.go:35) é criada com uma literal, parecida com `Config(http_addr=..., database_url=...)`.

`DATABASE_URL` não possui padrão propositalmente: iniciar apontando para banco desconhecido seria perigoso. RabbitMQ pode ficar vazio na API, porque só o worker o exige.

### Linhas 40–54: validação explícita

`net.SplitHostPort` separa host e porta. `_` descarta o host, porque esse bloco só valida a porta. `strconv.Atoi` é equivalente a `int(port)`.

```go
if err != nil || portErr != nil || n < 1 || n > 65535 {
```

`||` é `or`. A condição rejeita formato inválido, conversão inválida e portas fora do intervalo.

As [linhas 45–52](../internal/config/config.go:45) chamam `validateURL`. Note a forma curta:

```go
if err := validateURL(...); err != nil {
    return Config{}, err
}
```

Ela declara `err` dentro do `if`, como no `if v := ...`. `Config{}` é uma configuração vazia, equivalente aproximado a `Config(...)` sem valores em uma estrutura que usa zero-values. Como o segundo retorno é erro, o chamador não deve usar o primeiro valor.

Na [linha 53](../internal/config/config.go:53), `UnmarshalText` converte o texto para `slog.Level`. É a conversão explícita que Pydantic faria automaticamente a partir de uma variável de ambiente.

### Linhas 56–70: loop, slice, struct anônima e ponteiro

```go
for _, setting := range []struct { ... }{ ... } {
```

Isso cria uma lista (`slice`) de structs temporárias e percorre seus itens. `_` ignora o índice. O equivalente Python é:

```python
for key, fallback, target in [
    ("DB_TIMEOUT", "3s", "db_timeout"),
    ...
]:
    ...
```

Go não possui reflexão natural para definir campos por string, então a struct guarda o endereço do campo a alterar:

```go
{"DB_TIMEOUT", "3s", &c.DBTimeout}
```

`&c.DBTimeout` significa “ponteiro para o campo”. Mais tarde, `*setting.target = d` escreve no valor apontado. Isto é análogo a `setattr(c, target_name, duration)`, mas com verificação de tipo em tempo de compilação e sem usar reflexão.

`time.ParseDuration` converte `"3s"`, `"500ms"` ou `"2m"` em `time.Duration`. Este tipo representa uma quantidade de tempo, comparável a `datetime.timedelta`.

### Linhas 74–85: URL sem vazar segredo

`schemes ...string` aceita quantidade variável de argumentos. Assim, a função pode receber `"postgres", "postgresql"` ou `"amqp", "amqps"`.

O erro não mostra a URL completa porque URLs de conexão costumam carregar usuário e senha. É semelhante a evitar `repr(settings.database_url)` em logs de Python.

## HTTP e Chi: internal/httpapi/server.go

Arquivo: [internal/httpapi/server.go](../internal/httpapi/server.go).

### Linhas 16–34: construir o handler

```go
func NewHandler(
    checkDB func(context.Context) error,
    timeout time.Duration,
    logger *slog.Logger,
) http.Handler {
```

FastAPI cria o roteador e registra rotas no import do módulo. Aqui, `NewHandler` é uma factory que recebe suas dependências e retorna `http.Handler`, a interface padrão de resposta HTTP. Isso permite montar a aplicação sem variável global de banco.

```go
mux := chi.NewRouter()
mux.Use(middleware.RequestID)
mux.Use(middleware.Recoverer)
```

É próximo a:

```python
app = FastAPI()
app.add_middleware(RequestIDMiddleware)
app.add_middleware(ExceptionOrCrashRecoveryMiddleware)
```

`RequestID` coloca um identificador na requisição. `Recoverer` captura panics ocorridos em um handler e evita derrubar o processo. Ele não deve ser usado para esconder erros de domínio; esses devem ser retornados e convertidos em respostas apropriadas.

As [linhas 20–22](../internal/httpapi/server.go:20) registram `GET /health`. A forma equivalente é:

```python
@app.get("/health")
def health():
    return {"status": "ok"}
```

O handler Go recebe explicitamente `w` para resposta e `r` para request. FastAPI converte retorno Python em JSON; Go escreve headers/status/corpo manualmente neste scaffold.

Nas [linhas 23–32](../internal/httpapi/server.go:23), `/ready` cria um contexto de prazo:

```go
ctx, cancel := context.WithTimeout(r.Context(), timeout)
defer cancel()
```

`r.Context()` carrega o cancelamento causado por desconexão do cliente ou encerramento do servidor. `WithTimeout` gera um filho que também expira em `timeout`. `defer cancel()` impede manter timer/referências além da requisição.

Uma aproximação FastAPI seria `asyncio.timeout(...)` em torno do ping. A diferença é que o contexto precisa ser propagado até o driver; pgx o recebe e pode interromper a operação. Contextos não devem ser colocados em structs nem substituídos por variáveis globais.

O `return` depois da resposta 503 é essencial: HTTP só pode enviar um status por resposta.

### Linhas 36–41: resposta JSON pequena

```go
w.Header().Set("Content-Type", "application/json")
w.Header().Set("Cache-Control", "no-store")
w.WriteHeader(code)
_, _ = fmt.Fprintf(w, "{\"status\":%q}\n", status)
```

Em FastAPI, `return {"status": status}` define JSON e status automaticamente. Em Go, a sequência é manual: headers primeiro, depois `WriteHeader`, depois corpo. Depois que corpo/status começa, headers não podem ser alterados.

`%q` serializa a string com aspas e escapes. Para respostas de negócio futuras, usaremos structs e `encoding/json`, em vez de construir JSON como texto.

`Fprintf` devolve quantidade de bytes e erro; `_` descarta ambos. Uma falha de escrita normalmente significa que o cliente foi embora; não há resposta alternativa possível.

### Linhas 43–73: servidor e graceful shutdown

`http.Server` é o equivalente mais baixo que Uvicorn/Gunicorn para esse programa. Os timeouts reduzem risco de conexões lentas prenderem recursos:

| Campo | Efeito | Aproximação Python |
| --- | --- | --- |
| `ReadHeaderTimeout` | prazo para receber cabeçalhos | proteção de servidor/proxy contra slow headers |
| `ReadTimeout` | prazo para ler corpo/requisição | request timeout de servidor |
| `WriteTimeout` | prazo para escrever resposta | response timeout |
| `IdleTimeout` | prazo para conexão keep-alive ociosa | keep-alive timeout |

Nas [linhas 53–54](../internal/httpapi/server.go:53):

```go
done := make(chan error, 1)
go func() { done <- server.Serve(listener) }()
```

`make(chan error, 1)` cria um canal com capacidade de um erro. Pense nele como uma fila segura entre goroutines. `go` cria uma goroutine, uma unidade de concorrência leve gerida pelo runtime Go. Isso lembra criar uma task com `asyncio.create_task`, mas não exige que a função seja `async`; chamadas bloqueantes são comuns em goroutines.

`server.Serve` bloqueia enquanto o servidor está ativo. Ao parar, sua goroutine envia o resultado para `done` com `done <- ...`.

O `select` das [linhas 55–72](../internal/httpapi/server.go:55) espera o primeiro evento disponível:

```go
case err := <-done:      // servidor parou sozinho
case <-ctx.Done():       // sinal de encerramento chegou
```

`ctx.Done()` devolve um canal fechado ao cancelar. Receber de um canal fechado não bloqueia, então o `select` avança.

Quando há sinal, o código cria `shutdownCtx` a partir de `context.Background()`. Ele não reutiliza o `ctx` já cancelado; se reutilizasse, `Shutdown` receberia imediatamente um contexto cancelado e não teria prazo para drenar requisições.

`server.Shutdown(shutdownCtx)` impede novas conexões e espera handlers ativos. Se o prazo expirar, `server.Close()` força fechamento. Isso corresponde à fase de graceful shutdown que Uvicorn/Gunicorn executa antes de matar workers, mas está explícita no programa.

`errors.Is(err, http.ErrServerClosed)` reconhece o erro esperado produzido pelo próprio encerramento. É equivalente à ideia de comparar uma exceção com uma classe/sentinela específica, mas também funciona com erros embrulhados.

## PostgreSQL: internal/platform/postgres/postgres.go

Arquivo: [internal/platform/postgres/postgres.go](../internal/platform/postgres/postgres.go).

`pgxpool.Pool` é mais próximo de um pool asyncpg/psycopg ou de um engine SQLAlchemy do que de um `QuerySet` Django. Não é ORM: SQL e mapeamento de resultados serão escritos explicitamente em adaptadores `infra` dos módulos.

Nas [linhas 12–24](../internal/platform/postgres/postgres.go:12):

1. `pgxpool.ParseConfig(databaseURL)` analisa a URL.
2. `cfg.MaxConns = 5` define o máximo de conexões desse processo.
3. `ConnectTimeout` limita a abertura de conexões.
4. `RuntimeParams["timezone"] = "UTC"` faz cada sessão do banco trabalhar em UTC.
5. `pgxpool.NewWithConfig` cria o pool.

O comentário diz que o pool é lazy: criá-lo não prova que PostgreSQL está acessível. A rota `/ready` recebe `pool.Ping` e verifica conectividade separadamente. Isso evita misturar startup de processo com liveness/readiness de dependência.

Em uma implementação futura, `booking/infra` receberá esse pool para executar SQL específico de agenda. Não deve chamar `postgres.Open` por conta própria, assim como uma view Django não deveria criar um novo `Engine` SQLAlchemy por request.

## RabbitMQ: internal/platform/rabbitmq/rabbitmq.go

Arquivo: [internal/platform/rabbitmq/rabbitmq.go](../internal/platform/rabbitmq/rabbitmq.go).

Em Python, Celery normalmente esconde a conexão AMQP. Aqui o código usa o protocolo por meio do cliente `amqp091-go`, justamente para permitir estudar exchanges, filas, confirms e ACKs quando o worker for implementado.

Na [linha 13](../internal/platform/rabbitmq/rabbitmq.go:13), RabbitMQ é obrigatório para o worker. A API pode iniciar sem ele, pois a futura outbox permitirá persistir a reserva antes da publicação no broker.

`amqp.DialConfig` nas [linhas 16–34](../internal/platform/rabbitmq/rabbitmq.go:16) recebe:

- `Heartbeat: 10 * time.Second`: detecta conexão inativa/caída;
- uma função `Dial` personalizada que usa `net.Dialer` e respeita contexto/timeout.

O callback de dial é parecido com injetar um conector de rede no client. Ele calcula o menor entre o timeout local e o deadline que já veio no contexto. Enquanto o handshake AMQP está em andamento, define deadline no socket; após a conexão abrir, a biblioteca remove essa limitação de socket para o funcionamento normal.

Erros públicos são deliberadamente genéricos. Uma URL com senha não deve aparecer em logs.

## Worker: cmd/worker e internal/worker

O [cmd/worker/main.go](../cmd/worker/main.go) repete a estrutura do main da API: logger, configuração, contexto por sinal e `run`. A diferença é que abre PostgreSQL e RabbitMQ antes de chamar `worker.Run`.

Nas [linhas 45–51](../cmd/worker/main.go:45), uma função anônima é registrada com `defer` para fechar RabbitMQ. `time.Now().Add(cfg.ShutdownTimeout)` é calculado no encerramento, então o prazo sempre começa naquele momento.

A [linha 52](../cmd/worker/main.go:52) passa três ideias ao worker:

```go
worker.Run(
    ctx,
    pool.Ping,
    conn.NotifyClose(make(chan *amqp.Error, 1)),
    cfg.WorkerInterval,
    cfg.DBTimeout,
    logger,
)
```

| Argumento | Papel | Aproximação Python |
| --- | --- | --- |
| `ctx` | sinal global de parada | evento de shutdown de Celery/process manager |
| `pool.Ping` | função de checagem de banco | callable injetado/fake em testes |
| `NotifyClose(...)` | canal avisado se RabbitMQ fechar | callback/evento de perda de conexão do client |
| `interval` | frequência de verificação | loop com `sleep`, mas com ticker Go |
| `timeout` | prazo de cada ping | timeout de operação |

No [internal/worker/worker.go](../internal/worker/worker.go), a [linha 13](../internal/worker/worker.go:13) recebe `brokerClosed <-chan *amqp.Error`. A seta no tipo indica canal apenas de leitura para essa função. Ela pode aguardar o evento, mas não pode enviar eventos falsos para ele.

As [linhas 14–18](../internal/worker/worker.go:14) criam uma closure chamada `check`. Uma closure pode usar variáveis da função externa, como `ctx`, `timeout` e `checkDB`, de modo parecido com função interna Python. Ela cria um contexto com prazo para cada ping e chama `cancel` por `defer`.

As [linhas 25–26](../internal/worker/worker.go:25) criam o ticker. Diferente de um loop `while True: time.sleep(interval)`, `time.NewTicker` entrega eventos em `ticker.C`. `defer ticker.Stop()` impede que ele permaneça ativo depois do retorno.

O laço nas [linhas 28–47](../internal/worker/worker.go:28) usa `select` para esperar simultaneamente:

- cancelamento global;
- fechamento do broker;
- próximo tick de verificação.

Não há polling ativo. A goroutine fica bloqueada até um desses eventos chegar. Hoje, perda do broker ou do banco causa retorno com erro para que um supervisor possa reiniciar o processo. A lógica de reconectar/consumir/retry ainda não existe.

Em Celery, boa parte desse lifecycle é escondida pelo framework. Aqui ele está visível porque o objetivo é aprender a infraestrutura e porque não queremos introduzir uma segunda abstração antes de existirem jobs reais.

## Arquitetura: Django apps, FastAPI routers e módulos Go

Os módulos em `internal/modules` são comparáveis a Django apps organizados por capacidade de negócio, mas não têm modelos/rotas ativos ainda:

```text
internal/modules/booking/
├── domain/        regras e tipos de negócio
├── application/   casos de uso e portas
└── infra/         SQL, HTTP, AMQP e providers concretos
```

| Camada Go | Papel | Paralelo Django/FastAPI | Não deve conter |
| --- | --- | --- | --- |
| `domain` | regras que fazem sentido sem rede/banco | parte pura de models/services, value objects | `Request`, `Response`, pgx, SQL, AMQP, SDK externo |
| `application` | orquestra um caso de uso | service layer, command handler, dependência de caso de uso | detalhes do driver ou handler HTTP concreto |
| `infra` | adapta mundo externo | repositories SQLAlchemy/Django ORM, routers/views, clients Celery/HTTP | regra central duplicada |

Um fluxo futuro de criar agendamento terá forma aproximada:

```text
POST /v1/public/shops/{slug}/appointments
    -> handler em booking/infra
    -> CreateAppointment em booking/application
    -> regras/estados em booking/domain
    -> SQL transacional em booking/infra
    -> cliente e intenção de notificação na mesma transação
```

O desenho é semelhante a um projeto Django que evita concentrar toda a regra em view/model signal, ou a um FastAPI que usa routers finos e service layer. A diferença é que Go torna as dependências visíveis por imports e parâmetros, enquanto Django frequentemente as oferece por modelos globais, ORM e settings.

Não crie um `domain` genérico na raiz do repositório. Cada domínio pertence ao módulo que o possui: `booking/domain`, `customers/domain`, `catalog/domain` etc. `internal/platform` é diferente de `infra`: ele é infraestrutura genérica compartilhada, como abrir um pool; `booking/infra` será a infraestrutura específica da agenda, como SQL para criar/reservar horários.

## Tradução prática de sintaxe

| Go | Python aproximado | Significado |
| --- | --- | --- |
| `x := value` | `x = value` | declara e atribui, inferindo tipo. Só pode ser usado dentro de funções. |
| `var x T` | `x: T` | declara sem valor explícito; recebe zero-value. |
| `T{Field: value}` | `T(field=value)` | literal de struct. |
| `&x` | referência de `x` | obtém ponteiro/endereço. |
| `*ptr` | desreferência | lê ou escreve valor apontado. |
| `defer f()` | `try/finally` | agenda `f` para saída da função atual. |
| `func(...) error` | `def ... -> Exception?` | retorna erro explícito em vez de lançar no fluxo comum. |
| `nil` | `None` | ausência para ponteiros, interfaces, slices, maps, channels e errors. |
| `[]T` | `list[T]` | slice: visão dinâmica sobre array. |
| `map[K]V` | `dict[K, V]` | mapa tipado. |
| `go f()` | task concorrente | inicia goroutine. |
| `chan T` | fila/canal assíncrono | comunicação sincronizada entre goroutines. |
| `select` | esperar primeiro evento | escolha entre operações de canais. |
| `interface` | `Protocol` estrutural | satisfeita implicitamente por métodos compatíveis. |

## O que evitar ao trazer hábitos de Django/FastAPI

- Não criar modelos globais que qualquer módulo pode usar sem limite. Cada módulo é dono de seus dados e regras.
- Não criar uma camada `repository` genérica para todas as entidades. Um repositório só entra quando um caso de uso concreto precisa dele.
- Não criar interfaces para tudo. Em Go, uma interface pequena costuma ser definida pelo consumidor, próximo ao caso de uso que precisa dela.
- Não usar `panic` como se fosse `raise HTTPException`. Erros esperados precisam voltar como `error` e ser mapeados no handler.
- Não guardar `context.Context` dentro de struct. Passe `ctx` como primeiro argumento a cada operação que possa bloquear.
- Não iniciar conexão de banco por request ou por módulo. Use um pool criado no startup e injetado nos adaptadores.
- Não usar goroutine como substituto de fila durável. Uma goroutine desaparece se o processo cair; lembretes futuros serão persistidos em PostgreSQL/outbox.
- Não tratar a consulta de disponibilidade como reserva. A proteção final contra sobreposição será feita pelo banco dentro de transação.

## Próxima leitura de código

Depois deste guia, a sequência mais útil é:

1. Ler [config.go](../internal/config/config.go) junto com [config_test.go](../internal/config/config_test.go), para ver a injeção de `getenv` em testes.
2. Ler [server.go](../internal/httpapi/server.go) junto com [server_test.go](../internal/httpapi/server_test.go), para ver `httptest`, handlers e graceful shutdown.
3. Ler [worker.go](../internal/worker/worker.go) junto com [worker_test.go](../internal/worker/worker_test.go), para ver canais e cancelamento sem RabbitMQ real.
4. Ler [architecture.md](architecture.md) e os `doc.go` de `booking`, `customers` e `notifications` antes de implementar `CreateAppointment`.

Esse próximo caso de uso será o primeiro que conectará as três camadas de um módulo: regras em `domain`, coordenação em `application` e transação/HTTP em `infra`.
