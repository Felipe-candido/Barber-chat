import { expect, test, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop, otherShop } from "./supabase-fixture";

async function passwordSession(page: Page) {
  const session = await installSupabaseSession(page);
  const writes: unknown[] = [];
  const state = { reject: false };
  await page.route("https://test-project.supabase.co/auth/v1/user", async (route) => {
    if (route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON());
      if (state.reject)
        return route.fulfill({ status: 422, json: { message: "Private provider detail" } });
    }
    return route.fulfill({ json: session.response.user });
  });
  let apiWrites = 0;
  await page.route("http://127.0.0.1:8080/**", async (route) => {
    if (route.request().method() !== "GET") apiWrites++;
    await route.fulfill({ status: 401, json: { error: { code: "authentication_required" } } });
  });
  return { session, writes, state, apiWrites: () => apiWrites };
}

test("password setup has associated field errors and never sends mismatched passwords", async ({
  page,
}) => {
  const state = await passwordSession(page);
  await page.setViewportSize({ width: 320, height: 960 });
  await page.goto("/definir-senha");
  const password = page.getByLabel("Nova senha", { exact: true });
  const confirmation = page.getByLabel("Confirmar senha", { exact: true });
  await password.fill("senha-forte-exemplo-123!");
  await confirmation.fill("outra-senha-forte-123!");
  await confirmation.evaluate((element) => {
    const target = window as Window & { qaSubmitCount?: number };
    target.qaSubmitCount = 0;
    element.closest("form")!.addEventListener("submit", () => {
      target.qaSubmitCount!++;
    });
  });
  await page.getByRole("button", { name: "Salvar senha" }).click();
  expect(
    await page.evaluate(() => (window as Window & { qaSubmitCount?: number }).qaSubmitCount),
  ).toBe(1);
  await expect(confirmation).toHaveAttribute("aria-invalid", "true");
  await expect(confirmation).toBeFocused();
  expect(
    await confirmation.evaluate((element) =>
      element
        .getAttribute("aria-describedby")!
        .split(" ")
        .map((id) => document.getElementById(id)?.textContent)
        .join(" "),
    ),
  ).toContain("Repita a mesma senha");
  expect(state.writes).toEqual([]);
  await page.screenshot({ path: "test-results/final-password-validation-320.png", fullPage: true });
  await confirmation.fill("senha-forte-exemplo-123!");
  await page.getByRole("button", { name: "Mostrar nova senha" }).click();
  await expect(password).toHaveAttribute("type", "text");
  await page.getByRole("button", { name: "Ocultar nova senha" }).click();
  await page.getByRole("button", { name: "Salvar senha" }).click();
  await expect(page.getByRole("status")).toContainText("Senha definida");
  // Supabase may add its own PKCE fields; the app contract is one password update.
  expect(state.writes).toHaveLength(1);
  expect(state.writes[0]).toMatchObject({ password: "senha-forte-exemplo-123!" });
  expect(state.apiWrites()).toBe(0);
  const stored = await page.evaluate(() =>
    JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }),
  );
  expect(stored).not.toContain("senha-forte-exemplo-123!");
});

