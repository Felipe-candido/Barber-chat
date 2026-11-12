import { NextRequest, NextResponse } from "next/server";
import { securityHeaders } from "./lib/security-headers";

export function proxy(request: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const development = process.env.NODE_ENV === "development";
  const local = ["localhost", "127.0.0.1", "[::1]"].includes(request.nextUrl.hostname);
  const secureSite = !development && !local;
  const policy = securityHeaders(
    nonce,
    process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8080",
    process.env.NEXT_PUBLIC_SUPABASE_URL ?? "http://127.0.0.1:54321",
    development,
    secureSite,
  );
  const headers = new Headers(request.headers);
  headers.set("x-nonce", nonce);
  headers.set("Content-Security-Policy", policy);
  const response = NextResponse.next({ request: { headers } });
  response.headers.set("Content-Security-Policy", policy);
  response.headers.set("Cache-Control", "private, no-store");
  if (secureSite) response.headers.set("Strict-Transport-Security", "max-age=31536000");
  return response;
}

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|icon.svg|icons/|sw\\.js$|offline\\.(?:html|css)$|manifest\\.webmanifest$).*)",
  ],
};
