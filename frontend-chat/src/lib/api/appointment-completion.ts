import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { instant, integer, type ShopRequest } from "./products";

export type CompletionMode = "automatic" | "manual";
export type CompletionSettings = {
  shop_id: string;
  mode: CompletionMode;
  version: number;
  automatic_since: string | null;
  updated_at: string;
};
function parse(value: unknown, shopID: string): CompletionSettings {
  const v = value as CompletionSettings;
  if (
    !v ||
    v.shop_id !== shopID ||
    !["automatic", "manual"].includes(v.mode) ||
    !integer(v.version, 1) ||
    !instant(v.updated_at) ||
    (v.mode === "automatic" ? !instant(v.automatic_since) : v.automatic_since !== null)
  )
    throw new ApiError(
      "invalid_response",
      "Não foi possível confirmar o modo de fechamento. Consulte novamente antes de salvar.",
    );
  return v;
}
function path(scope: ShopRequest) {
  return `/api/v1/admin/shops/${shopPathSegment(scope.slug)}/appointment-completion`;
}
export async function getCompletionSettings(scope: ShopRequest, signal?: AbortSignal) {
  return parse(
    await requestJSON(path(scope), { authenticated: true, expectedUserID: scope.userID, signal }),
    scope.shopID,
  );
}
export async function saveCompletionSettings(
  scope: ShopRequest,
  mode: CompletionMode,
  version: number,
  signal?: AbortSignal,
) {
  const result = parse(
    await requestJSON(path(scope), {
      method: "PUT",
      body: { mode, version },
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope.shopID,
  );
  if (result.mode !== mode || result.version !== version + 1)
    throw new ApiError(
      "invalid_response",
      "O resultado do salvamento precisa ser conferido. Consulte o modo atual antes de tentar novamente.",
    );
  return result;
}
