import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

export type SubscriptionOffer = {
  shop_id: string;
  offer_id: string;
  plan_key: string;
  amount_cents: number;
  currency: "BRL";
  interval: "month";
  interval_count: 1;
  livemode: false;
  recovering: boolean;
};
export type SubscriptionCheckout = {
  attempt_id: string;
  intent: "publish" | "manual_booking";
  url: string;
  expires_at: string;
  livemode: false;
};
const hex = /^[a-f0-9]{64}$/;
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou condições ou endereço de pagamento incompatíveis. Não abra links alternativos; confira a situação antes de tentar novamente.",
  );
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid();
  return value as Record<string, unknown>;
}
export function parseSubscriptionOffer(value: unknown, shopID: string): SubscriptionOffer {
  const o = record(value);
  if (
    o.shop_id !== shopID ||
    typeof o.offer_id !== "string" ||
    !hex.test(o.offer_id) ||
    typeof o.plan_key !== "string" ||
    !o.plan_key.trim() ||
    o.plan_key.length > 100 ||
    !Number.isSafeInteger(o.amount_cents) ||
    Number(o.amount_cents) <= 0 ||
    o.currency !== "BRL" ||
    o.interval !== "month" ||
    o.interval_count !== 1 ||
    o.livemode !== false ||
    typeof o.recovering !== "boolean"
  )
    invalid();
  return {
    shop_id: shopID,
    offer_id: o.offer_id,
    plan_key: o.plan_key,
    amount_cents: Number(o.amount_cents),
    currency: "BRL",
    interval: "month",
    interval_count: 1,
    livemode: false,
    recovering: o.recovering,
  };
}
export function parseSubscriptionCheckout(value: unknown): SubscriptionCheckout {
  const o = record(value);
  if (
    typeof o.attempt_id !== "string" ||
    !uuid.test(o.attempt_id) ||
    o.attempt_id === "00000000-0000-0000-0000-000000000000" ||
    !["publish", "manual_booking"].includes(String(o.intent)) ||
    o.livemode !== false ||
    typeof o.url !== "string" ||
    o.url.length > 8192 ||
    typeof o.expires_at !== "string" ||
    !Number.isFinite(Date.parse(o.expires_at)) ||
    Date.parse(o.expires_at) <= Date.now() + 30_000
  )
    invalid();
  let url: URL;
  try {
    url = new URL(o.url);
  } catch {
    invalid();
  }
  if (
    url.protocol !== "https:" ||
    url.host !== "checkout.stripe.com" ||
    url.username ||
    url.password ||
    !url.pathname.startsWith("/c/")
  )
    invalid();
  return {
    attempt_id: o.attempt_id,
    intent: o.intent as SubscriptionCheckout["intent"],
    url: url.href,
    expires_at: o.expires_at,
    livemode: false,
  };
}
export async function getSubscriptionOffer(
  slug: string,
  shopID: string,
  userID: string,
  signal: AbortSignal,
) {
  return parseSubscriptionOffer(
    await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/billing/offer`, {
      authenticated: true,
      expectedUserID: userID,
      signal,
    }),
    shopID,
  );
}
export async function startSubscriptionCheckout(
  slug: string,
  userID: string,
  offerID: string,
  signal: AbortSignal,
  intent: SubscriptionCheckout["intent"] = "publish",
) {
  if (!hex.test(offerID)) invalid();
  return parseSubscriptionCheckout(
    await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/billing/checkout`, {
      method: "POST",
      authenticated: true,
      expectedUserID: userID,
      body: { intent, offer_id: offerID },
      signal,
    }),
  );
}
