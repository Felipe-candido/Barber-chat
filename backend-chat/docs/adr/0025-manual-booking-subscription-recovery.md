# 0025 — Assinatura dentro do agendamento manual

Data: 2026-10-30. Estado: implementado em TEST.

## Lacuna encontrada

O painel criava pelo endpoint administrativo, mas consultava profissionais e
horários públicos. Isso impedia o uso de uma unidade paga porém não publicada,
além de impedir preencher o rascunho antes de contratar a assinatura.

## Observação administrativa

GET /admin/shops/{slug}/booking/professionals e /booking/availability reutilizam
os casos de uso de disponibilidade com autoridade explícita fornecida pelo
adaptador autenticado. A unidade resolvida deve coincidir com o escopo, e a
transação revalida o vínculo atual. Barbeiro só recebe seu profissional e não
pode consultar outra agenda por query string. Locks de calendário precedem o
guard de staff quando necessários, seguindo a ordem da criação/desativação.

Esses GETs não precisam de assinatura/publicação e não gravam contato/reserva.
Criação mantém os guards comerciais, idempotência e exclusão de sobreposição.
Endpoints públicos continuam com os guards anteriores. Sem SQL ou migration nova.

## Interface e pagamento

A criação recusada por assinatura mantém o formulário no modal. Owner pode
consultar a oferta e confirmar Checkout com intent=manual_booking. A apresentação
do plano agora é compartilhada em components/billing e pode ser embutida no
modal existente, sem empilhar diálogos. Configurações preserva a apresentação
anterior. O pagamento abre em outra aba; dados do cliente não vão à Stripe.

Conferir assinatura apenas consulta a API; Retomar revisão reconsulta os slots.
Se o horário mudou, o usuário escolhe outro mantendo nome/telefone. Nenhum POST
de reserva ou de cobrança é repetido automaticamente após o pagamento. Contratos
correntes não oferecem nova assinatura; configurações abre em outra aba.

O barbeiro não consulta billing. Após o owner regularizar, pode atualizar horário
e revisar; a confirmação continua revalidando a cobertura no backend.

## Incerteza e descarte

Se uma reserva anterior teve resposta incerta, até uma rejeição comercial posterior
preserva sua chave/corpo. A retomada recupera primeiro o comprovante, pois a própria
reserva salva pode ocupar o horário e não aparecer na disponibilidade. Fechamento
fica bloqueado enquanto houver envio incerto ou Checkout em andamento. Rascunho
descartável exige confirmação antes de fechar o modal.

O rascunho é apenas memória da aba: recarga forçada perde os dados, com aviso de
beforeunload. Não armazenar nome/telefone, segredo de tentativa ou URL de Checkout
em armazenamento persistente. Trocar unidade/identidade desmonta o fluxo.

## Verificação

Testes PostgreSQL para leitura sem assinatura/publicação, isolamento e escopo de
barbeiro; testes de navegador para pagamento seguido de revisão, slot ocupado,
contrato pendente, resposta perdida, recuperação idempotente e celular.
