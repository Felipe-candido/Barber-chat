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
  booking_option_not_found:
    "Este serviço ou profissional não está mais disponível. Escolha novamente.",
  appointment_not_found:
    "Código não encontrado nesta barbearia. Confira o código e o link utilizado.",
  invalid_customer: "Confira seu nome, sobrenome e telefone com DDD ou código internacional.",
  invalid_availability: "Escolha um serviço, profissional e uma data dentro do período disponível.",
  invalid_agenda: "Confira o mês selecionado e atualize a agenda para reiniciar a paginação.",
  invalid_appointment: "Confira o horário e os dados da reserva. O início precisa estar no futuro.",
  slot_unavailable: "Esse horário não está mais disponível. Atualize os horários e escolha outro.",
  idempotency_conflict:
    "Esta tentativa já foi usada com outros dados. Consulte a reserva antes de recomeçar.",
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
    method?: "GET" | "POST";
    body?: unknown;
    signal?: AbortSignal;
    authenticated?: boolean;
    idempotencyKey?: string;
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
            return auth.currentAccessToken();
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
        (method === "POST"
          ? options.idempotencyKey
            ? " O resultado é incerto. Use Repetir confirmação para recuperar a mesma tentativa."
            : " O resultado do envio é incerto: consulte o estado antes de iniciar outra operação."
          : ""),
    );
  } finally {
    clearTimeout(timer);
  }
}
