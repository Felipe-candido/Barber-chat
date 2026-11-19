export type SupabaseBrowserConfig = {
  url: string;
  publishableKey: string;
};

export function supabaseBrowserConfig(): SupabaseBrowserConfig {
  const rawURL = (process.env.NEXT_PUBLIC_SUPABASE_URL ?? "").trim();
  const publishableKey = (process.env.NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY ?? "").trim();

  if (!rawURL || !publishableKey) {
    throw new Error(
      "O acesso está indisponível no momento. Tente mais tarde ou fale com o suporte.",
    );
  }
  if (publishableKey.startsWith("sb_secret_") || publishableKey === "service_role") {
    throw new Error("O acesso está indisponível no momento. Fale com o suporte para continuar.");
  }

  let url: URL;
  try {
    url = new URL(rawURL);
  } catch {
    throw new Error("O acesso está indisponível no momento. Fale com o suporte para continuar.");
  }
  const localHTTP =
    url.protocol === "http:" &&
    (url.hostname === "localhost" || url.hostname === "127.0.0.1" || url.hostname === "::1");
  if (
    (url.protocol !== "https:" && !localHTTP) ||
    url.username ||
    url.password ||
    (url.pathname !== "/" && url.pathname !== "") ||
    url.search ||
    url.hash
  ) {
    throw new Error("O acesso está indisponível no momento. Fale com o suporte para continuar.");
  }

  return { url: url.toString().replace(/\/$/, ""), publishableKey };
}
