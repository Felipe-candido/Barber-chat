import { test, expect } from "@playwright/test";
import { createTestSession, testEmail, testPassword } from "./supabase-fixture";

test("login sends credentials only to Supabase and the API receives only the access token", async ({
  page,
}) => {
  const session = createTestSession();
  let supabaseBody: unknown;
  let backendAuthorization = "";
  let backendBody = "";

  await page.route("https://test-project.supabase.co/auth/v1/token**", async (route) => {
    supabaseBody = route.request().postDataJSON();
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(session.response),
    });
  });
  await page.route("**/api/v1/public/shops/**/services", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: "[]" });
  });
  await page.route("http://127.0.0.1:8080/api/v1/admin/services", async (route) => {
    backendAuthorization = route.request().headers().authorization ?? "";
    backendBody = route.request().postData() ?? "";
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({
        id: "22222222-2222-4222-8222-222222222222",
        name: "Corte autenticado",
        description: "Teste",
        duration_minutes: 30,
        price_cents: 4500,
        currency: "BRL",
        active: true,
      }),
    });
  });

  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  await page.getByLabel("Senha", { exact: true }).fill(testPassword);
  await expect(page.getByLabel("Senha", { exact: true })).toHaveAttribute("type", "password");
  await page.getByRole("button", { name: "Mostrar senha", exact: true }).click();
  await expect(page.getByLabel("Senha", { exact: true })).toHaveAttribute("type", "text");
  await page.getByRole("button", { name: "Ocultar senha", exact: true }).click();
  await page.getByRole("button", { name: "Entrar no painel", exact: true }).click();

  await expect(page).toHaveURL(/\/admin$/);
  expect(supabaseBody).toMatchObject({ email: testEmail, password: testPassword });
  const stored = await page.evaluate(() => JSON.stringify({ ...localStorage }));
  expect(stored).toContain(session.accessToken);
  expect(stored).not.toContain(testPassword);

  await page.goto("/admin/servicos");
  await page.getByRole("button", { name: "Adicionar serviço" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do serviço").fill("Corte autenticado");
  await dialog.getByLabel(/Descrição/).fill("Teste");
  await dialog.getByLabel("Preço (R$)").fill("45,00");
  await dialog.getByRole("button", { name: "Adicionar serviço" }).click();
  await expect(page.getByText("Serviço “Corte autenticado” criado na API.")).toBeVisible();

  expect(backendAuthorization).toBe("Bearer " + session.accessToken);
  expect(backendBody).not.toContain(testEmail);
  expect(backendBody).not.toContain(testPassword);
  expect(backendBody).not.toContain(session.accessToken);
});

test("invalid credentials show a generic error and do not create a session", async ({ page }) => {
  await page.route("https://test-project.supabase.co/auth/v1/token**", async (route) => {
    await route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({
        code: 400,
        error_code: "invalid_credentials",
        msg: "Invalid login credentials",
      }),
    });
  });
  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill("unknown@example.com");
  await page.getByLabel("Senha", { exact: true }).fill("wrong-password");
  await page.getByRole("button", { name: "Entrar no painel" }).click();

  await expect(page).toHaveURL(/\/login$/);
  await expect(page.locator(".login-form-error")).toContainText("E-mail ou senha inválidos");
  const stored = await page.evaluate(() => JSON.stringify({ ...localStorage }));
  expect(stored).not.toContain("wrong-password");
});

test("recovery and invitation remain clearly simulated and dismissible", async ({ page }) => {
  await page.goto("/login");
  await page.getByRole("button", { name: "Esqueci minha senha" }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("E-mail da conta").fill("demo@example.com");
  await dialog.getByRole("button", { name: "Simular recuperação" }).click();
  await expect(dialog.getByRole("status")).toContainText("nenhum e-mail foi enviado");
  await dialog.getByRole("button", { name: "Voltar ao login" }).click();
  await page.getByRole("button", { name: "Saiba como funciona o convite" }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Não é necessário compartilhar uma senha");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("login fits desktop and mobile screens without client errors", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 960 });
    await page.goto("/login");
    await expect(page.getByRole("heading", { name: /Bem-vindo de volta/ })).toBeVisible();
    await expect(page.getByRole("button", { name: "Entrar no painel" })).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBeTruthy();
    await page.screenshot({ path: "test-results/login-" + width + ".png", fullPage: true });
  }
  expect(errors).toEqual([]);
});
