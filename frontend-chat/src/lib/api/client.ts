import { apiBaseURL } from "./config";

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
  admin_access_unavailable:
    "A criação local está desabilitada. Confira DEV_SHOP_SLUG e DEV_FRONTEND_ORIGIN no backend.",
  shop_not_found: "Barbearia não encontrada ou inativa. Confira o slug configurado.",
  invalid_body: "A API rejeitou o formato do cadastro. Revise os campos.",
  unsupported_media_type: "A API exige o corpo da requisição em JSON.",
  body_too_large: "O cadastro ultrapassa o limite de 64 KiB. Reduza a descrição.",
  invalid_service: "Revise o nome, a duração e o preço do serviço.",
  temporarily_unavailable: "A API está temporariamente indisponível. Tente novamente em instantes.",
  internal_error: "A API não conseguiu concluir a operação.",
};
export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Não foi possível concluir a operação.";
}
export async function requestJSON(
  path: string,
  options: { method?: "GET" | "POST"; body?: unknown; signal?: AbortSignal } = {},
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
  const timer = setTimeout(() => timeout.abort(), 10000);
  const signal = options.signal
    ? AbortSignal.any([options.signal, timeout.signal])
    : timeout.signal;
  try {
    const response = await fetch(base + path, {
      method,
      body,
      signal,
      cache: "no-store",
      credentials: "omit",
      headers: body ? { "Content-Type": "application/json" } : undefined,
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
          ? " O resultado do envio é incerto: atualize a lista antes de repetir o cadastro."
          : ""),
    );
  } finally {
    clearTimeout(timer);
  }
}
