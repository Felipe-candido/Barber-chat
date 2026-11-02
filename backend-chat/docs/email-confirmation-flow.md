# Cadastro e confirmação de e-mail

Atualizado em 2026-11-02. Reenvio e recuperação de links implementados no frontend,
usando o Supabase Auth existente. Nenhuma migration ou alteração do backend Go.

## Para recuperar um link expirado

1. Abra `/confirmar-email` no frontend, ou use Reenviar confirmação de e-mail
   na tela de login. Informe o mesmo e-mail usado para criar a conta.
2. Solicite outro link e confira entrada/spam. A resposta não revela se uma
   conta existe nem garante entrega na caixa de entrada.
3. Abra o e-mail mais recente no navegador e endereço em que pediu o reenvio.
   O SDK cria um novo desafio PKCE e guarda seu verificador nesse navegador.
4. Após confirmar, o retorno fixo `/cadastro/barbearia` restaura a sessão e
   permite preencher o cadastro da unidade. Se já confirmou em outro navegador,
   entre com e-mail/senha pelo login e continue com a conta existente.

Não recriar conta para renovar o link. Reenvio usa `auth.resend({type:'signup'})`,
sem senha, sem service_role e sem criar usuário local, barbearia ou assinatura.

## Configuração do projeto Supabase

No projeto, em Authentication > URL Configuration:

- Site URL: endereço principal do frontend, por exemplo `http://127.0.0.1:3000`.
- Redirect URLs: permitir `http://127.0.0.1:3000/cadastro/barbearia`.
- Se também usar localhost, permitir `http://localhost:3000/cadastro/barbearia`.
- Na publicação, cadastrar a origem HTTPS real com o mesmo caminho fixo.

O redirect é calculado pela origem atual do navegador, sem aceitar destino
externo fornecido por query string. Não alternar localhost/127.0.0.1 durante
a mesma confirmação, pois o verificador está no armazenamento daquela origem.

Manter confirmação de e-mail habilitada no provedor Email. Com o fluxo padrão
implementado, o botão do template Confirm signup usa `{{ .ConfirmationURL }}`;
um link que apenas abra a home do app não executa a confirmação no Supabase.
Templates próprios com token_hash exigem outro handler; não foram introduzidos.

Se houver bloqueio/limite de envio, confira os logs Auth e a configuração SMTP.
A espera de 60s na interface não substitui os limites impostos pelo Supabase.
O serviço SMTP padrão tem restrições; produção precisa de configuração de envio
adequada ao projeto. Esta entrega não altera configurações remotas nem envia
e-mails reais por ferramentas de teste.

## Comportamento da aplicação

- Após signup sem sessão, mostrar e-mail de destino e formulário de reenvio
  preenchido, inicialmente aguardando 60s.
- Página de reenvio também atende quem fechou a aba de cadastro.
- Bloquear envio duplicado enquanto pendente e aplicar espera local entre tentativas.
- Falha de rede não dispara novo envio automático. Preservar e-mail e orientar
  conferir a caixa de entrada antes de tentar novamente.
- Login com email_not_confirmed mostra instrução de confirmação, não senha incorreta.
- Link expirado/inválido mostra mensagem local e acesso ao reenvio/login;
  nunca renderizar error_description arbitrário vindo da URL.
- Remover parâmetros/fragmentos do callback depois da inicialização do SDK.
- Um callback com falha não pode usar silenciosamente a conta anteriormente
  conectada, inclusive depois de Verificar sessão novamente.
- Login posterior explícito pode recuperar o fluxo mesmo com erro de callback
  anteriormente armazenado na inicialização do SDK.
- A confirmação continua exigida no formulário da unidade e validada pela API;
  o browser não concede acesso de proprietário por metadata.

Fluxo: cadastro Supabase → confirmação → sessão → cadastro explícito da barbearia
e membership owner → painel. Em acessos seguintes: login Supabase → token →
validação `/me` e vínculos na API Go → escolha da unidade → painel autorizado.
O pagamento para publicar/agendar permanece com as regras comerciais existentes.

## Validação

Testes de navegador em `tests/email-confirmation.spec.ts`, `tests/onboarding.spec.ts`
e `tests/login.spec.ts`: novo PKCE no reenvio, troca única de código, destinos
fixos, conta não confirmada, expiração em query/fragmento, sessão anterior,
limite de envio, resposta perdida, conta desconhecida e ausência de senhas na API.
Supabase é simulado nesses testes; recebimento real depende do projeto/SMTP.

Fontes oficiais: [reenvio](https://supabase.com/docs/reference/javascript/auth-resend),
[redirects](https://supabase.com/docs/guides/auth/redirect-urls),
[erros Auth](https://supabase.com/docs/guides/auth/debugging/error-codes),
[templates](https://supabase.com/docs/guides/auth/auth-email-templates),
[SMTP](https://supabase.com/docs/guides/auth/auth-smtp).
