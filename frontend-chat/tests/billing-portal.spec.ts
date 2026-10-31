import { test, expect, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, otherShop, testShop } from "./supabase-fixture";
import { parseBillingPortal } from "../src/lib/api/billing-portal";

const path = `/admin/${testShop.slug}/configuracoes`;
const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}`;
const session = () => ({
  shop_id: testShop.shop_id,
  url: "https://billing.stripe.com/p/session/test",
  open_until: new Date(Date.now() + 240_000).toISOString(),
  livemode: false,
});
function commercial(status = "active", canceled = false) {
  const paid = status === "active";
  return {
    shop_id: testShop.shop_id,
    slug: testShop.slug,
    publication_managed: true,
    publication_requested: false,
    public_booking_enabled: false,
    can_publish: paid,
    blockers: paid ? [] : ["subscription_required"],
    readiness: { address: true, service: true, professional: true, schedule: true },
    subscription: {
      status,
      plan_key: status === "none" ? "" : "standard_monthly",
      operation_allowed: paid,
      reason: paid
        ? "subscribed"
        : status === "canceled"
          ? "subscription_canceled"
          : "subscription_required",
      coverage_until: paid ? new Date(Date.now() + 86_400_000).toISOString() : null,
      cancel_at_period_end: canceled,
    },
  };
}
async function setup(page: Page) {
  const identity = await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  await page.route(base + "/settings", (route) =>
    route.fulfill({
      json: {
        ...testShop,
        timezone: "America/Sao_Paulo",
        phone: "",
        description: "",
        version: 1,
        address: {
          postal_code: "01310100",
          street: "Avenida Paulista",
          number: "100",
          complement: "",
          district: "Bela Vista",
          city: "São Paulo",
          state: "SP",
        },
      },
    }),
  );
  await page.route(base + "/commercial-status", (route) => route.fulfill({ json: commercial() }));
  return identity;
}
const dialog = (page: Page) => page.getByRole("dialog", { name: "Gerenciar assinatura na Stripe" });
const manage = (page: Page) =>
  page.getByRole("button", { name: "Gerenciar assinatura", exact: true });

test("portal parser accepts only this tenant's fresh TEST Stripe session", () => {
  expect(parseBillingPortal(session(), testShop.shop_id).url).toContain(
    "https://billing.stripe.com/p/session/",
  );
  for (const invalid of [
    null,
    {},
    { ...session(), shop_id: otherShop.shop_id },
    { ...session(), livemode: true },
    { ...session(), open_until: new Date().toISOString() },
    { ...session(), open_until: new Date(Date.now() + 3_600_000).toISOString() },
    ...[
      "javascript:alert(1)",
      "https://billing.stripe.com.evil.test/p/session/test",
      "http://billing.stripe.com/p/session/test",
      "https://user:secret@billing.stripe.com/p/session/test",
      "https://billing.stripe.com:444/p/session/test",
      "https://billing.stripe.com/other",
      "https://billing.stripe.com/p/session/test#fragment",
    ].map((url) => ({ ...session(), url })),
  ]) {
    expect(() => parseBillingPortal(invalid, testShop.shop_id)).toThrow();
  }
});

test("owner opens only its Customer portal and preserves draft; checking only reads status", async ({
  page,
}) => {
  const identity = await setup(page);
  let requests = 0,
    publications = 0,
    canceled = false,
    reads = 0;
  await page.route(base + "/commercial-status", (route) => {
    reads++;
    return route.fulfill({ json: commercial("active", canceled) });
  });
  await page.route(base + "/publication", (route) => {
    publications++;
    return route.abort();
  });
  await page.route(base + "/billing/portal", async (route) => {
    requests++;
    expect(route.request().method()).toBe("POST");
    expect(route.request().headers().authorization).toBe("Bearer " + identity.accessToken);
    expect(route.request().postDataJSON()).toEqual({});
    await route.fulfill({ json: session() });
  });
  await page.goto(path);
  await expect(manage(page)).toBeVisible();
  expect(requests).toBe(0);
  await page.getByLabel("Nome da barbearia", { exact: true }).fill("Meu rascunho");
  await manage(page).click();
  const link = dialog(page).getByRole("link", { name: "Abrir portal da Stripe" });
  await expect(link).toHaveAttribute("href", session().url);
  await expect(link).toHaveAttribute("target", "_blank");
  expect(requests).toBe(1);
  canceled = true;
  await dialog(page).getByRole("button", { name: "Conferir situação", exact: true }).click();
  await expect(page.getByText("Há um cancelamento programado.", { exact: false })).toBeVisible();
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toHaveValue("Meu rascunho");
  expect(publications).toBe(0);
  expect(requests).toBe(1);
  expect(reads).toBeGreaterThan(1);
});

test("errors and incompatible URL never expose links or retry automatically", async ({ page }) => {
  await setup(page);
  let requests = 0;
  await page.route(base + "/billing/portal", (route) => {
    requests++;
    return requests === 1
      ? route.fulfill({ status: 503, json: { error: { code: "portal_unavailable" } } })
      : route.fulfill({ json: { ...session(), shop_id: otherShop.shop_id } });
  });
  await page.goto(path);
  await manage(page).click();
  await expect(dialog(page).getByRole("alert")).toContainText("Não foi possível abrir o portal");
  await expect(dialog(page).getByRole("link")).toHaveCount(0);
  expect(requests).toBe(1);
  await dialog(page).getByRole("button", { name: "Preparar novo acesso" }).click();
  await expect(dialog(page).getByRole("alert")).toContainText("não foi validado");
  await expect(dialog(page).getByRole("link")).toHaveCount(0);
  expect(requests).toBe(2);
  await page.keyboard.press("Escape");
  await expect(dialog(page)).not.toBeVisible();
});

test("short opening window removes stale links", async ({ page }) => {
  await setup(page);
  await page.clock.install();
  await page.route(base + "/billing/portal", (route) => route.fulfill({ json: session() }));
  await page.goto(path);
  await manage(page).click();
  await expect(dialog(page).getByRole("link")).toBeVisible();
  await page.clock.fastForward(215_000);
  await expect(dialog(page).getByRole("link")).toHaveCount(0);
  await expect(dialog(page).getByRole("alert")).toContainText(
    "prazo para abrir este link terminou",
  );
});

test("return URL does not activate or publish and canceled contracts retain history access", async ({
  page,
}) => {
  await setup(page);
  let posts = 0;
  await page.route(base + "/commercial-status", (route) =>
    route.fulfill({ json: commercial("canceled") }),
  );
  await page.route(base + "/billing/portal", (route) => {
    posts++;
    return route.fulfill({ json: session() });
  });
  await page.goto(path + "?portal=returned&paid=true");
  await expect(page.getByText("Você voltou do portal.", { exact: false })).toBeVisible();
  await expect(page.getByText("Assinatura encerrada", { exact: true })).toBeVisible();
  await expect(manage(page)).toBeVisible();
  await expect(page.getByRole("link", { name: "Abrir agendamento" })).toHaveCount(0);
  expect(posts).toBe(0);
});

test("legacy shops and shops without a subscription have no portal", async ({ page }) => {
  await setup(page);
  await page.route(base + "/commercial-status", (route) =>
    route.fulfill({ json: commercial("none") }),
  );
  await page.goto(path);
  await expect(page.getByText("Sem assinatura confirmada", { exact: true })).toBeVisible();
  await expect(manage(page)).toHaveCount(0);
  await page.route(base + "/commercial-status", (route) =>
    route.fulfill({
      json: {
        ...commercial(),
        publication_managed: false,
        public_booking_enabled: true,
        can_publish: false,
        blockers: ["operator_managed"],
        subscription: {
          ...commercial().subscription,
          status: "not_required",
          reason: "legacy_access",
          plan_key: "",
          coverage_until: null,
        },
      },
    }),
  );
  await page.reload();
  await expect(page.getByText("Acesso anterior mantido", { exact: true })).toBeVisible();
  await expect(manage(page)).toHaveCount(0);
});

test("portal dialog fits narrow screens and larger fonts", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 760 });
  await setup(page);
  await page.route(base + "/billing/portal", (route) => route.fulfill({ json: session() }));
  await page.goto(path);
  await page.addStyleTag({ content: "html { font-size:200% !important; }" });
  await manage(page).click();
  await expect(dialog(page).getByRole("link")).toBeVisible();
  expect(await dialog(page).evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
  await page.screenshot({ path: "test-results/billing-portal-mobile.png", fullPage: true });
  await page.keyboard.press("Escape");
  await expect(dialog(page)).not.toBeVisible();
});
