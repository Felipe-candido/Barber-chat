import { expect, test, type Page } from "@playwright/test";
import { emptyAgenda, installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

test.use({ timezoneId: "Asia/Tokyo" });
const panel = "/admin/" + testShop.slug;
const appointment = {
  id: "55555555-5555-4555-8555-555555555555",
  shop_name: testShop.name,
  status: "scheduled",
  service_name: "Corte reservado",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  professional_id: "66666666-6666-4666-8666-666666666666",
  professional_name: "Barbeiro Teste",
  starts_at: "2026-10-04T01:30:00Z",
  ends_at: "2026-10-04T02:30:00Z",
  shop_timezone: "America/Sao_Paulo",
  customer_name: "Cliente Teste",
  customer_phone: "+5511912345678",
};
async function mockPanel(page: Page) {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route("**/api/v1/admin/shops/*/services", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/v1/admin/shops/*/settings", (route) =>
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
  await page.route("**/api/v1/admin/shops/*/appointments*", (route) =>
    route.fulfill({ json: { ...emptyAgenda, appointments: [appointment] } }),
  );
}

test("desktop navigation identifies the current section and routes sharing through verified settings", async ({
  page,
}) => {
  await mockPanel(page);
  await page.goto(panel);
  await expect(page.getByRole("heading", { name: "Agenda", exact: true })).toBeVisible();
  const nav = page.getByRole("navigation", { name: "Navegação principal" });
  await expect(nav.getByRole("link", { name: "Agenda", exact: true })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect(nav.getByRole("link", { name: "Barbeiros", exact: true })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Navegação rápida" })).toBeHidden();
  await expect(page.getByRole("link", { name: "Gerenciar link de agendamento" })).toHaveAttribute(
    "href",
    panel + "/configuracoes#publication",
  );
  await expect(page.getByRole("button", { name: "Copiar link do chat" })).toHaveCount(0);
  await page.screenshot({ path: "test-results/admin-agenda-desktop.png", fullPage: true });
  await nav.getByRole("link", { name: "Serviços", exact: true }).click();
  await expect(
    page.getByRole("navigation").getByRole("link", { name: "Serviços", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await expect(page.locator(".topbar")).toContainText(testShop.name);
});

test("mobile navigation contains keyboard focus, closes with Escape and restores the trigger", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await mockPanel(page);
  await page.goto(panel);
  // The open modal makes the underlying trigger inert; its DOM attributes still update.
  const trigger = page.locator('nav[aria-label="Navegação rápida"] button');
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("navigation", { name: "Navegação principal" })).toHaveCount(0);
  await trigger.focus();
  await page.keyboard.press("Enter");
  const drawer = page.getByRole("dialog", { name: "Menu do painel" });
  await expect(drawer).toBeVisible();
  expect(await drawer.evaluate((element) => element.matches(":modal"))).toBe(true);
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(drawer.getByRole("button", { name: "Fechar menu" })).toBeFocused();
  for (let index = 0; index < 10; index++) {
    await page.keyboard.press("Tab");
    expect(await drawer.evaluate((element) => element.contains(document.activeElement))).toBe(true);
  }
  await drawer.getByRole("button", { name: "Fechar menu" }).focus();
  await page.keyboard.press("Shift+Tab");
  await expect(drawer.getByRole("link", { name: "Gerenciar link de agendamento" })).toBeFocused();
  await page.screenshot({ path: "test-results/admin-navigation-320.png", fullPage: true });
  await page.keyboard.press("Escape");
  await expect(drawer).not.toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(page.locator("body")).not.toHaveCSS("overflow", "hidden");
  await trigger.click();
  await drawer.getByRole("link", { name: "Serviços", exact: true }).click();
  await expect(page).toHaveURL(panel + "/servicos");
  await expect(drawer).not.toBeVisible();
  await trigger.click();
  await drawer.getByRole("link", { name: "Gerenciar link de agendamento" }).click();
  await expect(drawer).not.toBeVisible();
  await expect(page).toHaveURL(panel + "/configuracoes#publication");
});

test("resizing an open mobile menu releases the modal before displaying desktop navigation", async ({
  page,
}) => {
  await page.setViewportSize({ width: 768, height: 1000 });
  await mockPanel(page);
  await page.goto(panel);
  await page.getByRole("button", { name: "Mais", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Menu do painel" })).toBeVisible();
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect(page.getByRole("dialog", { name: "Menu do painel" })).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Navegação principal" })).toHaveCount(1);
  await expect(page.locator("body")).not.toHaveCSS("overflow", "hidden");
});

test("barber navigation never renders owner-only destinations", async ({ page }) => {
  await mockPanel(page);
  await page.route("**/api/v1/admin/shops/*/access", (route) =>
    route.fulfill({ json: { role: "barber", professional_id: appointment.professional_id } }),
  );
  await page.goto(panel);
  await expect(page.getByRole("heading", { name: "Agenda", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Barbeiros", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Contas de acesso", exact: true })).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Mais", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: "Menu do painel" });
  await expect(drawer.getByRole("link", { name: "Agenda", exact: true })).toBeVisible();
  await expect(drawer.getByRole("link", { name: "Barbeiros", exact: true })).toHaveCount(0);
  await expect(drawer.getByRole("link", { name: "Contas de acesso", exact: true })).toHaveCount(0);
});

test("agenda opens on the week and switches to day and month at every width", async ({ page }) => {
  test.setTimeout(120000);
  await mockPanel(page);
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(panel);
    await expect(page.locator(".agenda-list-item")).toHaveCount(1);
    await expect(page.getByRole("button", { name: "Novo agendamento" })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: "test-results/admin-agenda-" + width + ".png", fullPage: true });
    await page.getByRole("button", { name: "Mês", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Ver dia 2026-10-03", exact: true }),
    ).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: "test-results/admin-agenda-month-" + width + ".png",
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "Dia", exact: true }).click();
  await page.getByRole("button", { name: "Próximo dia" }).click();
  await expect(page.getByText("Nenhum agendamento neste dia.")).toBeVisible();
  await page.getByRole("button", { name: "Hoje", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Ver dia " + emptyAgenda.today, exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator(".agenda-list-item")).toHaveCount(1);
  await page.locator(".agenda-list-item").click();
  const details = page.getByRole("dialog", { name: "Detalhes do agendamento" });
  await expect(details).toContainText("22:30–23:30");
  await expect(details).toContainText("40 minutos");
});

test("an unloaded page cannot be presented as an empty day", async ({ page }) => {
  await mockPanel(page);
  await page.route("**/api/v1/admin/shops/*/appointments*", (route) =>
    route.fulfill({ json: { ...emptyAgenda, appointments: [appointment], next_cursor: "next" } }),
  );
  await page.goto(panel);
  await page.getByRole("button", { name: "Dia", exact: true }).click();
  await page.getByRole("button", { name: "Ver dia 2026-10-04" }).click();
  await expect(
    page.getByText("Nenhum agendamento neste dia entre as reservas carregadas."),
  ).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "Esta lista ainda é parcial" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Carregar mais agendamentos" })).toBeVisible();
});
