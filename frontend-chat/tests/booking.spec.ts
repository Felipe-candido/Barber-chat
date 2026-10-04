import { expect, Page, test } from "@playwright/test";

const service = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Corte real",
  description: "Corte completo",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  active: true,
};
const professional = { id: "22222222-2222-4222-8222-222222222222", name: "Felipe" };
const code = "ABCDEFGH-IJKLMNOP-QRSTUVWX-YZ234567";
const appointment = {
  id: "33333333-3333-4333-8333-333333333333",
  shop_name: "Barbearia de teste",
  status: "scheduled",
  service_name: "Corte real",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  professional_id: professional.id,
  professional_name: professional.name,
  starts_at: "2026-10-05T12:30:00Z",
  ends_at: "2026-10-05T13:30:00Z",
  shop_timezone: "America/Sao_Paulo",
};

async function fixture(page: Page, mode: "normal" | "network" | "conflict" = "normal") {
  const attempts: { key: string | undefined; body: unknown }[] = [];
  let cancelled = false;
  let cancels = 0;
  await page.route("http://127.0.0.1:8080/api/v1/public/shops/teste/**", async (route) => {
    const request = route.request();
    expect(request.headers().authorization).toBeUndefined();
    const headers = {
      "Access-Control-Allow-Origin": "http://127.0.0.1:3100",
      "Access-Control-Allow-Methods": "GET, POST",
      "Access-Control-Allow-Headers": "Content-Type, Idempotency-Key",
    };
    if (request.method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers });
      return;
    }
    const path = new URL(request.url()).pathname;
    let json: unknown;
    let status = 200;
    if (path.endsWith("/services")) json = [service];
    else if (path.endsWith("/professionals"))
      json = {
        shop_name: appointment.shop_name,
        shop_timezone: appointment.shop_timezone,
        today: "2026-10-05",
        last_booking_date: "2027-01-03",
        professionals: [professional],
      };
    else if (path.endsWith("/availability"))
      json = {
        shop_timezone: appointment.shop_timezone,
        slots:
          mode === "conflict" && attempts.length
            ? []
            : [{ starts_at: appointment.starts_at, ends_at: appointment.ends_at }],
      };
    else if (path.endsWith("/appointments")) {
      attempts.push({ key: request.headers()["idempotency-key"], body: request.postDataJSON() });
      if (mode === "network" && attempts.length === 1) {
        await route.abort("failed");
        return;
      }
      if (mode === "conflict") {
        json = { error: { code: "slot_unavailable" } };
        status = 409;
      } else {
        status = attempts.length === 1 ? 201 : 200;
        json = { appointment, code, replayed: attempts.length > 1 };
      }
    } else if (path.endsWith("/lookup") || path.endsWith("/cancel")) {
      expect(request.url()).not.toContain(code);
      if (request.postDataJSON().code !== code) {
        status = 404;
        json = { error: { code: "appointment_not_found" } };
      } else {
        if (path.endsWith("/cancel")) {
          cancelled = true;
          cancels++;
        }
        json = {
          appointment: { ...appointment, status: cancelled ? "cancelled" : "scheduled" },
          can_cancel: !cancelled,
        };
      }
    } else throw new Error("Unexpected booking route " + path);
    await route.fulfill({ status, json, headers });
  });
  return { attempts, cancels: () => cancels };
}

async function fillBooking(page: Page) {
  await page.goto("/b/teste");
  await page.getByRole("button", { name: /Corte real/ }).click();
  await page
    .getByRole("combobox", { name: "Profissional", exact: true })
    .selectOption(professional.id);
  await page.getByRole("button", { name: "09:30", exact: true }).click();
  await page.getByLabel("Nome", { exact: true }).fill("Ana");
  await page.getByLabel("Sobrenome", { exact: true }).fill("Silva");
  await page.getByLabel("Telefone com DDD").fill("(11) 91234-5678");
  await page.getByRole("button", { name: "Revisar agendamento" }).click();
}

test("books with real API fields, displays receipt and code, then cancels explicitly", async ({
  page,
}) => {
  const state = await fixture(page);
  await fillBooking(page);
  expect(state.attempts).toHaveLength(0);
  await page.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Agendamento registrado!" })).toBeVisible();
  await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
  await expect(
    page.getByRole("button", { name: "Cancelar agendamento", exact: true }),
  ).toBeVisible();
  expect(state.attempts[0].key).toMatch(/^[a-f0-9]{64}$/);
  expect(state.attempts[0].body).toEqual({
    service_id: service.id,
    professional_id: professional.id,
    starts_at: appointment.starts_at,
    customer: {
      first_name: "Ana",
      last_name: "Silva",
      phone: "(11) 91234-5678",
      phone_country: "BR",
    },
  });
  await page.getByRole("button", { name: "Cancelar agendamento", exact: true }).click();
  expect(state.cancels()).toBe(0);
  await page.getByRole("button", { name: "Manter agendamento" }).click();
  await page.getByRole("button", { name: "Cancelar agendamento", exact: true }).click();
  await page.getByRole("button", { name: "Sim, cancelar agendamento" }).click();
  await expect(page.getByRole("status")).toContainText("O horário foi liberado");
  expect(state.cancels()).toBe(1);
  const persisted = await page.evaluate(() =>
    JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }),
  );
  expect(persisted).not.toContain("91234");
  expect(persisted).not.toContain("Ana");
});

test("ambiguous network failure repeats the exact same key and payload", async ({ page }) => {
  const state = await fixture(page, "network");
  await fillBooking(page);
  await page.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
  await expect(page.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Corrigir meus dados" })).toBeDisabled();
  await page.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
  expect(state.attempts).toHaveLength(2);
  expect(state.attempts[0]).toEqual(state.attempts[1]);
});

test("lost slot returns to availability without a false success", async ({ page }) => {
  await fixture(page, "conflict");
  await fillBooking(page);
  await page.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("não está mais disponível");
  await expect(page.getByText("Nenhum horário livre nesta data.", { exact: false })).toBeVisible();
  await expect(page.getByText("Agendamento registrado!")).toHaveCount(0);
});

test("customer can look up a code without login, including on a narrow screen", async ({
  page,
}) => {
  await fixture(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/b/teste");
  await page.getByRole("button", { name: "Já tenho um código" }).click();
  await page.getByLabel("Código recebido").fill("wrong");
  await page.getByRole("button", { name: "Consultar agendamento", exact: true }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Código não encontrado");
  await page.getByLabel("Código recebido").fill(code);
  await page.getByRole("button", { name: "Consultar agendamento", exact: true }).click();
  await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
  await expect(page.getByText("09:30", { exact: true })).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
});
