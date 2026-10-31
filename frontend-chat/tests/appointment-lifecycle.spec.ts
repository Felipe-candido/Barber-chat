import { expect, test, type Page } from "@playwright/test";
import {
  emptyAgenda,
  installSupabaseSession,
  mockAdminAccess,
  testIdentity,
  testShop,
} from "./supabase-fixture";
import type {
  AppointmentStatusDetail,
  AppointmentStatusInput,
} from "../src/lib/api/appointment-status";

const base = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug;
const id = "55555555-5555-4555-8555-555555555555",
  profA = "66666666-6666-4666-8666-666666666666",
  profB = "77777777-7777-4777-8777-777777777777";
function initial(barber = false): AppointmentStatusDetail {
  return {
    appointment: {
      id,
      shop_name: testShop.name,
      status: "completed",
      service_name: "Corte",
      duration_minutes: 40,
      price_cents: 5000,
      currency: "BRL",
      professional_id: barber ? profA : profB,
      professional_name: barber ? "Seu barbeiro" : "Outro barbeiro da unidade",
      starts_at: "2026-10-03T12:30:00Z",
      ends_at: "2026-10-03T13:30:00Z",
      shop_timezone: "America/Sao_Paulo",
      customer_name: "Cliente Teste",
      customer_phone: "+5511912345678",
      version: 2,
      status_source: "automatic",
      status_reason: "Reserved interval elapsed",
      status_actor_id: null,
      updated_at: "2026-10-03T13:30:15Z",
    },
    history: [
      {
        version: 2,
        previous_status: "scheduled",
        status: "completed",
        source: "automatic",
        actor_id: null,
        reason: "Reserved interval elapsed",
        occurred_at: "2026-10-03T13:30:15Z",
      },
    ],
    history_truncated: false,
    replayed: false,
    result_version: 2,
  };
}
function changed(
  current: AppointmentStatusDetail,
  body: AppointmentStatusInput,
): AppointmentStatusDetail {
  const version = current.appointment.version + 1,
    at = "2026-10-31T15:00:00Z";
  return {
    appointment: {
      ...current.appointment,
      status: body.status,
      version,
      status_source: "staff",
      status_reason: body.reason,
      status_actor_id: testIdentity.user_id,
      updated_at: at,
    },
    history: [
      {
        version,
        previous_status: current.appointment.status,
        status: body.status,
        source: "staff",
        actor_id: testIdentity.user_id,
        reason: body.reason,
        occurred_at: at,
      },
      ...current.history,
    ],
    history_truncated: false,
    replayed: false,
    result_version: version,
  };
}
async function setup(page: Page, barber = false) {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  if (barber)
    await page.route(base + "/access", (route) =>
      route.fulfill({ json: { role: "barber", professional_id: profA } }),
    );
  let current = initial(barber);
  const writes: { key: string; body: AppointmentStatusInput }[] = [];
  await page.route(base + "/appointments*", (route) =>
    route.fulfill({ json: { ...emptyAgenda, appointments: [current.appointment] } }),
  );
  await page.route(base + "/appointments/" + id, (route) => route.fulfill({ json: current }));
  return {
    session,
    writes,
    get current() {
      return current;
    },
    set current(value: AppointmentStatusDetail) {
      current = value;
    },
  };
}
async function open(page: Page) {
  await page.goto("/admin/" + testShop.slug);
  await page.locator(".agenda-list-item").click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Nova situação")).toBeVisible();
  return dialog;
}

test("main owner account corrects any barber and preserves the automatic audit", async ({
  page,
}) => {
  const app = await setup(page);
  await page.route(base + "/appointments/" + id + "/status", (route) => {
    const r = route.request();
    expect(r.headers().authorization).toBe("Bearer " + app.session.accessToken);
    expect(r.headers()["idempotency-key"]).toMatch(/^[a-f0-9]{64}$/);
    const body = r.postDataJSON();
    expect(body).toEqual({
      status: "no_show",
      expected_version: 2,
      reason: "Cliente não compareceu",
    });
    app.writes.push({ key: r.headers()["idempotency-key"], body });
    app.current = changed(app.current, body);
    return route.fulfill({ json: app.current });
  });
  const dialog = await open(page);
  await expect(dialog).toContainText("Outro barbeiro da unidade");
  await expect(dialog).toContainText("Como proprietário, você pode gerenciar qualquer atendimento");
  await expect(dialog).toContainText("Concluído automaticamente");
  await dialog.getByLabel("Nova situação").selectOption("no_show");
  await dialog.getByLabel("Motivo da alteração").fill("Cliente não compareceu");
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator(".agenda-list-item")).toContainText("Não compareceu");
  expect(app.writes).toHaveLength(1);
  await page.locator(".agenda-list-item").click();
  await page.getByRole("dialog").locator("summary", { hasText: "Histórico da situação" }).click();
  await expect(page.getByRole("dialog")).toContainText("Horário reservado encerrado.");
  await expect(page.getByRole("dialog")).toContainText("Cliente não compareceu");
});

test("barber corrects own appointment and automatic processing never runs in browser", async ({
  page,
}) => {
  const app = await setup(page, true);
  await page.route(base + "/appointments/" + id + "/status", (route) => {
    const r = route.request();
    app.writes.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
    app.current = changed(app.current, r.postDataJSON());
    return route.fulfill({ json: app.current });
  });
  const dialog = await open(page);
  expect(app.writes).toHaveLength(0);
  await expect(dialog).toContainText("Você pode gerenciar os atendimentos do seu próprio barbeiro");
  await dialog.getByLabel("Nova situação").selectOption("cancelled");
  await dialog.getByLabel("Motivo da alteração").fill("Cancelamento registrado depois");
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog).toHaveCount(0);
  expect(app.writes).toHaveLength(1);
  expect(app.writes[0].body).not.toHaveProperty("professional_id");
  expect(app.writes[0].body).not.toHaveProperty("actor_id");
});

