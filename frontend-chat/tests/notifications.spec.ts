import { test, expect } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop, emptyAgenda } from "./supabase-fixture";

const base = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug;
const settings = {
  enabled: false,
  delivery_mode: "request",
  reminder_lead_minutes: 1440,
  version: 1,
  delivery_available: true,
  automatic_available: false,
};
const connection = {
  state: "disconnected",
  desired_connected: false,
  stale: false,
  provider_configured: true,
  verified_at: "2026-11-01T12:00:00Z",
  last_connected_at: null,
  error_code: "",
};
const appointment = {
  id: "55555555-5555-4555-8555-555555555555",
  shop_name: testShop.name,
  status: "scheduled",
  service_name: "Corte",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  professional_id: "66666666-6666-4666-8666-666666666666",
  professional_name: "Palma",
  starts_at: "2026-10-03T12:30:00Z",
  ends_at: "2026-10-03T13:30:00Z",
  shop_timezone: "America/Sao_Paulo",
  customer_name: "Cliente Teste",
  customer_phone: "+5511912345678",
};

test("owner connects with private temporary QR and saves manual preferences", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(base + "/settings", (route) =>
    route.fulfill({
      json: {
        ...testShop,
        timezone: "America/Sao_Paulo",
        phone: "",
        description: "",
        address: null,
        version: 1,
      },
    }),
  );
  await page.route(base + "/notifications/settings", (route) =>
    route.fulfill({
      json:
        route.request().method() === "PUT"
          ? { ...settings, ...route.request().postDataJSON(), version: 2 }
          : settings,
    }),
  );
  await page.route(base + "/whatsapp/connection", (route) => route.fulfill({ json: connection }));
  const png =
    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aR4QAAAAASUVORK5CYII=";
  let connects = 0;
  await page.route(base + "/whatsapp/connect", (route) => {
    connects++;
    return route.fulfill({
      json: {
        connection: { ...connection, state: "qr_required", desired_connected: true },
        qr: { png, discard_after: new Date(Date.now() + 1500).toISOString() },
      },
    });
  });
  await page.goto(`/admin/${testShop.slug}/configuracoes`);
  const section = page.getByRole("region", { name: "WhatsApp da barbearia" });
  await expect(section.getByText("Desconectado", { exact: true })).toBeVisible();
  await section.getByRole("button", { name: "Conectar WhatsApp", exact: true }).click();
  await expect(section.getByRole("img", { name: /QR code/ })).toBeVisible();
  expect(connects).toBe(1);
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain("iVBORw0KGgo");
  await expect(section.getByRole("img", { name: /QR code/ })).toHaveCount(0, { timeout: 5000 });
  await expect(section.getByText(/O QR expirou/)).toBeVisible();
  await section.getByRole("checkbox").check();
  await section.getByRole("button", { name: "Salvar preferências de WhatsApp" }).click();
  await expect(section.getByText(/Preferências de WhatsApp salvas/)).toBeVisible();
  await expect(section.getByLabel("Forma de enviar o lembrete")).toHaveValue("request");
  await page.screenshot({ path: "test-results/whatsapp-settings.png", fullPage: true });
});

test("manual reminder is sent once and uncertain result blocks repetition", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(base + "/appointments*", (route) =>
    route.fulfill({ json: { ...emptyAgenda, appointments: [appointment] } }),
  );
  let sends = 0;
  const reminder = {
    kind: "booking_reminder",
    state: "ready",
    reason: "",
    mode: "request",
    due_at: "2026-10-01T12:30:00Z",
    expires_at: "2026-10-03T12:29:00Z",
    can_send: true,
  };
  await page.route(base + "/appointments/" + appointment.id + "/notifications", (route) =>
    route.fulfill({
      json: {
        delivery_available: true,
        messages: [{ ...reminder, state: sends ? "unknown" : "ready", can_send: !sends }],
      },
    }),
  );
  await page.route(
    base + "/appointments/" + appointment.id + "/notifications/reminder",
    (route) => {
      sends++;
      return route.fulfill({ json: { state: "unknown", reason: "transport_unknown" } });
    },
  );
  await page.goto(`/admin/${testShop.slug}`);
  await page.locator(".agenda-list-item").first().click();
  const section = page.getByRole("region", { name: "Mensagens de WhatsApp do atendimento" });
  await section.getByRole("button", { name: "Enviar lembrete", exact: true }).click();
  await expect(
    section.getByText(
      "A resposta não comprova se a mensagem chegou. A repetição automática está bloqueada.",
    ),
  ).toBeVisible();
  await expect(section.getByRole("button", { name: "Enviar lembrete", exact: true })).toHaveCount(
    0,
  );
  await section.getByRole("button", { name: "Conferir envios" }).click();
  expect(sends).toBe(1);
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
  await page.screenshot({ path: "test-results/whatsapp-appointment-mobile.png", fullPage: true });
});
