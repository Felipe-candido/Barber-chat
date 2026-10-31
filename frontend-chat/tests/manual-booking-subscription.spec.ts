import { test, expect, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}`;
const publicBase = `http://127.0.0.1:8080/api/v1/public/shops/${testShop.slug}`;
const professional = { id: "55555555-5555-4555-8555-555555555555", name: "Barbeiro de teste" };
const service = {
  id: "66666666-6666-4666-8666-666666666666",
  name: "Corte de teste",
  description: "",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  active: true,
};
const slot = { starts_at: "2026-11-02T12:30:00Z", ends_at: "2026-11-02T13:30:00Z" };
const offer = {
  shop_id: testShop.shop_id,
  offer_id: "a".repeat(64),
  plan_key: "standard_monthly",
  amount_cents: 3990,
  currency: "BRL",
  interval: "month",
  interval_count: 1,
  livemode: false,
  recovering: false,
};
function commercial(paid: boolean, status = paid ? "active" : "none") {
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
      reason: paid ? "subscribed" : "subscription_required",
      coverage_until: paid ? new Date(Date.now() + 86400000).toISOString() : null,
      cancel_at_period_end: false,
    },
  };
}
const dialog = (page: Page) => page.getByRole("dialog", { name: "Novo agendamento", exact: true });
async function setup(page: Page, role: "owner" | "barber" = "owner") {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(base + "/access", (route) =>
    route.fulfill({ json: { role, professional_id: role === "barber" ? professional.id : null } }),
  );
  await page.route(base + "/services", (route) => route.fulfill({ json: [service] }));
  const state = {
    paid: false,
    contract: "none",
    availability: [slot],
    availabilityReads: 0,
    commercialReads: 0,
    offerReads: 0,
    checkoutWrites: [] as unknown[],
    bookings: [] as { key: string; input: Record<string, unknown> }[],
    publicReads: 0,
    publications: 0,
  };
  await page.route(base + "/booking/professionals**", (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    return route.fulfill({
      json: {
        shop_name: testShop.name,
        shop_timezone: "America/Sao_Paulo",
        today: "2026-11-02",
        last_booking_date: "2027-01-31",
        professionals: [professional],
      },
    });
  });
  await page.route(base + "/booking/availability**", (route) => {
    state.availabilityReads++;
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    return route.fulfill({
      json: { shop_timezone: "America/Sao_Paulo", slots: state.availability },
    });
  });
  await page.route(base + "/commercial-status", (route) => {
    state.commercialReads++;
    return route.fulfill({ json: commercial(state.paid, state.paid ? "active" : state.contract) });
  });
  await page.route(base + "/billing/offer", (route) => {
    state.offerReads++;
    return route.fulfill({ json: { ...offer, recovering: state.checkoutWrites.length > 0 } });
  });
  await page.route(base + "/billing/checkout", (route) => {
    state.checkoutWrites.push(route.request().postDataJSON());
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    return route.fulfill({
      json: {
        attempt_id: "77777777-7777-4777-8777-777777777777",
        intent: "manual_booking",
        url: "https://checkout.stripe.com/c/pay/cs_test_existing",
        expires_at: new Date(Date.now() + 3600000).toISOString(),
        livemode: false,
      },
    });
  });
  await page.route(base + "/publication", (route) => {
    state.publications++;
    return route.abort();
  });
  const saved = {
    appointment: {
      id: "88888888-8888-4888-8888-888888888888",
      shop_name: testShop.name,
      status: "scheduled",
      service_name: service.name,
      duration_minutes: 40,
      price_cents: 5000,
      currency: "BRL",
      professional_id: professional.id,
      professional_name: professional.name,
      ...slot,
      shop_timezone: "America/Sao_Paulo",
    },
    code: "ABCDEFGH-IJKLMNOP-QRSTUVWX-YZ234567",
    replayed: false,
  };
  await page.route(publicBase + "/**", (route) => {
    if (route.request().url().includes("/appointments/lookup"))
      return route.fulfill({ json: { appointment: saved.appointment, can_cancel: true } });
    state.publicReads++;
    return route.fulfill({ status: 404, json: { error: { code: "shop_not_found" } } });
  });
  await page.route(base + "/appointments", (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    state.bookings.push({
      key: route.request().headers()["idempotency-key"],
      input: route.request().postDataJSON(),
    });
    return state.paid
      ? route.fulfill({ status: 201, json: saved })
      : route.fulfill({
          status: role === "owner" ? 402 : 403,
          json: { error: { code: "shop_subscription_required" } },
        });
  });
  return { state, saved };
}
async function fillBooking(page: Page) {
  await page.goto("/admin/" + testShop.slug);
  await page.getByRole("button", { name: "Novo agendamento", exact: true }).click();
  await dialog(page)
    .getByRole("combobox", { name: "Serviço", exact: true })
    .selectOption(service.id);
  const professionalField = dialog(page).getByRole("combobox", {
    name: "Profissional",
    exact: true,
  });
  if (await professionalField.isEnabled()) await professionalField.selectOption(professional.id);
  await dialog(page).getByRole("button", { name: "09:30", exact: true }).click();
  await dialog(page).getByLabel("Nome", { exact: true }).fill("Ana");
  await dialog(page).getByLabel("Sobrenome", { exact: true }).fill("Silva");
  await dialog(page).getByLabel("Telefone com DDD").fill("11912345678");
  await dialog(page).getByRole("button", { name: "Revisar agendamento" }).click();
}
async function submit(page: Page) {
  await dialog(page).getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
}

