export const configuredShopSlug = (process.env.NEXT_PUBLIC_SHOP_SLUG ?? "").trim();
export const publicBookingPath = configuredShopSlug
  ? "/b/" + encodeURIComponent(configuredShopSlug)
  : "/chat";

export function apiBaseURL() {
  const raw = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8080";
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error("Configure NEXT_PUBLIC_API_BASE_URL com a URL HTTP da API.");
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  )
    throw new Error(
      "NEXT_PUBLIC_API_BASE_URL deve conter somente a URL base HTTP da API, sem credenciais.",
    );
  return url.toString().replace(/\/$/, "");
}
