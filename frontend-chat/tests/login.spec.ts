import { test, expect } from "@playwright/test";

test("login preview opens the panel without saving or sending credentials", async ({ page }) => {
  const password = "fictional-preview-password";
  const email = "demo@example.com";
  const submissions: string[] = [];
  page.on("request", (request) => {
    if (request.postData()) submissions.push(request.postData()!);
  });
  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill(email);
  await page.getByLabel("Senha", { exact: true }).fill(password);
  await expect(page.getByLabel("Senha", { exact: true })).toHaveAttribute("type", "password");
  await page.getByRole("button", { name: "Mostrar senha", exact: true }).click();
  await expect(page.getByLabel("Senha", { exact: true })).toHaveAttribute("type", "text");
  await page.getByRole("button", { name: "Ocultar senha", exact: true }).click();
  await page.getByRole("button", { name: "Entrar no painel", exact: true }).click();
  await expect(page).toHaveURL(/\/admin$/);
  await expect(page.getByRole("heading", { name: /Sua agenda, em dia/ })).toBeVisible();
  const stored = await page.evaluate(() =>
    JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }),
  );
  expect(stored).not.toContain(password);
  expect(stored).not.toContain(email);
  expect(submissions.join(" ")).not.toContain(password);
  expect(submissions.join(" ")).not.toContain(email);
});

test("recovery and invitation are clearly simulated and dismissible", async ({ page }) => {
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
