# Tarefa: mensagem de WhatsApp personalizável por barbearia

Pedido do dono do produto (15/11/2026). Escrito para o Codex implementar: exige
migration, regeneração do sqlc e testes Go.

## O que o dono da barbearia deve conseguir

Em Configurações → WhatsApp, editar dois textos opcionais para cada tipo de
mensagem (confirmação e lembrete):

- **Saudação**: substitui a frase depois de "Olá, {nome}!". Padrões atuais:
  "Seu horário está confirmado." e "Passando para lembrar do seu horário."
- **Recado final**: linha extra antes do código, por exemplo "Chegue 5 minutos
  antes" ou "Em caso de atraso, avise por aqui".

Com os campos vazios, a mensagem fica exatamente como o modelo padrão em
`notifications/application/message.go`.

## O que NÃO deve ser editável

O bloco de dados (barbearia, serviço, profissional, data, horário, valor,
endereço), o código do agendamento e o aviso de guardar o código. Motivos:

- o código é o único meio de o cliente consultar ou cancelar;
- o consentimento do cliente cobre mensagens sobre o agendamento, não marketing;
- texto totalmente livre em disparo automático aumenta o risco de bloqueio do
  número pelo WhatsApp.

## Regras propostas

- Até 300 caracteres por campo, uma linha (passar por `messageLine`), sem
  caracteres de controle. Recusar links (`http://`, `https://`, `www.`) na
  primeira versão.
- Sem variáveis no texto livre: o nome do cliente já vem no "Olá, {nome}!".
- Colunas novas em `shop_notification_settings` (quatro `VARCHAR(300) NOT NULL
  DEFAULT ''` com CHECK de tamanho), só o owner altera, mesma rota de
  preferências já usada pela tela.
- **Determinismo**: a mensagem enviada deve usar o texto vigente quando o
  agendamento foi criado. Gravar os dois textos no snapshot cifrado do job
  (`BookingSnapshot`), não ler a configuração na hora do envio. Assim um retry
  ou um lembrete agendado não muda de texto depois.
- Subir `BookingMessageTemplateVersion` para 2 e manter a versão 1 renderizando
  jobs antigos sem os campos novos. Ajustar a CHECK de `template_version` se
  houver.
- Tela: dois campos de texto por tipo, contador de caracteres e uma **prévia**
  da mensagem completa com dados de exemplo.

## Testes

Texto vazio = padrão; limite de tamanho; quebra de linha e caracteres bidi
neutralizados; link recusado; snapshot antigo (v1) continua renderizando; mudar
a configuração não altera mensagens de agendamentos já criados; barbeiro sem
papel de owner não altera.
