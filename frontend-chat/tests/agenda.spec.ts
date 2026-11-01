import { test, expect } from "@playwright/test";
import { emptyAgenda, installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

test.use({ timezoneId: "Asia/Tokyo" });
const path = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug + "/appointments*";
const appointment = {
  id: "55555555-5555-4555-8555-555555555555",
  shop_name: testShop.name,
  status: "scheduled",
  service_name: "Corte reservado",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  professional_id: "66666666-6666-4666-8666-666666666666",
  professional_name: "Palma",
  starts_at: "2026-10-04T01:30:00Z",
  ends_at: "2026-10-04T02:30:00Z",
  shop_timezone: "America/Sao_Paulo",
  customer_name: "Cliente Teste",
  customer_phone: "+5511912345678",
};

test("authorized agenda uses shop dates and displays private details on mobile", async ({
  page,
}) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(path, (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    return route.fulfill({ json: { ...emptyAgenda, appointments: [appointment] } });
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/admin/" + testShop.slug);
  // The week view is the default and groups bookings by the shop's civil day.
  await expect(page.getByRole("button", { name: "Semana", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  const day = page.locator('[data-date="2026-10-03"]');
  await expect(day.locator(".agenda-list-item")).toContainText("22:30–23:30");
  await expect(day.locator(".agenda-list-item")).toContainText("Cliente Teste");
  await expect(page.locator('[data-date="2026-10-04"] .agenda-list-item')).toHaveCount(0);
  await page.screenshot({ path: "test-results/agenda-week-mobile.png", fullPage: true });
  await page.locator(".agenda-list-item").click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("+5511912345678");
  await expect(dialog).toContainText("22:30–23:30");
  await expect(dialog).toContainText("40 minutos");
  await expect(dialog).toContainText("R$ 50,00");
  await page.screenshot({ path: "test-results/agenda-details-mobile.png", fullPage: true });
  await dialog.getByRole("button", { name: "Fechar janela" }).click();
  await page.getByRole("button", { name: "Dia", exact: true }).click();
  await expect(page.locator(".agenda-list-item")).toHaveCount(1);
  await page.screenshot({ path: "test-results/agenda-day-mobile.png", fullPage: true });
  await page.getByRole("button", { name: "Ver dia 2026-10-04" }).click();
  await expect(page.getByText("Nenhum agendamento neste dia.")).toBeVisible();
  await page.getByRole("button", { name: "Mês", exact: true }).click();
  await expect(page.getByRole("heading", { name: "outubro de 2026" })).toBeVisible();
  await page.screenshot({ path: "test-results/agenda-calendar-mobile.png", fullPage: true });
  await page.getByRole("button", { name: "Ver dia 2026-10-03" }).click();
  await expect(page.getByRole("button", { name: "Dia", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.locator(".agenda-list-item")).toHaveCount(1);
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
});

test("pagination, focus refresh, cancellation and month navigation", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let cancelled = false;
  await page.route(path, (route) => {
    const query = new URL(route.request().url()).searchParams;
    if (query.get("month") === "2026-11")
      return route.fulfill({
        json: {
          ...emptyAgenda,
          month: "2026-11",
          from_date: "2026-10-26",
          until_date: "2026-12-07",
        },
      });
    if (query.has("after"))
      return route.fulfill({
        json: {
          ...emptyAgenda,
          appointments: [
            {
              ...appointment,
              id: "77777777-7777-4777-8777-777777777777",
              customer_name: "Segundo Cliente",
            },
          ],
        },
      });
    return route.fulfill({
      json: {
        ...emptyAgenda,
        appointments: [{ ...appointment, status: cancelled ? "cancelled" : "scheduled" }],
        next_cursor: cancelled ? null : "next",
      },
    });
  });
  await page.goto("/admin/" + testShop.slug);
  await page.getByRole("button", { name: "Carregar mais agendamentos" }).click();
  await expect(page.locator(".agenda-list-item")).toHaveCount(2);
  cancelled = true;
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(page.locator(".agenda-list-item")).toHaveCount(1);
  await expect(page.locator(".agenda-list-item")).toContainText("Cancelado");
  await page.getByRole("button", { name: "Mês", exact: true }).click();
  await expect(page.locator(".agenda-event")).toContainText("Cancelado");
  await page.getByRole("button", { name: "Próximo mês" }).click();
  await expect(page.getByRole("heading", { name: "novembro de 2026" })).toBeVisible();
  await expect(page.locator(".agenda-event")).toHaveCount(0);
  await page.getByRole("button", { name: "Hoje", exact: true }).click();
  await expect(page.getByRole("heading", { name: "outubro de 2026" })).toBeVisible();
  await expect(page.locator(".agenda-event")).toContainText("Cancelado");
});

test("agenda failures are not disguised as an empty calendar and retry is read-only", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let unavailable = true;
  await page.route(path, (route) => {
    expect(route.request().method()).toBe("GET");
    return route.fulfill(
      unavailable
        ? { status: 503, json: { error: { code: "temporarily_unavailable" } } }
        : { json: emptyAgenda },
    );
  });
  await page.goto("/admin/" + testShop.slug);
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "temporariamente indisponível",
  );
  await expect(page.getByText("0 agendamentos carregados", { exact: false })).toHaveCount(0);
  unavailable = false;
  await page.getByRole("button", { name: "Atualizar agenda" }).click();
  await expect(page.getByText("0 agendamentos carregados", { exact: false })).toBeVisible();
});
