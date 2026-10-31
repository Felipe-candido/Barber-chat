import { expect, test } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, otherShop, testShop } from "./supabase-fixture";

test("product identity and login controls stay usable at mobile, tablet and desktop sizes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 960 });
    await page.goto("/login");
    await expect(page).toHaveTitle("Acesso administrativo · Barber-chat");
    await expect(page.getByRole("link", { name: "Barber-chat — acesso" })).toBeVisible();
    await expect(page.getByLabel("E-mail", { exact: true })).toBeVisible();
    const password = page.getByLabel("Senha", { exact: true });
    await password.fill("example-password-only-for-ui-test");
    await page.getByRole("button", { name: "Mostrar senha", exact: true }).click();
    await expect(password).toHaveAttribute("type", "text");
    await page.getByRole("button", { name: "Ocultar senha", exact: true }).click();
    await expect(password).toHaveAttribute("type", "password");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const target = await page
      .getByRole("button", { name: "Mostrar senha", exact: true })
      .boundingBox();
    expect(target?.height).toBeGreaterThanOrEqual(44);
    expect(target?.width).toBeGreaterThanOrEqual(44);
    await page.screenshot({ path: `test-results/foundation-login-${width}.png`, fullPage: true });
  }
  expect(errors).toEqual([]);
});

test("native dialog restores keyboard focus and invitation help describes the real flow", async ({
  page,
}) => {
  await page.goto("/login");
  const trigger = page.getByRole("button", { name: "Como obter acesso" });
  await trigger.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "Como seu acesso é liberado" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveAttribute("aria-describedby", /.+/);
  await expect(dialog).toContainText("Você receberá um convite");
  await expect(page.locator("dialog :focus")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("login colors meet text contrast and reduced motion remains available", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/login");
  const samples = await page.evaluate(() => {
    const button = document.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    const input = document.getElementById("login-email")!;
    const label = document.querySelector('label[for="login-email"]')!;
    const colors = (element: Element) => {
      const style = getComputedStyle(element);
      return { color: style.color, background: style.backgroundColor };
    };
    return {
      button: colors(button),
      input: colors(input),
      inputBorder: {
        color: getComputedStyle(input).borderTopColor,
        background: colors(input).background,
      },
      label: { color: colors(label).color, background: colors(input).background },
      reducedMotion: getComputedStyle(button).transitionDuration,
    };
  });
  function luminance(rgb: string) {
    const channels = rgb
      .match(/[\d.]+/g)!
      .slice(0, 3)
      .map(Number)
      .map((value) => {
        const channel = value / 255;
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
      });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  }
  for (const sample of [samples.button, samples.input, samples.label]) {
    const values = [luminance(sample.color), luminance(sample.background)].sort((a, b) => b - a);
    expect((values[0] + 0.05) / (values[1] + 0.05)).toBeGreaterThanOrEqual(4.5);
  }
  const border = [
    luminance(samples.inputBorder.color),
    luminance(samples.inputBorder.background),
  ].sort((a, b) => b - a);
  expect((border[0] + 0.05) / (border[1] + 0.05)).toBeGreaterThanOrEqual(3);
  expect(samples.reducedMotion.split(",").every((value) => parseFloat(value) === 0)).toBe(true);
});

test("shop selection shows only API identities without inheriting the original shop brand", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/admin");
  await expect(
    page.getByRole("heading", { name: "Escolha sua barbearia", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: `Entrar em ${testShop.name}` })).toHaveAttribute(
    "href",
    `/admin/${testShop.slug}/servicos`,
  );
  await expect(page.getByRole("link", { name: `Entrar em ${otherShop.name}` })).toHaveAttribute(
    "href",
    `/admin/${otherShop.slug}/servicos`,
  );
  await expect(page.getByRole("img", { name: /Palma/i })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: "test-results/foundation-shop-selection-320.png", fullPage: true });
});

test("logout errors remain visible until dismissed and are not reported as success", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route("https://test-project.supabase.co/auth/v1/logout**", (route) =>
    route.fulfill({
      status: 503,
      json: { message: "Test unavailable" },
    }),
  );
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: "Escolha sua barbearia" })).toBeVisible();
  await page.clock.install();
  await page.getByRole("button", { name: "Sair da conta", exact: true }).click();
  const notice = page
    .getByRole("alert")
    .filter({ has: page.getByRole("button", { name: "Fechar aviso" }) });
  await expect(notice).toContainText("Não foi possível encerrar a sessão");
  await page.clock.fastForward(10000);
  // The live region wraps a fixed-position toast, so check its visible content.
  await expect(
    notice.getByText("Não foi possível encerrar a sessão. Tente novamente.", { exact: true }),
  ).toBeVisible();
  await notice.getByRole("button", { name: "Fechar aviso" }).click();
  await expect(page.getByRole("button", { name: "Fechar aviso" })).toHaveCount(0);
});
