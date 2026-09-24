export type SupabaseBrowserConfig = {
  url: string;
  publishableKey: string;
};

export function supabaseBrowserConfig(): SupabaseBrowserConfig {
  const rawURL = (process.env.NEXT_PUBLIC_SUPABASE_URL ?? "").trim();
  const publishableKey = (process.env.NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY ?? "").trim();

  if (!rawURL || !publishableKey) {
    throw new Error("Configure NEXT_PUBLIC_SUPABASE_URL e NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY.");
  }
  if (publishableKey.startsWith("sb_secret_") || publishableKey === "service_role") {
    throw new Error("Use somente a chave publishable do Supabase no frontend.");
  }

  let url: URL;
  try {
    url = new URL(rawURL);
  } catch {
    throw new Error("NEXT_PUBLIC_SUPABASE_URL deve ser uma URL válida.");
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
    throw new Error(
      "NEXT_PUBLIC_SUPABASE_URL deve ser a origem HTTPS do projeto (HTTP é aceito apenas localmente).",
    );
  }

  return { url: url.toString().replace(/\/$/, ""), publishableKey };
}
