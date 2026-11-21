import { apiBaseURL } from "./config";

const SESSION_TIMEOUT_MS = 10_000;
// Allow the API's 30-second request deadline plus network/response overhead.
const REQUEST_TIMEOUT_MS = 40_000;

export class ApiError extends Error {
  constructor(
    public readonly code: string,
    message: string,
    public readonly status = 0,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
const messages: Record<string, string> = {
  invalid_time_off:
    "Confira as datas. Escolha dias de hoje em diante, até um ano por vez, dentro dos próximos dois anos.",
  time_off_has_appointments:
    "Já há atendimentos nesse período. Remaneje ou cancele esses agendamentos antes de bloquear os dias.",
  time_off_overlap:
    "Já existe uma folga nesses dias. Confira os períodos programados antes de adicionar outro.",
  time_off_conflict:
    "Este bloqueio já foi usado com outras datas. Atualize as folgas e confira o período antes de tentar novamente.",
  completion_settings_changed:
    "O modo de fechamento mudou. Consulte o modo atual e revise antes de salvar novamente.",
  invalid_completion_settings:
    "Escolha como os atendimentos serão concluídos e confira a configuração atual.",
  invalid_contact: "Confira o nome e o telefone completo com DDD. O sobrenome é opcional.",
  invalid_contact_query:
    "Confira a busca. Para telefone, informe o número completo com DDD; para reiniciar a lista, limpe a busca.",
  contact_exists: "Já existe um contato com esse telefone nesta barbearia.",
  contact_access_denied:
    "Seu acesso aos contatos não está disponível. Confira sua sessão e o vínculo com a barbearia.",
  contact_not_found: "Contato não encontrado nesta barbearia.",
  contact_unavailable:
    "Não foi possível confirmar a operação. Confira a lista antes de cadastrar novamente.",
  connection_busy:
    "Já existe uma operação de conexão em andamento. Confira o estado antes de tentar novamente.",
  activation_required: "Fale com o suporte para ativar o WhatsApp da sua barbearia.",
  provider_not_configured:
    "O WhatsApp ainda não está pronto para uso. Fale com o suporte para conectar.",
  settings_conflict:
    "As preferências de WhatsApp mudaram. Confira o estado e revise antes de salvar.",
  invalid_settings:
    "Confira como e quando os lembretes serão enviados. Se o envio automático estiver indisponível, fale com o suporte.",
  invalid_report_query:
    "Confira as datas e os filtros (até 366 dias). Atualize a consulta para reiniciar o histórico.",
  report_unavailable: "Não foi possível validar os dados do relatório. Atualize a consulta.",
  invalid_appointment_status: "Escolha uma situação válida e informe o motivo em uma única linha.",
  appointment_version_changed:
    "O atendimento mudou, possivelmente pela conclusão automática. Atualize os detalhes e revise a alteração.",
  appointment_status_unchanged:
    "O atendimento já está nessa situação. Atualize os detalhes antes de corrigir.",
  appointment_transition_too_early:
    "Ausência só pode ser registrada após o início. Marcar como realizado exige que o horário reservado tenha terminado.",
  appointment_interval_conflict:
    "Outro agendamento ocupa esse intervalo. A situação não foi alterada; confira a agenda.",
  appointment_command_pending:
    "A alteração ainda precisa ser confirmada. Recupere a tentativa original.",
  appointment_status_unavailable:
    "Não foi possível confirmar a alteração. Confira o atendimento e recupere a tentativa original.",
  invalid_product: "Revise o nome, a descrição e o preço positivo do produto.",
  invalid_stock_movement: "Informe quantidade inteira, motivo e movimentação válidos.",
  product_not_found: "Produto não encontrado nesta barbearia. Atualize o catálogo.",
  product_version_conflict:
    "O produto ou estoque mudou. Atualize a lista antes de editar novamente.",
  product_command_pending:
    "A movimentação ainda precisa ser confirmada. Recupere a tentativa original.",
  invalid_sale: "Revise os produtos, as quantidades e os dados da anulação.",
  sale_price_changed:
    "O preço ou cadastro de um produto mudou. Atualize os produtos e revise a venda antes de confirmar.",
  sale_product_unavailable:
    "Um produto não está mais disponível. Atualize o catálogo e revise a seleção.",
  insufficient_stock:
    "O estoque é insuficiente para esta operação. Nenhuma venda ou retirada parcial foi registrada.",
  sale_appointment_unavailable:
    "Este atendimento não pode receber a venda. Confira o profissional e a situação do agendamento.",
  sale_not_found: "Venda não encontrada no seu acesso a esta barbearia.",
  sale_version_changed: "A venda mudou. Consulte seus detalhes novamente antes de anular.",
  sale_already_cancelled:
    "A venda já foi anulada com outra decisão de estoque ou motivo. Atualize os detalhes.",
  stock_limit_exceeded: "A devolução ultrapassa o limite de estoque. A operação não foi salva.",
  sale_command_pending: "A confirmação desta venda está pendente. Recupere a tentativa original.",
  invalid_sales_query:
    "Confira as datas, os filtros e o período de até um ano. Atualize a lista para reiniciar a paginação.",
  sale_unavailable:
    "Não foi possível confirmar a operação. Confira as vendas e recupere a tentativa original se o resultado for incerto.",
  product_unavailable:
    "Não foi possível confirmar a operação de estoque. Confira o histórico e recupere a tentativa original.",
  billing_not_configured:
    "Não foi possível abrir a assinatura agora. Fale com o suporte para continuar.",
  billing_not_required:
    "Sua barbearia já possui acesso liberado. Fale com o suporte antes de contratar uma assinatura.",
  subscription_exists:
    "Já existe uma assinatura para esta barbearia. Confira a situação atual; não abra outra cobrança.",
  checkout_offer_changed:
    "As condições mudaram. Consulte e revise o plano novamente antes de continuar.",
  checkout_confirmation_pending:
    "Seu pedido de assinatura foi enviado. Estamos aguardando a confirmação do pagamento. Confira sua assinatura antes de tentar novamente.",
  checkout_requires_review:
    "Precisamos conferir seu pagamento. Fale com o suporte antes de iniciar outra assinatura.",
  checkout_expired:
    "O link de pagamento expirou. Confira o plano novamente para abrir outro pagamento.",
  checkout_unavailable:
    "Não conseguimos confirmar o link de pagamento. Confira o plano novamente para continuar.",
  portal_not_configured: "O portal de assinatura ainda não está disponível. Fale com o suporte.",
  portal_not_available:
    "A assinatura ainda não está disponível para gerenciamento. Atualize a situação; se o pagamento já foi feito, aguarde a confirmação.",
  portal_unavailable:
    "Não foi possível abrir o portal. Nenhuma alteração da assinatura foi solicitada por esta ação. Você pode preparar um novo acesso.",
  shop_not_found: "Barbearia não encontrada ou inativa. Volte à seleção de barbearias.",
  invalid_body: "Não foi possível enviar os dados. Confira os campos e tente novamente.",
  unsupported_media_type: "Não foi possível enviar os dados. Atualize a página e tente novamente.",
  body_too_large: "O texto está muito longo. Reduza a descrição e tente novamente.",
  invalid_service: "Revise o nome, a duração e o preço do serviço.",
  service_not_found: "Este serviço não está mais disponível. Atualize a lista para continuar.",
  invalid_professional: "Informe um nome de barbeiro com até 100 caracteres.",
  professional_not_found:
    "Este barbeiro não está disponível nesta barbearia. Atualize a equipe e tente novamente.",
  professional_changed:
    "O cadastro foi alterado desde que você abriu a edição. Confira os dados atuais antes de salvar novamente.",
  invalid_professional_edit:
    "Confira o nome e as opções do barbeiro. Recarregue o cadastro se os dados tiverem mudado.",
  invalid_professional_services: "Selecione serviços ativos e agendáveis desta barbearia.",
  invalid_professional_schedule:
    "Confira os dias, abertura, fechamento, intervalo e margem da agenda.",
  temporarily_unavailable:
    "Não conseguimos atender seu pedido agora. Tente novamente em instantes.",
  internal_error: "Não foi possível concluir. Tente novamente em instantes.",
  authentication_required: "Entre novamente para concluir esta operação.",
  invalid_access_token: "Seu acesso expirou. Entre novamente para continuar.",
  access_denied:
    "Acesso não liberado. Confira com o responsável o cadastro do seu usuário e o vínculo ativo com a barbearia.",
  origin_not_allowed:
    "Não foi possível acessar por este endereço. Abra o link oficial do Na Regua ou fale com o suporte.",
  invalid_shop_registration:
    "Confira seu nome, o nome da barbearia, o endereço do link e o fuso horário.",
  invalid_shop_address:
    "Informe o endereço completo: CEP, rua, número (ou S/N), bairro, cidade e estado.",
  invalid_shop_settings:
    "Confira o nome, o telefone brasileiro com DDD e a descrição de até 1000 caracteres.",
  shop_settings_conflict:
    "As configurações mudaram desde que você abriu esta tela. Confira os dados salvos antes de salvar novamente.",
  shop_slug_taken: "Esse endereço de link já está em uso. Escolha outro para sua barbearia.",
  email_confirmation_required: "Confirme seu e-mail antes de cadastrar a barbearia.",
  registration_not_configured:
    "O cadastro está indisponível no momento. Tente mais tarde ou fale com o suporte.",
  publication_blocked:
    "A barbearia ainda não pode ser publicada. Confira a situação atual e complete as pendências.",
  publication_not_managed: "Fale com o suporte para ativar ou pausar o link desta barbearia.",
  shop_subscription_required:
    "A assinatura precisa ser regularizada para criar novos agendamentos ou vendas. O proprietário gerencia a assinatura em Configurações.",
  booking_option_not_found:
    "Este serviço ou profissional não está mais disponível. Escolha novamente.",
  appointment_not_found:
    "Código não encontrado nesta barbearia. Confira o código e o link utilizado.",
  invalid_customer: "Confira seu nome, sobrenome e telefone com DDD ou código internacional.",
  invalid_availability: "Escolha um serviço, profissional e uma data dentro do período disponível.",
  invalid_agenda: "Confira o mês selecionado e atualize a agenda para reiniciar a paginação.",
  invalid_staff_account: "Confira o e-mail e selecione um barbeiro cadastrado e ativo.",
  staff_account_exists:
    "Já existe uma conta ou tentativa para esse barbeiro. Atualize e confira a lista.",
  staff_account_not_found: "Conta ou barbeiro não encontrado nesta barbearia.",
  staff_account_must_be_inactive:
    "Desative o acesso atual antes de substituir a conta. Depois, atualize a lista.",
  staff_account_needs_review:
    "O convite ainda precisa ser confirmado. Atualize a lista e peça ajuda ao suporte antes de enviar outro.",
  staff_accounts_not_configured:
    "Não foi possível criar o acesso agora. Fale com o suporte para continuar.",
  staff_account_provider_rejected:
    "Não encontramos essa conta. Confira o e-mail e o código de recuperação com o suporte.",
  invalid_appointment: "Confira o horário e os dados da reserva. O início precisa estar no futuro.",
  slot_unavailable: "Esse horário não está mais disponível. Atualize os horários e escolha outro.",
  idempotency_conflict:
    "Esta tentativa já foi usada com outros dados. Confira o histórico antes de iniciar outra operação.",
  idempotency_expired:
    "A tentativa expirou. Consulte pelo código ou contate a barbearia antes de fazer outra reserva.",
  cancellation_closed:
    "Este agendamento não pode mais ser cancelado. O cancelamento é permitido antes do início.",
  rate_limited: "Muitas tentativas em pouco tempo. Aguarde um minuto e tente novamente.",
};
export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Não foi possível concluir a operação.";
}

// Supabase session restoration does not accept our AbortSignal. Bound the wait
// without cancelling the SDK's own session maintenance or storing another token.
function abortable<T>(operation: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const abort = () => reject(new DOMException("Request aborted", "AbortError"));
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
    operation.then(
      (value) => {
        signal.removeEventListener("abort", abort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", abort);
        reject(error);
      },
    );
  });
}
export async function requestJSON(
  path: string,
  options: {
    method?: "GET" | "POST" | "PUT" | "DELETE";
    body?: unknown;
    signal?: AbortSignal;
    authenticated?: boolean;
    idempotencyKey?: string;
    expectedUserID?: string;
    expectedNoContent?: boolean;
  } = {},
): Promise<unknown> {
  const method = options.method ?? "GET";
  let base: string;
  try {
    base = apiBaseURL();
  } catch (error) {
    throw new ApiError("configuration", errorMessage(error));
  }
  const body = options.body === undefined ? undefined : JSON.stringify(options.body);
  if (body && new TextEncoder().encode(body).length > 65536)
    throw new ApiError("body_too_large", messages.body_too_large, 413);
  const timeout = new AbortController();
  let timer = setTimeout(() => timeout.abort(), SESSION_TIMEOUT_MS);
  const signal = options.signal
    ? AbortSignal.any([options.signal, timeout.signal])
    : timeout.signal;
  try {
    const headers: Record<string, string> = {};
    if (body) headers["Content-Type"] = "application/json";
    if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey;
    if (options.authenticated) {
      let accessToken: string | null;
      try {
        accessToken = await abortable(
          (async () => {
            const auth = await import("../supabase/auth");
            signal.throwIfAborted();
            return auth.currentAccessToken(options.expectedUserID);
          })(),
          signal,
        );
      } catch (error) {
        if (signal.aborted) throw error;
        throw new ApiError("authentication_unavailable", errorMessage(error));
      }
      if (!accessToken) {
        throw new ApiError("authentication_required", messages.authentication_required, 401);
      }
      headers.Authorization = "Bearer " + accessToken;
    }
    signal.throwIfAborted();
    clearTimeout(timer);
    timer = setTimeout(() => timeout.abort(), REQUEST_TIMEOUT_MS);
    const response = await fetch(base + path, {
      method,
      body,
      signal,
      cache: "no-store",
      credentials: "omit",
      headers: Object.keys(headers).length ? headers : undefined,
    });
    if (response.ok && options.expectedNoContent) {
      if (response.status !== 204)
        throw new ApiError(
          "invalid_response",
          "Não conseguimos confirmar a alteração. Atualize a página para conferir antes de tentar novamente.",
          response.status,
        );
      return undefined;
    }
    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      if (signal.aborted) throw new Error("Request aborted");
      throw new ApiError(
        "invalid_response",
        "Não conseguimos carregar as informações. Tente novamente em instantes.",
        response.status,
      );
    }
    if (!response.ok) {
      const envelope = payload as { error?: { code?: unknown } };
      const code = typeof envelope?.error?.code === "string" ? envelope.error.code : "http_error";
      throw new ApiError(
        code,
        messages[code] ?? "Não foi possível concluir seu pedido. Tente novamente em instantes.",
        response.status,
      );
    }
    return payload;
  } catch (error) {
    if (options.signal?.aborted) throw error;
    if (error instanceof ApiError) throw error;
    const reason = timeout.signal.aborted
      ? "A resposta está demorando mais do que o normal."
      : "Não conseguimos conectar agora. Confira sua internet.";
    throw new ApiError(
      timeout.signal.aborted ? "timeout" : "network_error",
      reason +
        (method !== "GET"
          ? options.idempotencyKey
            ? " Ainda não conseguimos confirmar se foi salvo. Use Repetir confirmação para conferir sem duplicar seu pedido."
            : " Ainda não conseguimos confirmar se foi salvo. Confira os dados antes de tentar novamente."
          : " Tente novamente em instantes."),
    );
  } finally {
    clearTimeout(timer);
  }
}
