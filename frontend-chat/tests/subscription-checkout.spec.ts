import { test, expect, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, otherShop, testShop } from "./supabase-fixture";
import {
  parseSubscriptionCheckout,
  parseSubscriptionOffer,
} from "../src/lib/api/subscription-checkout";

const path = `/admin/${testShop.slug}/configuracoes`;
const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}`;
const offer = {
  shop_id: testShop.shop_id,
  offer_id: "a".repeat(64),
  plan_key: "standard_monthly",
  amount_cents: 9900,
  currency: "BRL",
  interval: "month",
  interval_count: 1,
  livemode: false,
  recovering: false,
};
const checkout = () => ({
  attempt_id: "55555555-5555-4555-8555-555555555555",
  intent: "publish",
  url: "https://checkout.stripe.com/c/pay/cs_test_example#test",
  expires_at: new Date(Date.now() + 3_600_000).toISOString(),
  livemode: false,
});
function commercial(paid = false) {
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
      status: paid ? "active" : "none",
      plan_key: paid ? "standard_monthly" : "",
      operation_allowed: paid,
      reason: paid ? "subscribed" : "subscription_required",
      coverage_until: paid ? new Date(Date.now() + 86_400_000).toISOString() : null,
      cancel_at_period_end: false,
    },
  };
}
const dialog = (page: Page) => page.getByRole("dialog", { name: "Assinatura mensal da barbearia" });
const start = (page: Page) =>
  page.getByRole("button", { name: "Ver plano e assinar", exact: true });
const prepare = (page: Page) =>
  dialog(page).getByRole("button", { name: "Preparar Checkout", exact: true });
async function setup(page: Page) {
  const session = await installSupabaseSession(page);
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
  return session;
}

test("price and hosted URL parsers reject incompatible, expired and cross-tenant responses", () => {
  expect(parseSubscriptionOffer(offer, testShop.shop_id)).toEqual(offer);
  for (const invalid of [
    null,
    {},
    { ...offer, shop_id: otherShop.shop_id },
    { ...offer, livemode: true },
    { ...offer, currency: "USD" },
    { ...offer, interval: "year" },
    { ...offer, interval_count: 2 },
    { ...offer, amount_cents: 0 },
    { ...offer, amount_cents: 1.5 },
    { ...offer, amount_cents: Number.MAX_SAFE_INTEGER + 1 },
    { ...offer, offer_id: "fake" },
  ])
    expect(() => parseSubscriptionOffer(invalid, testShop.shop_id)).toThrow();
  expect(parseSubscriptionCheckout(checkout()).url).toContain("https://checkout.stripe.com/c/");
  for (const invalid of [
    null,
    { ...checkout(), livemode: true },
    { ...checkout(), expires_at: new Date().toISOString() },
    { ...checkout(), attempt_id: "00000000-0000-0000-0000-000000000000" },
    ...[
      "javascript:alert(1)",
      "http://checkout.stripe.com/c/pay/fake",
      "https://checkout.stripe.com.evil.test/c/pay/fake",
      "https://user:secret@checkout.stripe.com/c/pay/fake",
      "https://checkout.stripe.com:444/c/pay/fake",
    ].map((url) => ({ ...checkout(), url })),
  ])
    expect(() => parseSubscriptionCheckout(invalid)).toThrow();
});

test("owner reviews server price before a single authenticated POST, then checks payment without publishing", async ({
  page,
}) => {
  const session = await setup(page);
  let paid = false,
    reads = 0,
    publications = 0;
  const writes: unknown[] = [];
  await page.route(base + "/commercial-status", (route) =>
    route.fulfill({ json: commercial(paid) }),
  );
  await page.route(base + "/publication", (route) => {
    publications++;
    return route.abort();
  });
  await page.route(base + "/billing/offer", (route) => {
    reads++;
    expect(route.request().method()).toBe("GET");
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    return route.fulfill({ json: offer });
  });
  await page.route(base + "/billing/checkout", async (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    writes.push(route.request().postDataJSON());
    await route.fulfill({ json: checkout() });
  });
  await page.goto(path);
  expect(reads).toBe(0);
  await start(page).click();
  await expect(dialog(page)).toContainText("R$ 99,00");
  await expect(dialog(page)).toContainText("por mês, por barbearia");
  await expect(dialog(page)).toContainText("não use cartão real");
  expect(writes).toEqual([]);
  await prepare(page).evaluate((button) => {
    (button as HTMLButtonElement).click();
    (button as HTMLButtonElement).click();
  });
  const link = dialog(page).getByRole("link", { name: "Continuar na Stripe" });
  await expect(link).toHaveAttribute("href", checkout().url);
  await expect(link).toHaveAttribute("target", "_blank");
  expect(writes).toEqual([{ intent: "publish", offer_id: offer.offer_id }]);
  paid = true;
  await dialog(page).getByRole("button", { name: "Conferir assinatura", exact: true }).click();
  await expect(dialog(page)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Publicar barbearia", exact: true })).toBeEnabled();
  await expect(start(page)).toHaveCount(0);
  expect(publications).toBe(0);
});

test("changed offer requires a new GET and explicit confirmation, without automatic POST", async ({
  page,
}) => {
  await setup(page);
  let reads = 0,
    writes = 0;
  await page.route(base + "/billing/offer", (route) => {
    reads++;
    return route.fulfill({
      json: {
        ...offer,
        offer_id: (reads === 1 ? "a" : "b").repeat(64),
        amount_cents: reads === 1 ? 9900 : 12900,
      },
    });
  });
  await page.route(base + "/billing/checkout", (route) => {
    writes++;
    if (writes === 1)
      return route.fulfill({ status: 409, json: { error: { code: "checkout_offer_changed" } } });
    expect(route.request().postDataJSON()).toEqual({ intent: "publish", offer_id: "b".repeat(64) });
    return route.fulfill({ json: checkout() });
  });
  await page.goto(path);
  await start(page).click();
  await prepare(page).click();
  await expect(dialog(page)).toContainText("condições");
  await dialog(page).getByRole("button", { name: "Consultar condições novamente" }).click();
  await expect(dialog(page)).toContainText("R$ 129,00");
  expect(writes).toBe(1);
  await prepare(page).click();
  await expect(dialog(page).getByRole("link", { name: "Continuar na Stripe" })).toBeVisible();
  expect(writes).toBe(2);
});

test("lost response recovers frozen conditions only after explicit retry", async ({ page }) => {
  await setup(page);
  let writes = 0;
  await page.route(base + "/billing/offer", (route) =>
    route.fulfill({ json: { ...offer, recovering: writes > 0 } }),
  );
  await page.route(base + "/billing/checkout", (route) => {
    writes++;
    return writes === 1 ? route.abort("failed") : route.fulfill({ json: checkout() });
  });
  await page.goto(path);
  await start(page).click();
  await prepare(page).click();
  await expect(dialog(page)).toContainText("A tentativa pode ter sido registrada");
  await dialog(page).getByRole("button", { name: "Consultar condições novamente" }).click();
  await expect(dialog(page)).toContainText("condições preservadas");
  expect(writes).toBe(1);
  await prepare(page).click();
  await expect(dialog(page).getByRole("link", { name: "Continuar na Stripe" })).toBeVisible();
  expect(writes).toBe(2);
});

test("dirty settings block Checkout; disabled billing and malicious URLs cannot expose a payment link", async ({
  page,
}) => {
  await setup(page);
  let reads = 0,
    configured = false;
  await page.route(base + "/billing/offer", (route) => {
    reads++;
    return configured
      ? route.fulfill({ json: offer })
      : route.fulfill({ status: 503, json: { error: { code: "billing_not_configured" } } });
  });
  await page.route(base + "/billing/checkout", (route) =>
    route.fulfill({ json: { ...checkout(), url: "https://evil.test/pay" } }),
  );
  await page.goto(path);
  await expect(start(page)).toBeEnabled();
  await page.getByLabel("Nome da barbearia", { exact: true }).fill("Rascunho mantido");
  await expect(start(page)).toBeDisabled();
  expect(reads).toBe(0);
  await page.getByLabel("Nome da barbearia", { exact: true }).fill(testShop.name);
  await start(page).click();
  await expect(dialog(page)).toContainText("STRIPE_WEBHOOK_SECRET");
  await expect(prepare(page)).toHaveCount(0);
  configured = true;
  await dialog(page).getByRole("button", { name: "Consultar condições novamente" }).click();
  await prepare(page).click();
  await expect(dialog(page)).toContainText("endereço de pagamento incompatíveis");
  await expect(dialog(page).getByRole("link")).toHaveCount(0);
});

test("existing contracts, manual shops and barbers never offer another subscription", async ({
  page,
}) => {
  await setup(page);
  let offerReads = 0;
  await page.route(base + "/billing/offer", (route) => {
    offerReads++;
    return route.abort();
  });
  await page.route(base + "/commercial-status", (route) =>
    route.fulfill({ json: commercial(true) }),
  );
  await page.goto(path);
  await expect(page.getByRole("button", { name: "Publicar barbearia", exact: true })).toBeEnabled();
  await expect(start(page)).toHaveCount(0);
  await page.unroute(base + "/commercial-status");
  await page.reload();
  await expect(
    page.getByText("Esta barbearia mantém o acesso anterior", { exact: false }),
  ).toBeVisible();
  await expect(start(page)).toHaveCount(0);
  await page.route(base + "/access", (route) =>
    route.fulfill({
      json: { role: "barber", professional_id: "55555555-5555-4555-8555-555555555555" },
    }),
  );
  await page.reload();
  await expect(
    page.getByText("Somente o proprietário pode consultar", { exact: false }),
  ).toBeVisible();
  await expect(start(page)).toHaveCount(0);
  expect(offerReads).toBe(0);
});

test("Checkout review fits mobile, desktop and enlarged text with keyboard access", async ({
  page,
}) => {
  await setup(page);
  await page.route(base + "/billing/offer", (route) => route.fulfill({ json: offer }));
  await page.goto(path);
  await start(page).click();
  await expect(prepare(page)).toBeVisible();
  for (const width of [320, 390, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(dialog(page)).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: `test-results/subscription-${width}.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 390, height: 900 });
  await page.addStyleTag({ content: "html { font-size: 200% !important; }" });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toHaveCount(0);
});
