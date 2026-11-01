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
    "A assinatura de teste ainda não está habilitada. Confira STRIPE_SECRET_KEY, STRIPE_MONTHLY_PRICE_ID e STRIPE_WEBHOOK_SECRET no backend.",
  billing_not_required:
    "Esta barbearia mantém o acesso anterior. Não abra uma assinatura para ela sem orientação do responsável pela plataforma.",
  subscription_exists:
    "Já existe uma assinatura para esta barbearia. Confira a situação atual; não abra outra cobrança.",
  checkout_offer_changed:
    "As condições mudaram. Consulte e revise o plano novamente antes de continuar.",
  checkout_confirmation_pending:
    "O Checkout já foi concluído e aguarda confirmação financeira. Confira a assinatura, sem abrir outra tentativa.",
  checkout_requires_review:
    "A tentativa precisa de conferência pelo responsável pela plataforma. Não crie outra assinatura por fora.",
  checkout_expired:
    "A Stripe confirmou que o Checkout expirou. Consulte as condições; uma nova tentativa exige sua confirmação.",
  checkout_unavailable:
    "Não foi possível confirmar o Checkout. Consulte as condições novamente para recuperar a mesma tentativa.",
  portal_not_configured:
    "O portal de teste ainda não foi configurado pelo responsável pela plataforma.",
  portal_not_available:
    "A assinatura ainda não está disponível para gerenciamento. Atualize a situação; se o pagamento já foi feito, aguarde a confirmação.",
  portal_unavailable:
    "Não foi possível abrir o portal. Nenhuma alteração da assinatura foi solicitada por esta ação. Você pode preparar um novo acesso.",
  shop_not_found: "Barbearia não encontrada ou inativa. Volte à seleção de barbearias.",
  invalid_body: "A API rejeitou o formato do cadastro. Revise os campos.",
  unsupported_media_type: "A API exige o corpo da requisição em JSON.",
  body_too_large: "O cadastro ultrapassa o limite de 64 KiB. Reduza a descrição.",
  invalid_service: "Revise o nome, a duração e o preço do serviço.",
  invalid_professional: "Informe um nome de barbeiro com até 100 caracteres.",
  invalid_professional_services: "Selecione serviços ativos e agendáveis desta barbearia.",
  invalid_professional_schedule:
    "Confira os dias, abertura, fechamento, intervalo e margem da agenda.",
  temporarily_unavailable: "A API está temporariamente indisponível. Tente novamente em instantes.",
  internal_error: "A API não conseguiu concluir a operação.",
  authentication_required: "Entre novamente para concluir esta operação.",
  invalid_access_token: "Sua sessão não foi aceita pela API. Entre novamente.",
  access_denied:
    "Acesso não liberado. Confira com o responsável o cadastro do seu usuário e o vínculo ativo com a barbearia.",
  origin_not_allowed:
    "A origem do frontend não está autorizada no backend. Confira FRONTEND_ORIGIN.",
  invalid_shop_registration:
    "Confira seu nome, o nome da barbearia, o endereço do link e o fuso horário.",
  invalid_shop_address:
    "Informe o endereço completo: CEP, rua, número (ou S/N), bairro, cidade e estado.",
  invalid_shop_settings:
    "Confira o nome, o telefone brasileiro com DDD e a descrição de até 1000 caracteres.",
  shop_settings_conflict:
    "As configurações mudaram desde que você abriu esta tela. Confira a versão atual antes de salvar novamente.",
  shop_slug_taken: "Esse endereço de link já está em uso. Escolha outro para sua barbearia.",
  email_confirmation_required: "Confirme seu e-mail antes de cadastrar a barbearia.",
  registration_not_configured:
    "O cadastro ainda não está habilitado na API. Confira SUPABASE_PUBLISHABLE_KEY no backend.",
  publication_blocked:
    "A barbearia ainda não pode ser publicada. Confira a situação atual e complete as pendências.",
  publication_not_managed:
    "A publicação desta barbearia é gerenciada pelo responsável pela plataforma.",
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
    "A tentativa foi registrada, mas precisa ser conferida. Atualize a lista e confira a conta no Supabase antes de enviar outro convite.",
  staff_accounts_not_configured:
    "Configure SUPABASE_SERVICE_ROLE_KEY somente no backend e reinicie a API para criar contas.",
  staff_account_provider_rejected:
    "O Supabase não confirmou essa conta. Confira o UUID e o e-mail no Supabase Authentication.",
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
    method?: "GET" | "POST" | "PUT";
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
          "A API não confirmou a atualização no formato esperado.",
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
        "A API retornou uma resposta inválida.",
        response.status,
      );
    }
    if (!response.ok) {
      const envelope = payload as { error?: { code?: unknown } };
      const code = typeof envelope?.error?.code === "string" ? envelope.error.code : "http_error";
      throw new ApiError(
        code,
        messages[code] ?? "A API retornou um erro (HTTP " + response.status + ").",
        response.status,
      );
    }
    return payload;
  } catch (error) {
    if (options.signal?.aborted) throw error;
    if (error instanceof ApiError) throw error;
    const reason = timeout.signal.aborted
      ? "A API demorou mais que o esperado."
      : "Não foi possível acessar a API. Confira a URL, o servidor e a origem permitida (CORS).";
    throw new ApiError(
      timeout.signal.aborted ? "timeout" : "network_error",
      reason +
        (method !== "GET"
          ? options.idempotencyKey
            ? " O resultado é incerto. Use Repetir confirmação para recuperar a mesma tentativa."
            : " O resultado do envio é incerto: consulte o estado antes de iniciar outra operação."
          : ""),
    );
  } finally {
    clearTimeout(timer);
  }
}
