import type { Page } from "@playwright/test";
import type { AccessibleShop } from "../src/lib/api/identity";

export const testEmail = "admin@example.com";
export const testPassword = "correct-horse-battery-staple";
export const testIdentity = {
  user_id: "11111111-1111-4111-8111-111111111111",
  display_name: "Felipe",
};
export const testShop: AccessibleShop = {
  shop_id: "33333333-3333-4333-8333-333333333333",
  name: "Barbearia do Felipe",
  slug: "barbearia-do-felipe",
};
export const otherShop: AccessibleShop = {
  shop_id: "44444444-4444-4444-8444-444444444444",
  name: "Barbearia Centro",
  slug: "barbearia-centro",
};

export const emptyAgenda = {
  month: "2026-10",
  today: "2026-10-03",
  from_date: "2026-09-28",
  until_date: "2026-11-09",
  shop_timezone: "America/Sao_Paulo",
  appointments: [],
  next_cursor: null,
};

export async function mockAdminAccess(page: Page, shops = [testShop]) {
  await page.route("http://127.0.0.1:8080/api/v1/admin/shops/*/commercial-status", (route) => {
    const slug = new URL(route.request().url()).pathname.split("/").at(-2);
    const shop = shops.find((item) => item.slug === slug);
    return shop
      ? route.fulfill({
          json: {
            shop_id: shop.shop_id,
            slug: shop.slug,
            publication_managed: false,
            publication_requested: false,
            public_booking_enabled: true,
            can_publish: false,
            blockers: ["operator_managed"],
            readiness: { address: false, service: false, professional: false, schedule: false },
            subscription: {
              status: "not_required",
              plan_key: "",
              operation_allowed: true,
              reason: "legacy_access",
              coverage_until: null,
              cancel_at_period_end: false,
            },
          },
        })
      : route.fulfill({ status: 403, json: { error: { code: "access_denied" } } });
  });
  await page.route("http://127.0.0.1:8080/api/v1/admin/shops/*/access", (route) =>
    route.fulfill({ json: { role: "owner", professional_id: null } }),
  );
  await page.route("http://127.0.0.1:8080/api/v1/admin/shops/*/appointments*", (route) =>
    route.fulfill({ json: emptyAgenda }),
  );
  for (const [path, payload] of [
    ["me", testIdentity],
    ["shops", shops],
  ] as const) {
    await page.route("http://127.0.0.1:8080/api/v1/admin/" + path, (route) =>
      route.fulfill({ json: payload }),
    );
  }
}

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
    ({ value }) => {
      if (!sessionStorage.getItem("test-session-installed")) {
        localStorage.setItem("sb-test-project-auth-token", JSON.stringify(value));
        sessionStorage.setItem("test-session-installed", "true");
      }
    },
    { value: session.response },
  );
  return session;
}
