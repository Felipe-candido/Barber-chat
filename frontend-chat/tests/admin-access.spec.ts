import { test, expect } from "@playwright/test";
import {
  installSupabaseSession,
  mockAdminAccess,
  testShop,
  otherShop,
  testIdentity,
  createTestSession,
} from "./supabase-fixture";

const service = {
  id: "55555555-5555-4555-8555-555555555555",
  name: "Corte Centro",
  description: "",
  duration_minutes: 30,
  price_cents: 4000,
  currency: "BRL",
  active: true,
};

test("selects and switches units, restores a direct route and logs out", async ({ page }) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  const requests: string[] = [];
  await page.route("**/api/v1/admin/shops/*/services", async (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    requests.push(new URL(route.request().url()).pathname);
    await route.fulfill({ json: route.request().url().includes(otherShop.slug) ? [service] : [] });
  });
  await page.route("https://test-project.supabase.co/auth/v1/logout**", (route) =>
    route.fulfill({ status: 204 }),
  );
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: /Escolha sua barbearia/ })).toBeVisible();
  await page.getByRole("link", { name: "Entrar em " + otherShop.name }).click();
  await expect(page.getByRole("heading", { name: service.name, exact: true })).toBeVisible();
  await expect(page.locator(".topbar")).toContainText(otherShop.name);
  await page.reload();
  await expect(page.getByRole("heading", { name: service.name, exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Trocar barbearia" }).click();
  await page.getByRole("link", { name: "Entrar em " + testShop.name }).click();
  await expect(page.getByRole("heading", { name: "Nenhum serviço ativo" })).toBeVisible();
  await expect(page.getByRole("heading", { name: service.name, exact: true })).toHaveCount(0);
  expect(requests.at(-1)).toBe("/api/v1/admin/shops/" + testShop.slug + "/services");
  await expect(page.getByRole("link", { name: "Gerenciar link de agendamento" })).toHaveAttribute(
    "href",
    "/admin/" + testShop.slug + "/configuracoes#publication",
  );
  await page.getByRole("button", { name: "Sair da conta" }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect(await page.evaluate(() => localStorage.getItem("sb-test-project-auth-token"))).toBeNull();
  await page.goto("/admin/" + otherShop.slug + "/servicos");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByText(service.name, { exact: true })).toHaveCount(0);
});

test("does not open any shop automatically when the list is empty", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page, []);
  let servicesCalls = 0;
  await page.route("**/api/v1/admin/shops/*/services", (route) => {
    servicesCalls++;
    return route.fulfill({ json: [] });
  });
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: "Nenhuma barbearia vinculada" })).toBeVisible();
  await expect(page.getByRole("link", { name: /^Entrar em/ })).toHaveCount(0);
  expect(servicesCalls).toBe(0);
});

test("typing another slug does not mount the administrative catalog", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let servicesCalls = 0;
  await page.route("**/api/v1/admin/shops/*/services", (route) => {
    servicesCalls++;
    return route.fulfill({ json: [service] });
  });
  await page.goto("/admin/" + otherShop.slug + "/servicos");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Barbearia não disponível");
  await expect(page.getByRole("button", { name: "Adicionar serviço" })).toHaveCount(0);
  expect(servicesCalls).toBe(0);
});

for (const code of ["authentication_required", "invalid_access_token"]) {
  test("a 401 on me returns to login: " + code, async ({ page }) => {
    if (code === "invalid_access_token") await installSupabaseSession(page);
    await page.route("**/api/v1/admin/me", (route) =>
      route.fulfill({ status: 401, json: { error: { code } } }),
    );
    await page.goto("/admin/" + testShop.slug + "/servicos");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("button", { name: "Adicionar serviço" })).toHaveCount(0);
  });
}

test("local access denial is not treated as a successful Supabase login", async ({ page }) => {
  await installSupabaseSession(page);
  await page.route("**/api/v1/admin/me", (route) =>
    route.fulfill({ status: 403, json: { error: { code: "access_denied" } } }),
  );
  await page.goto("/admin");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Acesso não liberado");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("proprietário");
  await expect(page.getByRole("heading", { name: /Escolha sua barbearia/ })).toHaveCount(0);
});

test("invalid identity or duplicate shops fail closed", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, testShop]);
  await page.goto("/admin");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("incompatíveis");
  await page.route("**/api/v1/admin/me", (route) =>
    route.fulfill({ json: { ...testIdentity, user_id: "invalid" } }),
  );
  await page.getByRole("button", { name: "Tentar novamente" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("incompatíveis");
});

test("public catalog never sends the existing administrative session", async ({ page }) => {
  await installSupabaseSession(page);
  let authorization: string | undefined;
  await page.route("**/api/v1/public/shops/*/services", (route) => {
    authorization = route.request().headers().authorization;
    return route.fulfill({ json: [service] });
  });
  await page.goto("/b/" + otherShop.slug);
  await expect(page.getByRole("button", { name: /Corte Centro/ })).toBeVisible();
  expect(authorization).toBeUndefined();
});

test("selection fits a narrow screen", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: /Escolha sua barbearia/ })).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
  await page.screenshot({ path: "test-results/shop-selection-320.png", fullPage: true });
});

test("waits for API verification before mounting private pages", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/admin/me", async (route) => {
    await gate;
    await route.fulfill({ json: testIdentity });
  });
  await page.route("**/api/v1/admin/shops/*/services", (route) => route.fulfill({ json: [] }));
  await page.goto("/admin/" + testShop.slug + "/servicos");
  await expect(page.getByRole("status")).toContainText("Validando seu acesso");
  await expect(page.getByRole("button", { name: "Adicionar serviço" })).toHaveCount(0);
  release();
  await expect(page.getByRole("button", { name: "Adicionar serviço" })).toBeVisible();
});

test("a delayed response from the previous shop cannot populate the next shop", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let previousRequested = false;
  let previousFinished = false;
  await page.route("**/api/v1/admin/shops/*/services", async (route) => {
    if (route.request().url().includes(testShop.slug)) {
      previousRequested = true;
      await gate;
      await route
        .fulfill({ json: [{ ...service, name: "Previous shop service" }] })
        .catch(() => {});
      previousFinished = true;
    } else await route.fulfill({ json: [service] });
  });
  await page.goto("/admin/" + testShop.slug + "/servicos");
  await expect.poll(() => previousRequested).toBe(true);
  await page.getByRole("link", { name: "Trocar barbearia" }).click();
  await page.getByRole("link", { name: "Entrar em " + otherShop.name }).click();
  await expect(page.getByRole("heading", { name: service.name, exact: true })).toBeVisible();
  release();
  await expect.poll(() => previousFinished).toBe(true);
  await expect(page.getByRole("heading", { name: "Previous shop service" })).toHaveCount(0);
});

test("session restoration cannot keep the access screen waiting indefinitely", async ({ page }) => {
  const session = createTestSession();
  await page.clock.install();
  await page.addInitScript(
    ({ value }) => {
      localStorage.setItem(
        "sb-test-project-auth-token",
        JSON.stringify({ ...value, expires_at: 1 }),
      );
    },
    { value: session.response },
  );
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let refreshing = false;
  await page.route("https://test-project.supabase.co/auth/v1/token**", async (route) => {
    refreshing = true;
    await gate;
    await route.fulfill({ json: session.response }).catch(() => {});
  });
  await page.goto("/admin");
  await expect.poll(() => refreshing).toBe(true);
  await expect(page.getByRole("status")).toContainText("Validando seu acesso");
  await page.clock.fastForward(11000);
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "demorou mais que o esperado",
  );
  release();
});