test("owner pays from the same booking draft and explicitly reviews before creating", async ({
  page,
}) => {
  const { state } = await setup(page);
  await fillBooking(page);
  await submit(page);
  await expect(dialog(page)).toContainText("Seus dados foram mantidos");
  await dialog(page).getByRole("button", { name: "Ver plano e assinar" }).click();
  await expect(dialog(page)).toContainText("R$ 39,90");
  expect(state.checkoutWrites).toEqual([]);
  await dialog(page)
    .getByRole("button", { name: "Preparar Checkout" })
    .evaluate((button) => {
      (button as HTMLButtonElement).click();
      (button as HTMLButtonElement).click();
    });
  const link = dialog(page).getByRole("link", { name: "Continuar na Stripe" });
  await expect(link).toHaveAttribute("target", "_blank");
  expect(state.checkoutWrites).toEqual([{ intent: "manual_booking", offer_id: offer.offer_id }]);
  state.paid = true;
  await dialog(page).getByRole("button", { name: "Conferir assinatura", exact: true }).click();
  await expect(
    dialog(page).getByRole("button", { name: "Retomar revisão do agendamento" }),
  ).toBeVisible();
  expect(state.bookings).toHaveLength(1);
  await dialog(page).getByRole("button", { name: "Retomar revisão do agendamento" }).click();
  await expect(dialog(page)).toContainText("horário reconsultado");
  await expect(dialog(page)).toContainText("Ana Silva");
  expect(state.availabilityReads).toBe(2);
  expect(state.bookings).toHaveLength(1);
  await submit(page);
  await expect(
    dialog(page).getByRole("heading", { name: "Agendamento registrado!" }),
  ).toBeVisible();
  expect(state.bookings).toHaveLength(2);
  expect(state.bookings[1].input).toEqual(state.bookings[0].input);
  expect(state.publicReads).toBe(0);
  expect(state.publications).toBe(0);
});

test("paid unpublished shop creates manually through private availability only", async ({
  page,
}) => {
  const { state } = await setup(page);
  state.paid = true;
  await fillBooking(page);
  await submit(page);
  await expect(
    dialog(page).getByRole("heading", { name: "Agendamento registrado!" }),
  ).toBeVisible();
  expect(state.publicReads).toBe(0);
  expect(state.commercialReads).toBe(0);
  expect(state.checkoutWrites).toHaveLength(0);
  expect(state.publications).toBe(0);
});

test("barber keeps the draft and asks the owner, without billing access", async ({ page }) => {
  const { state } = await setup(page, "barber");
  await fillBooking(page);
  await submit(page);
  await expect(dialog(page)).toContainText("Procure o proprietário");
  await expect(dialog(page).getByRole("button", { name: "Ver plano e assinar" })).toHaveCount(0);
  expect(state.commercialReads).toBe(0);
  expect(state.offerReads).toBe(0);
  expect(state.checkoutWrites).toHaveLength(0);
  state.paid = true;
  await dialog(page).getByRole("button", { name: "Atualizar horário e revisar" }).click();
  await expect(dialog(page)).toContainText("Ana Silva");
  await submit(page);
  await expect(
    dialog(page).getByRole("heading", { name: "Agendamento registrado!" }),
  ).toBeVisible();
  expect(state.commercialReads).toBe(0);
});

test("an occupied slot after payment preserves contact and requires another selection", async ({
  page,
}) => {
  const { state } = await setup(page);
  await fillBooking(page);
  await submit(page);
  await expect(dialog(page).getByRole("button", { name: "Ver plano e assinar" })).toBeVisible();
  state.paid = true;
  state.availability = [];
  await dialog(page).getByRole("button", { name: "Conferir assinatura", exact: true }).click();
  await dialog(page).getByRole("button", { name: "Retomar revisão do agendamento" }).click();
  await expect(dialog(page)).toContainText("horário mudou ou foi ocupado");
  expect(state.bookings).toHaveLength(1);
  state.availability = [{ starts_at: "2026-11-02T13:30:00Z", ends_at: "2026-11-02T14:30:00Z" }];
  await dialog(page).getByRole("button", { name: "Atualizar horários" }).click();
  await dialog(page).getByRole("button", { name: "10:30", exact: true }).click();
  await expect(dialog(page).getByLabel("Nome", { exact: true })).toHaveValue("Ana");
  await expect(dialog(page).getByLabel("Telefone com DDD")).toHaveValue("11912345678");
});

