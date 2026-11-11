# Ambiente Stripe explícito, proxies confiáveis e CSP por requisição

Status: implementado. Data: 04/11/2026.

## Ambientes e pagamentos

APP_ENV aceita development, test e production; o padrão é development.
STRIPE_MODE aceita test e live; o padrão continua test. Live exige APP_ENV=production,
FRONTEND_ORIGIN HTTPS, chave sk_live_, preço e segredo de webhook configurados.
O código não ativa cobranças reais nem troca credenciais locais automaticamente.

O modo selecionado é verificado em Customer, preço, Checkout Session, configuração
e sessão de Portal, assinaturas, faturas, pagamentos e eventos assinados. A URL da
API Stripe permanece fixa, os redirects são recusados e o HMAC usa o corpo original.
O frontend apresenta o ambiente informado pela API, sem presumir teste durante
carregamento. A confirmação da oferta inclui o ambiente e rejeita mudança de modo.

A migration 31 associa cada banco a um único ambiente Stripe. Bancos com registros
anteriores ficam vinculados a test. Contas/recibos não podem misturar ambientes;
trocar a configuração para live não converte cobertura de teste em acesso pago.
Produção usa banco separado, com suas migrations e recursos Stripe reais. Não
remover essa proteção para reutilizar faturas simuladas. A API confere a associação
antes de servir; a migration deve ser aplicada antes da nova versão. O downgrade
recusa remover suporte após vinculação a live. Contas legadas unknown continuam
sujeitas à revisão, sem reaproveitamento silencioso.

## Rede e abuso

TRUSTED_PROXY_CIDRS contém apenas redes/IPs dos proxies reais. Vazio não confia em
cabeçalhos encaminhados. Redes universais são recusadas. O endereço vem da conexão
TCP; somente um peer confiável autoriza a leitura de X-Forwarded-For. A cadeia é
percorrida da direita para a esquerda até o primeiro endereço não confiável.
Cabeçalhos possuem limites de tamanho/quantidade. RemoteAddr é preservado.

O limitador público de booking usa esse endereço validado. Onboarding recebe quota
por IP; Checkout/Portal e mensagens/conexão também recebem quota por ator autenticado,
compartilhada entre unidades. Nenhuma quota usa nome, telefone, token ou tenant
fornecido no corpo. Mapas são limitados e expiram. Retornar 429/Retry-After sem retry
automático de escritas. Webhooks autenticados não são submetidos à quota de usuários.
Os limites são por processo: o alvo inicial é uma API na EC2. Escala horizontal exige
quota de borda/distribuída; não apresentar o contador em memória como limite global.

Em APP_ENV=production, a API exige TLS ou HTTPS declarado por um proxy confiável,
sem redirecionar corpos/credenciais de POST. Health/readiness permitem sondas privadas.
O listener privado de callback Evolution não deve ser publicado no host. Caddy
encerra TLS e redireciona HTTP; o template fica em infra/aws/Caddyfile.

## Frontend

Proxy gera nonce aleatório por requisição e sobrescreve x-nonce/CSP recebidos do
cliente. O layout aguarda connection(), impedindo reutilizar nonce de HTML estático.
HTML recebe no-store. Scripts usam nonce e strict-dynamic, sem unsafe-inline nem
unsafe-eval em produção. Origens de conexão são a API e o Supabase configurados.
Fontes são locais; imagens data/blob permitem o QR. Frames e objects são bloqueados.
style-src-attr permite os estilos de layout existentes; blocos de estilo continuam
restritos. Essa exceção não libera scripts inline. HTTPS público recebe HSTS e
upgrade-insecure-requests; o desenvolvimento em loopback continua funcionando.

Adicionar nosniff, DENY, no-referrer e Permissions-Policy. Portal/Checkout hospedados
abrem por navegação, sem iframe ou inclusão de scripts externos de cobrança.

## Segredos e publicação

O .env da raiz já havia sido removido do índice pelo usuário. O teste de arquitetura
impede reintroduzir arquivos .env rastreados; não reescrever histórico automaticamente.
A senha exposta deve ser rotacionada no Supabase e atualizada na Render/configuração
local; essa etapa foi explicitamente adiada pelo usuário. Não registrar os valores.
Nenhuma atualização de código invalida sozinha um segredo presente no histórico.

Go mínimo 1.27.3; Next.js 16.4.5 e transitivas atualizadas. Recompilar e publicar API,
worker, ferramentas e frontend para substituir artefatos vulneráveis já hospedados.
