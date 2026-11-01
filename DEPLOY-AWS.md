# Deploy do Barber-chat na AWS (EC2 + S3)

Guia para colocar o Barber-chat no ar em **uma única instância EC2** com Docker Compose e, depois, adicionar um **bucket S3 com CloudFront** para imagens. É o caminho mais simples e barato para um MVP; no fim há as próximas evoluções quando o volume crescer.

> Valores entre `<>` são seus: troque `<seu-dominio>` por algo como `barberchat.com.br`.

---

## Visão geral

```text
Navegador ──HTTPS──► Caddy (EC2 :443) ──► web  (Next.js :3000)
                                       └─► api  (Go :8080) ──► postgres
                                                  │          └─► rabbitmq
                                       worker (Go) ┘
Imagens:  navegador ──PUT (URL pré-assinada)──► S3 ◄── CloudFront ──► navegador
```

- `app.<seu-dominio>` → frontend; `api.<seu-dominio>` → API.
- Caddy emite e renova os certificados HTTPS (Let's Encrypt) sozinho.
- Postgres e RabbitMQ ficam **só na rede interna do Docker**, sem porta pública.
- A autenticação continua no Supabase; só mudam as URLs permitidas.

### Custos (confira sempre no console)

- **Conta nova (desde 15/07/2025):** a AWS dá até US$ 200 em créditos, num plano gratuito que termina em 6 meses ou quando os créditos acabam.
- **t4g.small:** está em teste gratuito de 750 h/mês até 31/12/2026.
- **IPv4 público:** custa US$ 0,005 por hora (cerca de US$ 3,60/mês), mesmo quando é um Elastic IP associado a uma instância.
- **Disco (EBS gp3), S3 e CloudFront:** cobrados por uso. Para um MVP, são poucos dólares por mês.

**Antes de tudo:** em *Billing → Budgets*, crie um orçamento mensal (ex.: US$ 15) com alerta por e-mail.

---

## Parte 1 — Preparar o código para produção

São três arquivos novos e um ajuste no Next. Faça numa branch e teste localmente com `docker compose` antes de ir para o servidor.

### 1.1 Next em modo `standalone`

`frontend-chat/next.config.ts`:

```ts
import type { NextConfig } from "next";
const config: NextConfig = {
  poweredByHeader: false,
  output: "standalone",
  distDir: process.env.BARBER_PLAYWRIGHT === "1" ? ".next-playwright" : ".next",
};
export default config;
```

### 1.2 `frontend-chat/Dockerfile`

As variáveis `NEXT_PUBLIC_*` são gravadas no JavaScript **na hora do build**. Por isso, entram como `ARG`.

```dockerfile
FROM node:24-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:24-alpine AS build
WORKDIR /app
ARG NEXT_PUBLIC_API_BASE_URL
ARG NEXT_PUBLIC_SUPABASE_URL
ARG NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:24-alpine AS run
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
CMD ["node", "server.js"]
```

O `npm run build` usa `next build --webpack`, como no `package.json`.

### 1.3 `backend-chat/Dockerfile`

Uma imagem gera os três binários: API, worker e o Goose para as migrations.

```dockerfile
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go install github.com/pressly/goose/v3/cmd/goose@v3.28.0 \
 && cp /go/bin/goose /out/goose

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ /app/
COPY db/migrations /app/migrations
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
```

### 1.4 `deploy/compose.prod.yaml` (na raiz do repositório)

```yaml
name: barber-chat-prod

services:
  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports: ["80:80", "443:443"]
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on: [web, api]

  web:
    build:
      context: ../frontend-chat
      args:
        NEXT_PUBLIC_API_BASE_URL: https://api.${DOMAIN}
        NEXT_PUBLIC_SUPABASE_URL: ${SUPABASE_URL}
        NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY: ${SUPABASE_PUBLISHABLE_KEY}
    restart: unless-stopped

  api:
    build: ../backend-chat
    restart: unless-stopped
    env_file: .env.prod
    environment:
      HTTP_ADDR: 0.0.0.0:8080
      FRONTEND_ORIGIN: https://app.${DOMAIN}
    depends_on:
      postgres: { condition: service_healthy }
      rabbitmq: { condition: service_healthy }

  worker:
    build: ../backend-chat
    entrypoint: ["/app/worker"]
    restart: unless-stopped
    env_file: .env.prod
    depends_on:
      postgres: { condition: service_healthy }
      rabbitmq: { condition: service_healthy }

  migrate:
    build: ../backend-chat
    entrypoint: ["/app/goose", "-dir", "/app/migrations", "postgres"]
    command: ["${DATABASE_URL}", "up"]
    profiles: ["tools"]
    depends_on:
      postgres: { condition: service_healthy }

  postgres:
    image: postgres:18.6-bookworm
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes: [postgres_data:/var/lib/postgresql]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \"$$POSTGRES_USER\" -d \"$$POSTGRES_DB\""]
      interval: 5s
      retries: 10

  rabbitmq:
    image: rabbitmq:4.3.5-management
    restart: unless-stopped
    environment:
      RABBITMQ_DEFAULT_USER: ${RABBITMQ_DEFAULT_USER}
      RABBITMQ_DEFAULT_PASS: ${RABBITMQ_DEFAULT_PASS}
    volumes: [rabbitmq_data:/var/lib/rabbitmq]
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 10s
      retries: 12

volumes:
  caddy_data:
  caddy_config:
  postgres_data:
  rabbitmq_data:
```

Repare que Postgres e RabbitMQ **não têm `ports:`**. Só o Caddy fica exposto.

### 1.5 `deploy/Caddyfile`

```caddyfile
app.{$DOMAIN} {
	encode zstd gzip
	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
		X-Content-Type-Options "nosniff"
		Referrer-Policy "strict-origin-when-cross-origin"
		X-Frame-Options "DENY"
	}
	reverse_proxy web:3000
}

api.{$DOMAIN} {
	encode zstd gzip
	header X-Content-Type-Options "nosniff"
	reverse_proxy api:8080
}
```

Adicione `environment: { DOMAIN: ${DOMAIN} }` ao serviço `caddy` no compose para o Caddy ler o domínio.

### 1.6 `deploy/.env.prod.example` (versione só o exemplo)

```dotenv
DOMAIN=<seu-dominio>

POSTGRES_USER=barber
POSTGRES_PASSWORD=<gere com: openssl rand -base64 32>
POSTGRES_DB=barber
RABBITMQ_DEFAULT_USER=barber
RABBITMQ_DEFAULT_PASS=<outra senha forte>

DATABASE_URL=postgres://barber:<senha>@postgres:5432/barber?sslmode=disable
RABBITMQ_URL=amqp://barber:<senha>@rabbitmq:5672/

SUPABASE_URL=https://<projeto>.supabase.co
SUPABASE_PUBLISHABLE_KEY=<publishable key>
SUPABASE_SERVICE_ROLE_KEY=<só se usar convites de equipe>

STRIPE_SECRET_KEY=
STRIPE_MONTHLY_PRICE_ID=
STRIPE_WEBHOOK_SECRET=
STRIPE_PORTAL_CONFIGURATION_ID=

LOG_LEVEL=INFO
```

O `sslmode=disable` só é aceitável porque o Postgres está na rede interna do Docker, na mesma máquina. Se migrar para RDS ou para o Postgres do Supabase, use `sslmode=require`.

Adicione `deploy/.env.prod` ao `.gitignore`.

---

## Parte 2 — Criar a instância EC2

### 2.1 Região e instância

1. Entre no console AWS e escolha a região **São Paulo (sa-east-1)**, para ter menor latência com os clientes no Brasil.
2. Vá em **EC2 → Launch instance**:
   - **Name:** `barber-chat-prod`
   - **AMI:** Ubuntu Server 24.04 LTS, arquitetura **64-bit (Arm)**
   - **Instance type:** `t4g.small` (2 vCPU, 2 GB de RAM)
   - **Key pair:** crie uma chave nova (`.pem`) e guarde-a bem. Ela é o único jeito de entrar por SSH.
   - **Network settings → Edit:**
     - SSH (22): origem **My IP**. Nunca deixe `0.0.0.0/0`.
     - HTTP (80) e HTTPS (443): origem `0.0.0.0/0` (Anywhere).
   - **Storage:** 30 GiB **gp3**
   - **Advanced details → Metadata version:** *V2 only* e **Metadata response hop limit = 2**. O valor 2 é necessário para que os containers Docker consigam usar a role IAM da Parte 5.
3. Clique em **Launch instance**.

### 2.2 IP fixo e DNS

1. Em **EC2 → Elastic IPs → Allocate**, associe o IP à instância. Assim o IP não muda quando a máquina reinicia.
2. No seu provedor de domínio (Registro.br, Cloudflare etc.), crie:
   - `A  app.<seu-dominio>  → <elastic-ip>`
   - `A  api.<seu-dominio>  → <elastic-ip>`

   Se usar Cloudflare, deixe a nuvem **cinza** (DNS only) na primeira vez, para o Caddy conseguir emitir o certificado.

### 2.3 Primeiro acesso e preparação

No PowerShell (Windows 10 e 11 já têm `ssh`):

```powershell
ssh -i C:\caminho\barber-chat.pem ubuntu@<elastic-ip>
```

No servidor:

```bash
# Atualizações e firewall básico
sudo apt update && sudo apt -y upgrade
sudo apt -y install unattended-upgrades git
sudo dpkg-reconfigure -plow unattended-upgrades

# 2 GB de swap: o build do Next pode passar de 2 GB de RAM
sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile
sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab

# Docker (repositório oficial)
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker ubuntu
exit   # saia e entre de novo para o grupo docker valer
```

### 2.4 Subir a aplicação

```bash
git clone https://github.com/Felipe-candido/Barber-chat.git
cd Barber-chat/deploy
cp .env.prod.example .env.prod
nano .env.prod          # preencha as senhas e chaves reais
ln -s .env.prod .env    # o compose lê .env para as variáveis ${...}

docker compose -f compose.prod.yaml build
docker compose -f compose.prod.yaml up -d postgres rabbitmq
docker compose -f compose.prod.yaml --profile tools run --rm migrate
docker compose -f compose.prod.yaml up -d
docker compose -f compose.prod.yaml ps
```

Se o repositório for privado, crie uma *deploy key* (somente leitura) no GitHub, ou use um token, para o `git clone`.

Teste:

- `https://api.<seu-dominio>/health` deve responder ok
- `https://api.<seu-dominio>/ready` deve responder ready (banco e fila conectados)
- `https://app.<seu-dominio>` deve abrir o login

### 2.5 Ajustes externos

- **Supabase → Authentication → URL Configuration:**
  - Site URL: `https://app.<seu-dominio>`
  - Redirect URLs: `https://app.<seu-dominio>/**`, que inclui `/definir-senha`.
- **Stripe:** crie o endpoint de webhook apontando para a rota de webhook da API em `https://api.<seu-dominio>`, e use o novo *signing secret* em `STRIPE_WEBHOOK_SECRET`.

### 2.6 Atualizar a aplicação (deploy de uma nova versão)

```bash
cd ~/Barber-chat && git pull
cd deploy
docker compose -f compose.prod.yaml build
docker compose -f compose.prod.yaml --profile tools run --rm migrate
docker compose -f compose.prod.yaml up -d
docker image prune -f
```

Comandos úteis:

- `docker compose -f compose.prod.yaml logs -f api` mostra os logs da API em tempo real.
- `docker compose -f compose.prod.yaml restart worker` reinicia só o worker.

---

## Parte 3 — Backups do Postgres

O banco está dentro da instância, então **sem backup, perder o disco é perder tudo**. Faça duas camadas:

1. **Snapshot do disco:** em **EC2 → Lifecycle Manager**, crie uma política diária para o volume da instância, guardando 7 snapshots.
2. **Dump lógico para o S3:** depois da Parte 4, em `crontab -e`:

```cron
15 3 * * * docker exec barber-chat-prod-postgres-1 pg_dump -U barber -Fc barber | aws s3 cp - s3://<bucket>/backups/barber-$(date +\%F).dump
```

Para isso, instale a AWS CLI (`sudo snap install aws-cli --classic`) e dê `s3:PutObject` em `backups/*` à role da Parte 5. Crie também uma regra de *lifecycle* no bucket para apagar os `backups/` depois de 30 dias.

**Teste a restauração** pelo menos uma vez, com `pg_restore` num Postgres local.

---

## Parte 4 — Bucket S3 para imagens

### 4.1 Como vai funcionar

1. O painel pede à API uma **URL pré-assinada** (válida por alguns minutos) para enviar uma imagem.
2. O navegador faz `PUT` da imagem **direto no S3**, sem passar pela EC2.
3. O frontend avisa a API, que grava a **chave** do arquivo no banco (ex.: `shops/<id>/logo-<uuid>.webp`).
4. As imagens são exibidas pelo **CloudFront** (`https://cdn.<seu-dominio>/<chave>`), com cache e HTTPS. O bucket continua **privado**.

### 4.2 Criar o bucket

1. Vá em **S3 → Create bucket**:
   - **Nome:** `barber-chat-media-<algo-único>`. O nome é global na AWS.
   - **Região:** a mesma da EC2.
   - **Object Ownership:** *ACLs disabled*, que já é o padrão.
   - **Block all public access:** **mantenha ligado**. Desde 2023, isso também já vem ativado por padrão em buckets novos.
   - **Default encryption:** SSE-S3.
2. Em **Permissions → CORS**, libere o upload a partir do painel:

```json
[
  {
    "AllowedOrigins": ["https://app.<seu-dominio>"],
    "AllowedMethods": ["PUT"],
    "AllowedHeaders": ["content-type", "content-length"],
    "MaxAgeSeconds": 3000
  }
]
```

Durante o desenvolvimento, adicione também `http://127.0.0.1:3000`.

### 4.3 CloudFront na frente do bucket

1. Vá em **CloudFront → Create distribution**:
   - **Origin:** o bucket. Escolha **Origin access control (OAC)** e deixe o console atualizar a *bucket policy*.
   - **Viewer protocol policy:** *Redirect HTTP to HTTPS*.
   - **Cache policy:** *CachingOptimized*.
   - **Alternate domain name:** `cdn.<seu-dominio>`. O certificado precisa ser pedido no **ACM, na região us-east-1**: é uma exigência do CloudFront.
2. No DNS, crie `CNAME cdn.<seu-dominio> → dxxxx.cloudfront.net`.

A bucket policy gerada libera leitura **apenas** para essa distribuição, por meio de uma condição `AWS:SourceArn`. O bucket continua sem acesso público.

---

## Parte 5 — Permissão da EC2 para o S3 (sem chaves no `.env`)

Não crie *access keys* de usuário IAM. Use uma **role** na instância.

1. Em **IAM → Policies → Create policy (JSON)**, crie a política:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": [
        "arn:aws:s3:::<bucket>/shops/*",
        "arn:aws:s3:::<bucket>/products/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject"],
      "Resource": "arn:aws:s3:::<bucket>/backups/*"
    }
  ]
}
```

2. Em **IAM → Roles → Create role**, escolha o caso de uso **EC2** e anexe a política. Nome: `barber-chat-ec2`.
3. Em **EC2 → instância → Actions → Security → Modify IAM role**, escolha `barber-chat-ec2`.

O SDK de Go encontra as credenciais sozinho, pelo serviço de metadados da instância. Por isso o *hop limit* 2 do passo 2.1 importa: sem ele, os containers não alcançam o serviço de metadados.

Adicione ao `.env.prod`:

```dotenv
AWS_REGION=sa-east-1
MEDIA_BUCKET=<bucket>
MEDIA_PUBLIC_BASE_URL=https://cdn.<seu-dominio>
```

---

## Parte 6 — Código: upload com URL pré-assinada

### 6.1 Dependências (backend)

```powershell
cd backend-chat
go get github.com/aws/aws-sdk-go-v2/config github.com/aws/aws-sdk-go-v2/service/s3
```

Isso segue a regra do `AGENTS.md` de dependência nova com versão fixada: o `go.sum` registra a versão.

### 6.2 Adaptador (ex.: `internal/platform/media/s3.go`)

```go
package media

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

var allowedTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

const maxImageBytes = 2 << 20 // 2 MiB

type Store struct {
	bucket  string
	presign *s3.PresignClient
}

func NewStore(ctx context.Context, bucket string) (*Store, error) {
	cfg, err := config.LoadDefaultConfig(ctx) // usa a role da EC2 em produção
	if err != nil {
		return nil, err
	}
	return &Store{bucket: bucket, presign: s3.NewPresignClient(s3.NewFromConfig(cfg))}, nil
}

type Upload struct {
	Key     string            `json:"key"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// PresignImage returns a short-lived PUT URL. Content-Type and Content-Length are signed,
// so S3 rejects a different file type or size.
func (s *Store) PresignImage(ctx context.Context, prefix, contentType string, size int64) (Upload, error) {
	ext, ok := allowedTypes[contentType]
	if !ok || size <= 0 || size > maxImageBytes {
		return Upload{}, fmt.Errorf("unsupported image")
	}
	key := fmt.Sprintf("%s/%s.%s", prefix, uuid.NewString(), ext)
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
		CacheControl:  aws.String("public, max-age=31536000, immutable"),
	}, s3.WithPresignExpires(5*time.Minute))
	if err != nil {
		return Upload{}, err
	}
	headers := map[string]string{}
	for name, values := range req.SignedHeader {
		if name != "Host" && len(values) > 0 {
			headers[name] = values[0]
		}
	}
	return Upload{Key: key, URL: req.URL, Headers: headers}, nil
}
```

### 6.3 Fluxo na API

1. `POST /api/v1/admin/shops/{slug}/media/uploads`, com o corpo `{ "kind": "product", "contentType": "image/webp", "size": 183442 }`:
   - autentique e autorize pela *membership*, como nos outros endpoints de admin;
   - monte o prefixo **a partir do tenant resolvido**, por exemplo `products/<shop_id>`, nunca a partir de um ID enviado pelo cliente;
   - responda com `{ key, url, headers }`.
2. O frontend envia o arquivo direto ao S3:

```ts
await fetch(upload.url, { method: "PUT", headers: upload.headers, body: file });
```

3. Por fim, o frontend chama o endpoint de salvar (ex.: `PATCH` do produto) com `image_key`. A API confere se a chave começa com o prefixo daquele tenant antes de gravar.

### 6.4 Banco

Crie uma migration nova, sem editar as que já foram aplicadas:

```sql
-- +goose Up
ALTER TABLE products ADD COLUMN image_key text;
-- +goose Down
ALTER TABLE products DROP COLUMN image_key;
```

Guarde **a chave**, não a URL. Assim dá para trocar o domínio do CDN sem migrar dados.

### 6.5 Exibir no Next

Em `next.config.ts`:

```ts
images: { remotePatterns: [{ protocol: "https", hostname: "cdn.<seu-dominio>" }] },
```

E nos componentes: `<Image src={`${cdn}/${product.image_key}`} ... />`.

### 6.6 Limpeza

Envios que começam mas nunca são salvos deixam arquivos órfãos. Duas opções:

- enviar primeiro para `tmp/` e **copiar** para o prefixo final ao salvar, com uma regra de *lifecycle* que apaga `tmp/` depois de 1 dia;
- ou ter um job no worker que remove as chaves que não estão no banco.

---

## Parte 7 — Checklist antes de divulgar o link

- [ ] Orçamento com alerta de cobrança criado
- [ ] SSH liberado só para o seu IP (ou use **Session Manager** e feche a porta 22)
- [ ] `.env.prod` fora do git, com senhas fortes e chaves de produção (Stripe *live* só quando for cobrar de verdade)
- [ ] `/health` e `/ready` respondendo pelo HTTPS
- [ ] Migrations aplicadas com `migrate`
- [ ] Snapshots diários do disco **e** dump do banco no S3, com uma restauração testada
- [ ] URLs do Supabase e webhook do Stripe atualizados
- [ ] Bucket sem acesso público, servido só pelo CloudFront
- [ ] Monitoramento simples: um verificador externo de uptime em `/health` (ex.: UptimeRobot) e o alarme do CloudWatch *StatusCheckFailed* na instância

## Quando evoluir

| Sinal | Próximo passo |
| --- | --- |
| Medo de perder o banco ou falta de tempo para cuidar dele | Postgres gerenciado: **RDS** ou o Postgres do próprio Supabase (`sslmode=require`) |
| Build lento ou travando na EC2 | Gerar as imagens no **GitHub Actions** e publicar no ECR ou GHCR; o servidor só faz `pull` |
| Mais de uma instância | Load Balancer e serviço de containers (ECS), com o RabbitMQ gerenciado (Amazon MQ) |
| Muita imagem | Gerar miniaturas no upload (worker) e servir em WebP/AVIF |
