import type { Page } from "@playwright/test";

export const testEmail = "admin@example.com";
export const testPassword = "correct-horse-battery-staple";

export function createTestSession() {
  const now = Math.floor(Date.now() / 1000);
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString("base64url");
  const accessToken =
    encode({ alg: "HS256", typ: "JWT" }) +
    "." +
    encode({
      aud: "authenticated",
      exp: now + 3600,
      iat: now,
      sub: "11111111-1111-4111-8111-111111111111",
    }) +
    ".test-signature";
  return {
    accessToken,
    response: {
      access_token: accessToken,
      token_type: "bearer",
      expires_in: 3600,
      expires_at: now + 3600,
      refresh_token: "test-refresh-token",
      user: {
        id: "11111111-1111-4111-8111-111111111111",
        aud: "authenticated",
        role: "authenticated",
        email: testEmail,
        email_confirmed_at: new Date().toISOString(),
        phone: "",
        app_metadata: { provider: "email", providers: ["email"] },
        user_metadata: {},
        identities: [],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        is_anonymous: false,
      },
    },
  };
}

export async function installSupabaseSession(page: Page) {
  const session = createTestSession();
  await page.addInitScript(
    ({ value }) => localStorage.setItem("sb-test-project-auth-token", JSON.stringify(value)),
    { value: session.response },
  );
  return session;
}
