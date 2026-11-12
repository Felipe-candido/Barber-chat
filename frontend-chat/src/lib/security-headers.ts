// Server-only policy construction. No credentials, broad HTTPS wildcard or
// caller-provided forwarding headers are used to select permitted origins.
export function securityHeaders(
  nonce: string,
  apiURL: string,
  supabaseURL: string,
  development: boolean,
  secureSite: boolean,
) {
  const allowedOrigin = (raw: string) => {
    const url = new URL(raw);
    const local = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
    if (
      url.username ||
      url.password ||
      !["https:", "http:"].includes(url.protocol) ||
      (secureSite && url.protocol !== "https:")
    )
      throw new Error("Security policy requires configured HTTPS API and Supabase origins.");
    if (url.protocol === "http:" && !local)
      throw new Error("HTTP origins are allowed only for local development.");
    return url.origin;
  };
  const api = allowedOrigin(apiURL),
    supabase = allowedOrigin(supabaseURL);
  const connect = new Set(["'self'", api, supabase, supabase.replace(/^http/, "ws")]);
  if (development) {
    connect.add("ws://localhost:*");
    connect.add("ws://127.0.0.1:*");
  }
  const directives = [
    "default-src 'self'",
    "worker-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${development ? " 'unsafe-eval'" : ""}`,
    `style-src 'self' 'nonce-${nonce}'`,
    // Existing chart/dialog layout uses React style properties. This exception
    // permits style attributes, while script attributes and style blocks stay restricted.
    "style-src-attr 'unsafe-inline'",
    `connect-src ${[...connect].join(" ")}`,
    "img-src 'self' data: blob:",
    "font-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "frame-src 'none'",
  ];
  if (secureSite) directives.push("upgrade-insecure-requests");
  return directives.join("; ");
}
