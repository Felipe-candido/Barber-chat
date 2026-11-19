export function shopPathSegment(slug: string) {
  if (!slug || slug !== slug.trim() || /[\/\\\s]/.test(slug) || slug === "." || slug === "..") {
    throw new Error(
      "O link desta barbearia está incompleto. Confira o endereço e tente novamente.",
    );
  }
  return encodeURIComponent(slug);
}

export function adminShopPath(slug: string) {
  return "/admin/" + shopPathSegment(slug);
}

export function publicBookingPath(slug: string) {
  return "/b/" + shopPathSegment(slug);
}

export function apiBaseURL() {
  const raw = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8080";
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(
      "O serviço está indisponível no momento. Tente mais tarde ou fale com o suporte.",
    );
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  )
    throw new Error(
      "O serviço está indisponível no momento. Tente mais tarde ou fale com o suporte.",
    );
  return url.toString().replace(/\/$/, "");
}