test("lost correction response survives reload and replays exact body and key", async ({
  page,
}) => {
  const app = await setup(page);
  await page.route(base + "/appointments/" + id + "/status", (route) => {
    const r = route.request();
    app.writes.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
    if (app.writes.length === 1) {
      app.current = changed(app.current, r.postDataJSON());
      return route.fulfill({
        status: 503,
        json: { error: { code: "appointment_status_unavailable" } },
      });
    }
    return route.fulfill({ json: { ...app.current, replayed: true } });
  });
  let dialog = await open(page);
  await dialog.getByLabel("Nova situação").selectOption("no_show");
  await dialog.getByLabel("Motivo da alteração").fill("Ausência confirmada");
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await page.reload();
  await page.locator(".agenda-list-item").click();
  dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Nova situação")).toBeDisabled();
  await expect(dialog.getByLabel("Nova situação")).toHaveValue("no_show");
  expect(app.writes).toHaveLength(1);
  await dialog.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(dialog).toHaveCount(0);
  expect(app.writes).toHaveLength(2);
  expect(app.writes[1]).toEqual(app.writes[0]);
});

test("version conflict reads the latest state before another explicit correction", async ({
  page,
}) => {
  const app = await setup(page);
  let first = true;
  await page.route(base + "/appointments/" + id + "/status", (route) => {
    const r = route.request();
    app.writes.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
    if (first) {
      first = false;
      app.current = changed(app.current, {
        status: "cancelled",
        expected_version: 2,
        reason: "Another operator",
      });
      return route.fulfill({
        status: 409,
        json: { error: { code: "appointment_version_changed" } },
      });
    }
    app.current = changed(app.current, r.postDataJSON());
    return route.fulfill({ json: app.current });
  });
  const dialog = await open(page);
  await dialog.getByLabel("Nova situação").selectOption("no_show");
  await dialog.getByLabel("Motivo da alteração").fill("Conferido com o barbeiro");
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog.getByText(/O atendimento mudou/)).toBeVisible();
  expect(app.writes).toHaveLength(1);
  await dialog.getByRole("button", { name: "Atualizar detalhes" }).click();
  await expect(dialog.getByLabel("Motivo da alteração")).toHaveValue("Conferido com o barbeiro");
  await dialog.getByLabel("Nova situação").selectOption("no_show");
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog).toHaveCount(0);
  expect(app.writes[1].body.expected_version).toBe(3);
  expect(app.writes[1].key).not.toBe(app.writes[0].key);
});

test("revoked or other-professional detail cannot mount status controls", async ({ page }) => {
  await setup(page, true);
  await page.route(base + "/appointments/" + id, (route) =>
    route.fulfill({ status: 403, json: { error: { code: "access_denied" } } }),
  );
  await page.goto("/admin/" + testShop.slug);
  await page.locator(".agenda-list-item").click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(/Acesso não liberado/)).toBeVisible();
  await expect(dialog.getByLabel("Nova situação")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "Salvar situação" })).toHaveCount(0);
});

test("early completion is rejected without pretending service was performed", async ({ page }) => {
  const app = await setup(page);
  app.current = {
    ...app.current,
    appointment: {
      ...app.current.appointment,
      status: "scheduled",
      version: 1,
      status_source: "booking",
      status_reason: "",
      updated_at: "2026-10-03T12:00:00Z",
    },
    history: [],
    result_version: 1,
  };
  await page.route(base + "/appointments/" + id + "/status", (route) => {
    app.writes.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    return route.fulfill({
      status: 409,
      json: { error: { code: "appointment_transition_too_early" } },
    });
  });
  const dialog = await open(page);
  await dialog.getByLabel("Nova situação").selectOption("completed");
  await dialog.getByLabel("Motivo da alteração").fill("Conferência");
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog.getByText(/Ausência só pode ser registrada após o início/)).toBeVisible();
  await expect(dialog.getByLabel("Motivo da alteração")).toHaveValue("Conferência");
  expect(app.writes).toHaveLength(1);
  await expect(dialog).toContainText("Agendado");
});

test("pending status mutation resists Escape and focus refresh, with mobile keyboard layout", async ({
  page,
}) => {
  const app = await setup(page);
  let release!: () => void;
  const waiting = new Promise<void>((resolve) => (release = resolve));
  await page.route(base + "/appointments/" + id + "/status", async (route) => {
    app.writes.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    await waiting;
    app.current = changed(app.current, route.request().postDataJSON());
    await route.fulfill({ json: app.current });
  });
  const dialog = await open(page);
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 920 });
    expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await dialog.getByLabel("Nova situação").selectOption("no_show");
  await dialog.getByLabel("Motivo da alteração").fill("Ausência");
  await page.screenshot({ path: "test-results/appointment-status-mobile.png", fullPage: true });
  await dialog.getByRole("button", { name: "Salvar situação" }).click();
  await expect(dialog.getByLabel("Nova situação")).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(dialog).toBeVisible();
  expect(app.writes).toHaveLength(1);
  release();
  await expect(dialog).toHaveCount(0);
});