test("password rejection keeps values without exposing provider details", async ({ page }) => {
  const state = await passwordSession(page);
  state.state.reject = true;
  await page.goto("/definir-senha");
  await page.getByLabel("Nova senha", { exact: true }).fill("senha-forte-exemplo-123!");
  await page.getByLabel("Confirmar senha", { exact: true }).fill("senha-forte-exemplo-123!");
  await page.getByRole("button", { name: "Salvar senha" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Não foi possível definir a senha",
  );
  await expect(page.getByLabel("Nova senha", { exact: true })).toHaveValue(
    "senha-forte-exemplo-123!",
  );
  await expect(page.getByText("Private provider detail")).toHaveCount(0);
  expect(state.writes).toHaveLength(1);
});

test("pending password change disables duplicate submissions and the return link", async ({
  page,
}) => {
  const state = await passwordSession(page);
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let puts = 0;
  await page.route("https://test-project.supabase.co/auth/v1/user", async (route) => {
    if (route.request().method() === "PUT") {
      puts++;
      await gate;
    }
    await route.fulfill({ json: state.session.response.user });
  });
  try {
    await page.goto("/definir-senha");
    await page.getByLabel("Nova senha", { exact: true }).fill("senha-forte-exemplo-123!");
    await page.getByLabel("Confirmar senha", { exact: true }).fill("senha-forte-exemplo-123!");
    await page.getByRole("button", { name: "Salvar senha" }).click();
    await expect(page.getByRole("button", { name: "Salvando…" })).toBeDisabled();
    await expect(page.getByLabel("Nova senha", { exact: true })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Mostrar nova senha" })).toBeDisabled();
    await expect(page.getByRole("link", { name: "Voltar ao login" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(puts).toBe(1);
    release();
    await expect(page.getByRole("status")).toContainText("Senha definida");
    expect(puts).toBe(1);
  } finally {
    release();
  }
});

test("missing invitation does not show password controls and offers explicit recovery", async ({
  page,
}) => {
  await page.goto("/definir-senha");
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Não foi possível validar o convite",
  );
  await expect(page.getByLabel("Nova senha", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Verificar convite novamente" })).toBeVisible();
  await page.getByRole("button", { name: "Verificar convite novamente" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Confira o link recebido");
  await expect(page.getByRole("link", { name: "Voltar ao login" })).toHaveAttribute(
    "href",
    "/login",
  );
});

test("auxiliary pages fit mobile, tablet, desktop and enlarged text", async ({ page }) => {
  await passwordSession(page);
  for (const path of ["/definir-senha", "/chat"]) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    if (path === "/definir-senha")
      await expect(page.getByLabel("Nova senha", { exact: true })).toBeVisible();
    for (const width of [320, 390, 768, 1440]) {
      await page.setViewportSize({ width, height: 960 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({
        path: `test-results/final-${path.slice(1)}-${width}.png`,
        fullPage: true,
      });
    }
    await page.setViewportSize({ width: 320, height: 960 });
    await page.evaluate(() => {
      document.documentElement.style.fontSize = "200%";
    });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => {
      document.documentElement.style.fontSize = "";
    });
  }
});

test("unknown shop and rejected shop access remain closed with clear recovery actions", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let privateCalls = 0;
  await page.route("**/api/v1/admin/shops/*/services", (route) => {
    privateCalls++;
    return route.fulfill({ json: [] });
  });
  await page.setViewportSize({ width: 320, height: 960 });
  await page.goto(`/admin/${otherShop.slug}/servicos`);
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Barbearia não disponível");
  expect(privateCalls).toBe(0);
  await page.screenshot({ path: "test-results/final-shop-unavailable-320.png", fullPage: true });
  await page.route(`**/api/v1/admin/shops/${testShop.slug}/access`, (route) =>
    route.fulfill({ status: 403, json: { error: { code: "access_denied" } } }),
  );
  await page.goto(`/admin/${testShop.slug}/servicos`);
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Não foi possível verificar seu acesso",
  );
  await expect(page.getByRole("button", { name: "Verificar novamente" })).toBeVisible();
  expect(privateCalls).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("modal keyboard cycling contains focus, skips disabled fields and restores the trigger", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route("**/api/v1/admin/shops/*/services", (route) => route.fulfill({ json: [] }));
  await page.goto(`/admin/${testShop.slug}/servicos`);
  const trigger = page.getByRole("button", { name: "Adicionar serviço", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Adicionar serviço", exact: true });
  await expect(dialog).toHaveAttribute("aria-modal", "true");
  const first = dialog.getByRole("button", { name: "Fechar janela" });
  const last = dialog.getByRole("button", { name: "Adicionar serviço", exact: true });
  await first.focus();
  await page.keyboard.press("Shift+Tab");
  await expect(last).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(first).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("the browser icon belongs to the product, not the original shop", async ({ page }) => {
  await page.goto("/login");
  const hrefs = await page
    .locator('link[rel="icon"]')
    .evaluateAll((elements) => elements.map((element) => element.getAttribute("href")));
  expect(hrefs.some((href) => href?.includes("icon.svg"))).toBe(true);
  expect(hrefs.some((href) => href?.includes("icon.png"))).toBe(false);
  const response = await page.request.get(hrefs.find((href) => href?.includes("icon.svg"))!);
  expect(response.ok()).toBe(true);
  expect(response.headers()["content-type"]).toContain("image/svg+xml");
  expect(await response.text()).toContain("Barber-chat");
});
