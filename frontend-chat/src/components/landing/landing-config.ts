// Public marketing settings. Everything here is presentation only: prices and
// plan rules are always confirmed by the API and Stripe before checkout.
export const PRODUCT_NAME = "Na Régua";

// Monthly price as charged by the Stripe price in STRIPE_MONTHLY_PRICE_ID.
// Keep both in step: the checkout always shows the real amount before payment.
export const PRICE_LABEL = (process.env.NEXT_PUBLIC_LANDING_PRICE ?? "").trim() || "R$ 49,90/mês";

// Free trial days advertised on the landing page. Leave 0 until the checkout
// really grants a trial; the page then promises free setup instead.
const trialDays = Number.parseInt(process.env.NEXT_PUBLIC_TRIAL_DAYS ?? "0", 10);
export const TRIAL_DAYS =
  Number.isInteger(trialDays) && trialDays > 0 && trialDays <= 90 ? trialDays : 0;

// Optional sales WhatsApp in international format, digits only (e.g. 5511999999999).
const salesWhatsApp = (process.env.NEXT_PUBLIC_SALES_WHATSAPP ?? "").replace(/\D/g, "");

const CHANNELS: Record<string, string> = {
  instagram: "Instagram",
  ig: "Instagram",
  tiktok: "TikTok",
  tt: "TikTok",
};

// Maps ?origem= or ?utm_source= to a known channel name. Unknown values are ignored
// so arbitrary query text never reaches the prefilled message.
export function channelFrom(value: string | string[] | undefined) {
  const raw = (Array.isArray(value) ? value[0] : value)?.trim().toLowerCase() ?? "";
  return CHANNELS[raw] ?? "";
}

export function whatsAppHref(channel: string) {
  if (salesWhatsApp.length < 10 || salesWhatsApp.length > 15) return "";
  const text = channel
    ? `Oi! Vi o ${PRODUCT_NAME} no ${channel} e quero saber mais para a minha barbearia.`
    : `Oi! Quero saber mais sobre o ${PRODUCT_NAME} para a minha barbearia.`;
  return `https://wa.me/${salesWhatsApp}?text=${encodeURIComponent(text)}`;
}
