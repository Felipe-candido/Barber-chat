# Deploy na EC2 (API, WhatsApp e lembretes)

Roteiro do que está em produção: a EC2 roda a API atrás do Caddy e, quando
habilitados, o Evolution (WhatsApp), o worker e o RabbitMQ. O site fica na
Netlify e o banco no Supabase. Decisões e limites: `backend-chat/docs/adr/0049-staged-ec2-containers.md`.

## Servidor

- Amazon Linux 2023, **4 GB de memória ou mais**. Com 2 GB o build do Evolution trava a máquina.
- Disco de 30 GB gp3, Elastic IP associado, portas 80 e 443 abertas, SSH só do seu IP.
- Role da instância com `AmazonSSMManagedInstanceCore` (acesso pelo Session Manager).
- Uma vez, como `ec2-user`:

```bash
sudo dnf install -y docker git
sudo systemctl enable --now docker && sudo usermod -aG docker ec2-user
sudo mkdir -p /usr/local/lib/docker/cli-plugins
for p in compose buildx; do
  v=$(curl -s https://api.github.com/repos/docker/$p/releases/latest | grep -m1 '"tag_name"' | cut -d'"' -f4)
  [ "$p" = compose ] && f="docker-compose-linux-x86_64" || f="buildx-$v.linux-amd64"
  sudo curl -sSL "https://github.com/docker/$p/releases/download/$v/$f" -o /usr/local/lib/docker/cli-plugins/docker-$p
  sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-$p
done
sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile swap swap defaults 0 0' | sudo tee -a /etc/fstab
git clone -b codex-appointments https://github.com/Felipe-candido/Barber-chat.git ~/Barber-chat
```

Saia e entre de novo na sessão para o grupo `docker` valer.

## Configuração

```bash
cd ~/Barber-chat/infra/aws
bash init-env.sh        # cria .env com segredos novos; nunca sobrescreve
nano .env            # domínio, site, Supabase e Stripe
bash deploy.sh check    # valida sem mostrar valores
```

Erros que já aconteceram e o `check` agora acusa:

| Sintoma | Causa |
| --- | --- |
| goose tenta `127.0.0.1:5432` | `DATABASE_URL` com `\` na frente, aspas ou `$` sem duplicar |
| conexão com o banco falha | host direto do Supabase (só IPv6); use o **pooler** com `sslmode=require` |
| painel com erro de CORS | `FRONTEND_ORIGIN` diferente do endereço exato do site |
| webhook do Stripe responde 404 | `STRIPE_WEBHOOK_SECRET` vazio, ou URL do destino sem `/api/v1/webhooks/stripe` |
| API não inicia por modo Stripe | `STRIPE_MODE` não combina com a chave, ou banco de outro ambiente |

Sem domínio próprio, `API_DOMAIN` pode ser `IP-COM-TRACOS.sslip.io` (ex.: `3-22-72-127.sslip.io`).

## Operação

Use sempre o `deploy.sh`: ele escolhe os arquivos Compose e o perfil pelo `.env`.
Um `docker compose up -d` sem `compose.notifications.yaml` desliga o WhatsApp da API.

```bash
bash deploy.sh up                 # sobe ou atualiza o que está habilitado no .env
bash deploy.sh update             # git pull, build, confere migrations e sobe
bash deploy.sh status
bash deploy.sh logs api           # TAIL=300 bash deploy.sh logs worker
bash deploy.sh migrate status     # bash deploy.sh migrate up aplica (faça backup antes)
```

Builds longos: rode com `nohup bash deploy.sh update > ~/deploy.log 2>&1 &` e acompanhe
com `tail -f ~/deploy.log`; a sessão do Session Manager cai por inatividade.

## Etapas

1. **API**: `.env` com Supabase e Stripe, `bash deploy.sh up`. No Stripe, um destino de
   eventos (snapshot, versão `2026-09-30.endive` ou `2026-10-28.endive`) para
   `https://<API_DOMAIN>/api/v1/webhooks/stripe`; o `whsec_` vai em `STRIPE_WEBHOOK_SECRET`.
   Na Netlify, `NEXT_PUBLIC_API_BASE_URL=https://<API_DOMAIN>` e novo deploy.
2. **WhatsApp**: copie do ambiente anterior `NOTIFICATION_PAYLOAD_ACTIVE_KEY_ID` e
   `NOTIFICATION_PAYLOAD_KEYS` (não gere novos), ligue `NOTIFICATION_PLANNING_ENABLED`
   e `NOTIFICATION_DELIVERY_ENABLED`, `bash deploy.sh up`. Ative a licença do Evolution
   pelo túnel `ssh -i chave.pem -L 8081:127.0.0.1:8081 ec2-user@<IP>` em
   `http://127.0.0.1:8081/manager/login` (a chave é `EVOLUTION_API_KEY`). Depois conecte
   o número em Configurações → WhatsApp no painel. Não crie instâncias pelo manager.
3. **Lembrete automático**: `NOTIFICATION_AUTOMATIC_ENABLED=true` e `bash deploy.sh up`
   (sobe worker e RabbitMQ).

## Backups

- Banco: backup do Supabase antes de qualquer `migrate up`.
- Sessão do WhatsApp e licença ficam nos volumes Docker: snapshot periódico do
  volume EBS (EC2 → Lifecycle Manager). Nunca use `docker compose down -v`.
