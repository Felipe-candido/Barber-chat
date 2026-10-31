import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

export type BillingPortal = {
  shop_id: string;
  url: string;
  open_until: string;
  livemode: false;
};

export function parseBillingPortal(value: unknown, shopID: string): BillingPortal {
  function invalid(): never {
    throw new ApiError(
      "invalid_response",
      "O endereço do portal não foi validado. Confira a situação e tente novamente pelas Configurações.",
    );
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid();
  const p = value as Record<string, unknown>;
  if (
    p.shop_id !== shopID ||
    p.livemode !== false ||
    typeof p.url !== "string" ||
    p.url.length > 8192 ||
    typeof p.open_until !== "string" ||
    !Number.isFinite(Date.parse(p.open_until)) ||
    Date.parse(p.open_until) <= Date.now() + 30_000 ||
    Date.parse(p.open_until) > Date.now() + 5 * 60_000
  )
    invalid();
  let url: URL;
  try {
    url = new URL(p.url);
  } catch {
    invalid();
  }
  if (
    url.protocol !== "https:" ||
    url.host !== "billing.stripe.com" ||
    url.username ||
    url.password ||
    url.hash ||
    (url.pathname !== "/p/session" && !url.pathname.startsWith("/p/session/"))
  )
    invalid();
  return { shop_id: shopID, url: url.href, open_until: p.open_until, livemode: false };
}

export async function openBillingPortal(
  slug: string,
  shopID: string,
  userID: string,
  signal: AbortSignal,
) {
  return parseBillingPortal(
    await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/billing/portal`, {
      method: "POST",
      body: {},
      authenticated: true,
      expectedUserID: userID,
      signal,
    }),
    shopID,
  );
}