test("existing contract and a failed payment check never trigger another subscription or booking", async ({
  page,
}) => {
  const { state } = await setup(page);
  state.contract = "pending";
  await fillBooking(page);
  await submit(page);
  await expect(dialog(page)).toContainText("confirmação pendente");
  await expect(dialog(page).getByRole("button", { name: "Ver plano e assinar" })).toHaveCount(0);
  await page.route(base + "/commercial-status", (route) =>
    route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } }),
  );
  await dialog(page).getByRole("button", { name: "Conferir assinatura", exact: true }).click();
  await expect(dialog(page)).toContainText("temporariamente indisponível");
  await expect(
    dialog(page).getByRole("button", { name: "Confirmar agendamento", exact: true }),
  ).toBeDisabled();
  expect(state.bookings).toHaveLength(1);
  expect(state.offerReads).toBe(0);
  expect(state.checkoutWrites).toHaveLength(0);
});

test("uncertain booking retains its original key across a billing denial and recovers before availability", async ({
  page,
}) => {
  const { state, saved } = await setup(page);
  let posts = 0;
  const keys: string[] = [];
  await page.route(base + "/appointments", (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    posts++;
    keys.push(route.request().headers()["idempotency-key"]);
    if (posts === 1)
      return route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } });
    if (posts === 2)
      return route.fulfill({
        status: 402,
        json: { error: { code: "shop_subscription_required" } },
      });
    return route.fulfill({ json: { ...saved, replayed: true } });
  });
  await fillBooking(page);
  await submit(page);
  await dialog(page).getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(dialog(page)).toContainText("resultado da tentativa anterior continua incerto");
  state.paid = true;
  await dialog(page).getByRole("button", { name: "Conferir assinatura", exact: true }).click();
  await dialog(page).getByRole("button", { name: "Retomar revisão do agendamento" }).click();
  expect(state.availabilityReads).toBe(1);
  expect(posts).toBe(2);
  await dialog(page).getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(
    dialog(page).getByRole("heading", { name: "Agendamento registrado!" }),
  ).toBeVisible();
  expect(keys).toHaveLength(3);
  expect(new Set(keys).size).toBe(1);
});

test("lost Checkout response preserves the booking and retry stays explicit", async ({ page }) => {
  const { state } = await setup(page);
  let writes = 0;
  await page.route(base + "/billing/checkout", (route) => {
    writes++;
    return writes === 1 ? route.abort("failed") : route.fallback();
  });
  await fillBooking(page);
  await submit(page);
  await dialog(page).getByRole("button", { name: "Ver plano e assinar" }).click();
  await dialog(page).getByRole("button", { name: "Preparar Checkout" }).click();
  await expect(dialog(page)).toContainText("A tentativa pode ter sido registrada");
  await dialog(page).getByRole("button", { name: "Consultar condições novamente" }).click();
  await expect(dialog(page)).toContainText("R$ 39,90");
  expect(writes).toBe(1);
  expect(state.bookings).toHaveLength(1);
  await dialog(page).getByRole("button", { name: "Preparar Checkout" }).click();
  await expect(dialog(page).getByRole("link", { name: "Continuar na Stripe" })).toBeVisible();
  expect(writes).toBe(2);
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toContainText("Descartar os dados deste agendamento?");
  await dialog(page).getByRole("button", { name: "Continuar preenchendo" }).click();
  await expect(dialog(page)).toContainText("Ana Silva");
});

test("manual subscription recovery fits a narrow viewport and enlarged text", async ({ page }) => {
  await setup(page);
  await page.setViewportSize({ width: 390, height: 900 });
  await fillBooking(page);
  await submit(page);
  await dialog(page).getByRole("button", { name: "Ver plano e assinar" }).click();
  await expect(dialog(page).getByRole("button", { name: "Preparar Checkout" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: "test-results/manual-booking-subscription-390.png",
    fullPage: true,
  });
  await page.addStyleTag({ content: "html {font-size:200% !important;}" });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const layout = await dialog(page).evaluate((element) => ({
    width: element.clientWidth,
    scroll: element.scrollWidth,
    overflowing: Array.from(element.querySelectorAll<HTMLElement>("*"))
      .filter(
        (item) => item.getBoundingClientRect().right > element.getBoundingClientRect().right + 1,
      )
      .map((item) => ({
        tag: item.tagName,
        class: item.className,
        text: item.textContent?.slice(0, 45),
      }))
      .slice(0, 8),
  }));
  expect(layout.scroll, JSON.stringify(layout)).toBeLessThanOrEqual(layout.width + 1);
});
