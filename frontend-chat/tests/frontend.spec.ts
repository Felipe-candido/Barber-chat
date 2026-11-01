import { test, expect, Page } from "@playwright/test";
import {
  installSupabaseSession,
  mockAdminAccess,
  testIdentity,
  testShop,
  emptyAgenda,
} from "./supabase-fixture";

// HTTP fixtures exercise the UI contract only; they are never shipped to the application.
const sample = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Corte da API",
  description: "Retornado pelo contrato de serviços",
  duration_minutes: 30,
  price_cents: 4500,
  currency: "BRL",
  active: true,
};
async function intercept(
  page: Page,
  handler: (method: string, body: unknown) => { status?: number; json: unknown },
) {
  await page.route("http://127.0.0.1:8080/api/v1/**", async (route) => {
    const request = route.request();
    if (request.method() === "OPTIONS") {
      await route.fulfill({
        status: 204,
        headers: {
          "Access-Control-Allow-Origin": "http://127.0.0.1:3100",
          "Access-Control-Allow-Methods": "GET, POST",
          "Access-Control-Allow-Headers": "Content-Type, Authorization",
        },
      });
      return;
    }
    const path = new URL(request.url()).pathname;
    const result =
      path === "/api/v1/admin/me"
        ? { json: testIdentity }
        : path === "/api/v1/admin/shops"
          ? { json: [testShop] }
          : path.endsWith("/access")
            ? { json: { role: "owner", professional_id: null } }
            : path.startsWith("/api/v1/admin/shops/") && path.endsWith("/appointments")
              ? { json: emptyAgenda }
              : handler(request.method(), request.postData() ? request.postDataJSON() : null);
    await route.fulfill({
      status: result.status ?? 200,
      contentType: "application/json",
      headers: { "Access-Control-Allow-Origin": "http://127.0.0.1:3100" },
      body: JSON.stringify(result.json),
    });
  });
}
test("lists API data, creates with the real payload and reloads the list", async ({ page }) => {
  await installSupabaseSession(page);
  let items = [sample];
  const bodies: unknown[] = [];
  await intercept(page, (method, body) => {
    if (method === "POST") {
      bodies.push(body);
      const input = body as object;
      const created = { ...sample, ...input, id: "22222222-2222-4222-8222-222222222222" };
      items = [...items, created];
      return { status: 201, json: created };
    }
    return { json: items };
  });
  await page.goto("/admin/barbearia-do-felipe/servicos");
  await expect(page.getByRole("heading", { name: "Corte da API", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do serviço").fill("Corte novo");
  await dialog.getByLabel("Descrição").fill("Novo serviço");
  await dialog.getByLabel("Preço (R$)").fill("62,50");
  await dialog.getByLabel("Duração (minutos)").fill("1");
  await dialog.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Corte novo", exact: true })).toBeVisible();
  expect(bodies).toEqual([
    { name: "Corte novo", description: "Novo serviço", price_cents: 6250, duration_minutes: 1 },
  ]);
  await expect(page.getByRole("status")).toContainText("criado na API");
  await page.reload();
  await expect(page.getByRole("heading", { name: "Corte novo", exact: true })).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem("palma-barbearia-demo-v1"))).toBeNull();
  // Unsupported API actions are explained, not presented as unusable controls.
  await expect(page.getByRole("button", { name: "Editar serviço" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Excluir Corte novo" })).toHaveCount(0);
  await expect(
    page.getByText("Edição e exclusão ainda não estão disponíveis.", { exact: false }),
  ).toBeVisible();
});
test("empty and failed lists never restore legacy demonstration data", async ({ page }) => {
  await installSupabaseSession(page);
  let fail = true;
  await intercept(page, () =>
    fail
      ? {
          status: 503,
          json: { error: { code: "temporarily_unavailable", message: "internal details" } },
        }
      : { json: [] },
  );
  await page.addInitScript(() =>
    localStorage.setItem(
      "palma-barbearia-demo-v1",
      JSON.stringify({ services: [{ name: "Old mock" }] }),
    ),
  );
  await page.goto("/admin/barbearia-do-felipe/servicos");
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "temporariamente indisponível",
  );
  await expect(page.getByText("Old mock")).toHaveCount(0);
  fail = false;
  await page.getByRole("button", { name: "Tentar novamente" }).click();
  await expect(page.getByRole("heading", { name: "Nenhum serviço ativo" })).toBeVisible();
});
test("public chat loads its slug and requires a configured professional", async ({ page }) => {
  const requested: string[] = [];
  await page.route("http://127.0.0.1:8080/api/v1/**", async (route) => {
    requested.push(route.request().url());
    await route.fulfill({
      json: route.request().url().includes("/professionals?")
        ? {
            shop_name: "Outra barbearia",
            shop_timezone: "America/Sao_Paulo",
            today: "2026-10-05",
            last_booking_date: "2027-01-03",
            professionals: [],
          }
        : [sample],
      headers: { "Access-Control-Allow-Origin": "http://127.0.0.1:3100" },
    });
  });
  await page.goto("/b/outra-barbearia");
  await page.getByRole("button", { name: /Corte da API/ }).click();
  expect(requested[0]).toContain("/api/v1/public/shops/outra-barbearia/services");
  await expect(page.getByText("Nenhum profissional habilitado", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: /Confirmar.*agendamento/ })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /Painel|agenda/ })).toHaveCount(0);
});
for (const [status, code, text] of [
  [403, "access_denied", "Acesso não liberado"],
  [404, "shop_not_found", "Barbearia não encontrada"],
  [422, "invalid_service", "Revise o nome"],
  [500, "internal_error", "não conseguiu concluir"],
] as const) {
  test("creation handles " + status + " without losing form data", async ({ page }) => {
    await installSupabaseSession(page);
    let attempts = 0;
    await intercept(page, (method) => {
      if (method === "POST") {
        attempts++;
        return { status, json: { error: { code, message: "server error" } } };
      }
      return { json: [sample] };
    });
    await page.goto("/admin/barbearia-do-felipe/servicos");
    await page.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Nome do serviço").fill("Teste com erro");
    await dialog.getByLabel("Preço (R$)").fill("0");
    await dialog.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText(text);
    await expect(dialog.getByLabel("Nome do serviço")).toHaveValue("Teste com erro");
    expect(attempts).toBe(1);
  });
}
test("rejects malformed responses and shows a readable network error", async ({ page }) => {
  await installSupabaseSession(page);
  await intercept(page, () => ({ json: { items: [sample] } }));
  await page.goto("/admin/barbearia-do-felipe/servicos");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("retornar uma lista");
  await page.unrouteAll();
  await page.route("http://127.0.0.1:8080/api/v1/**", (route) => route.abort("failed"));
  await page.getByRole("button", { name: "Tentar novamente" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Não foi possível acessar a API",
  );
});
test("pending requests disable duplicate submissions; a failed refresh keeps creation success distinct", async ({
  page,
}) => {
  await installSupabaseSession(page);
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let created = false;
  let posts = 0;
  await page.route("http://127.0.0.1:8080/api/v1/**", async (route) => {
    if (route.request().method() === "POST") {
      posts++;
      await gate;
      created = true;
      await route.fulfill({ status: 201, json: sample });
    } else
      await route.fulfill(
        created
          ? { status: 503, json: { error: { code: "temporarily_unavailable" } } }
          : { json: [] },
      );
  });
  await mockAdminAccess(page);
  await page.goto("/admin/barbearia-do-felipe/servicos");
  await page.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do serviço").fill("Serviço");
  await dialog.getByLabel("Preço (R$)").fill("0");
  await dialog.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "Salvando…" })).toBeDisabled();
  await expect(dialog.getByLabel("Nome do serviço")).toBeDisabled();
  release();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("status")).toContainText("criado na API");
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "temporariamente indisponível",
  );
  expect(posts).toBe(1);
});
test("agenda and responsive screens display a real empty API result without mock appointments", async ({
  page,
}) => {
  // Five navigations and screenshots need a larger total budget on slower machines.
  test.setTimeout(120000);
  await installSupabaseSession(page);
  await intercept(page, () => ({ json: [sample] }));
  await page.setViewportSize({ width: 390, height: 844 });
  for (const path of [
    "/admin",
    "/admin/barbearia-do-felipe",
    "/admin/barbearia-do-felipe/servicos",
    "/b/barbearia-do-felipe",
  ]) {
    await page.goto(path);
    const title =
      path === "/admin"
        ? "Escolha sua barbearia"
        : path.endsWith("/servicos")
          ? "Serviços"
          : path.startsWith("/b/")
            ? "Reserve seu horário"
            : "Agenda";
    await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    ).toBeTruthy();
    await page.screenshot({
      path: "test-results/" + path.replaceAll("/", "-") + "-integrated-mobile.png",
      fullPage: true,
    });
  }
  await page.goto("/admin/barbearia-do-felipe");
  await expect(page.getByRole("button", { name: "Novo agendamento" })).toBeVisible();
  await expect(page.getByText("0 agendamentos carregados", { exact: false })).toBeVisible();
  await expect(page.locator(".agenda-list-item")).toHaveCount(0);
  await expect(page.getByText("Sem agendamentos")).toHaveCount(7);
});
