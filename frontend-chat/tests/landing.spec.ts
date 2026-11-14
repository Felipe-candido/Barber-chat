import { test, expect } from "@playwright/test";

test("home page is the public landing and sends barbers to sign up", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  const response = await page.goto("/");
  expect(response?.status()).toBe(200);
  await expect(page).toHaveURL("/");
  await expect(page).toHaveTitle("Na Regua · Agenda online para barbearias");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Sua barbearia na regua.");
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", "index, follow");

  const signups = page.getByRole("link", { name: /Criar conta/ });
  expect(await signups.count()).toBeGreaterThanOrEqual(3);
  for (const href of await signups.evaluateAll((links) =>
    links.map((link) => link.getAttribute("href")),
  ))
    expect(href).toBe("/cadastro");
  await expect(page.getByRole("banner").getByRole("link", { name: "Entrar" })).toHaveAttribute(
    "href",
    "/login",
  );

  // Screenshots load from this site only, within the CSP.
  const images = page.locator("main img");
  await images.first().scrollIntoViewIfNeeded();
  for (const src of await images.evaluateAll((items) =>
    items.map((item) => (item as HTMLImageElement).src),
  ))
    expect(new URL(src).origin).toBe(new URL(page.url()).origin);

  await page.getByText("Preciso pagar para começar?").click();
  await expect(page.getByText("Criar a conta e configurar a barbearia é grátis.")).toBeVisible();
  // Without NEXT_PUBLIC_TRIAL_DAYS the page shows the default price and promises no trial.
  await expect(page.locator("#preco")).toContainText("R$ 49,90/mês");
  await expect(page.locator("main")).not.toContainText("dias grátis");

  await page.getByRole("banner").getByRole("link", { name: "Criar conta" }).click();
  await expect(page).toHaveURL("/cadastro");
  expect(errors.filter((text) => /Content Security Policy/i.test(text))).toEqual([]);
});

test("WhatsApp link names a known channel and ignores unknown query text", async ({ page }) => {
  await page.goto("/?origem=tiktok");
  const link = page.getByRole("link", { name: "Chamar no WhatsApp" }).last();
  const href = (await link.getAttribute("href")) ?? "";
  expect(href.startsWith("https://wa.me/5511999999999?text=")).toBe(true);
  expect(decodeURIComponent(href.split("text=")[1])).toContain("Vi o Na Regua no TikTok");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");

  await page.goto("/?utm_source=%3Cscript%3E");
  const plain =
    (await page.getByRole("link", { name: "Chamar no WhatsApp" }).last().getAttribute("href")) ??
    "";
  expect(decodeURIComponent(plain.split("text=")[1])).toBe(
    "Oi! Quero saber mais sobre o Na Regua para a minha barbearia.",
  );
});

test("landing fits a phone screen without sideways scrolling", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/");
  const width = await page.evaluate(() => document.documentElement.scrollWidth);
  expect(width).toBeLessThanOrEqual(375);
  await expect(page.getByRole("link", { name: "Criar conta grátis" }).first()).toBeVisible();
});
