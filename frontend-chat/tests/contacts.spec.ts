import { test, expect, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, otherShop, testShop } from "./supabase-fixture";

const base = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug + "/contacts";
const first = {
  id: "55555555-5555-4555-8555-555555555555",
  shop_id: testShop.shop_id,
  first_name: "Cliente",
  last_name: "Anterior",
  name: "Cliente Anterior",
  phone: "+5511912345678",
  created_at: "2026-11-01T12:00:00Z",
};
const saved = {
  id: "66666666-6666-4666-8666-666666666666",
  shop_id: testShop.shop_id,
  first_name: "Renato",
  last_name: "",
  name: "Renato",
  phone: "+5511987654321",
  created_at: "2026-11-02T12:00:00Z",
};
async function access(page: Page, role: "owner" | "barber" = "owner") {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  if (role === "barber")
    await page.route("http://127.0.0.1:8080/api/v1/admin/shops/*/access", (route) =>
      route.fulfill({
        json: { role: "barber", professional_id: "77777777-7777-4777-8777-777777777777" },
      }),
    );
  return session;
}
async function fillContact(page: Page) {
  await page.getByRole("button", { name: "Novo contato", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome", { exact: true }).fill("Renato");
  await dialog.getByLabel("Telefone com DDD").fill("(11) 98765-4321");
  return dialog;
}

for (const role of ["owner", "barber"] as const) {
  test(`${role} reads existing contacts and manually creates one without surname or persisted PII`, async ({
    page,
  }) => {
    const session = await access(page, role);
    let contacts = [first],
      posts = 0;
    await page.route(base + "**", (route) => {
      expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
      if (route.request().method() === "POST") {
        posts++;
        expect(route.request().postDataJSON()).toEqual({
          first_name: "Renato",
          last_name: "",
          phone: "(11) 98765-4321",
          phone_country: "BR",
        });
        contacts = [saved, ...contacts];
        return route.fulfill({ status: 201, json: saved });
      }
      return route.fulfill({ json: { items: contacts, next_cursor: null } });
    });
    await page.goto(`/admin/${testShop.slug}/contatos`);
    await expect(page.getByRole("heading", { name: "Contatos", exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: first.name, exact: true })).toBeVisible();
    const dialog = await fillContact(page);
    await dialog.getByRole("button", { name: "Cadastrar contato", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Renato", exact: true })).toBeVisible();
    expect(posts).toBe(1);
    const storage = await page.evaluate(() =>
      JSON.stringify({ local: { ...localStorage }, tab: { ...sessionStorage } }),
    );
    expect(storage).not.toContain("Renato");
    expect(storage).not.toContain(saved.phone);
  });
}

test("pagination and name/phone filters keep contact listing explicit", async ({ page }) => {
  await access(page);
  const queries: URLSearchParams[] = [];
  await page.route(base + "**", (route) => {
    const q = new URL(route.request().url()).searchParams;
    queries.push(q);
    const filtered = q.has("name") || q.has("phone");
    return route.fulfill({
      json: {
        items: q.get("cursor") ? [saved] : [first],
        next_cursor: !filtered && !q.get("cursor") ? "second_page" : null,
      },
    });
  });
  await page.goto(`/admin/${testShop.slug}/contatos`);
  await page.getByRole("button", { name: "Carregar mais contatos" }).click();
  await expect(page.getByRole("listitem")).toHaveCount(2);
  expect(queries.some((q) => q.get("cursor") === "second_page")).toBeTruthy();
  await page.getByLabel("Buscar contato", { exact: true }).fill("Cliente");
  await page.getByRole("button", { name: "Buscar", exact: true }).click();
  await expect.poll(() => queries.at(-1)?.get("name")).toBe("Cliente");
  await expect(page.getByRole("listitem")).toHaveCount(1);
  await page.getByLabel("Buscar contato", { exact: true }).fill("(11) 91234-5678");
  await page.getByRole("button", { name: "Buscar", exact: true }).click();
  await expect.poll(() => queries.at(-1)?.get("phone")).toBe("(11) 91234-5678");
  expect(queries.at(-1)?.get("phone_country")).toBe("BR");
  expect(queries.at(-1)?.has("name")).toBe(false);
});

for (const mode of ["lost_response", "conflict"] as const) {
  test(`${mode} is recovered through a read without repeating the create`, async ({ page }) => {
    await access(page);
    let posts = 0,
      checks = 0;
    let exists = mode === "conflict";
    await page.route(base + "**", (route) => {
      if (route.request().method() === "POST") {
        posts++;
        exists = true;
        return mode === "lost_response"
          ? route.abort("failed")
          : route.fulfill({ status: 409, json: { error: { code: "contact_exists" } } });
      }
      const q = new URL(route.request().url()).searchParams;
      if (q.has("phone")) checks++;
      return route.fulfill({ json: { items: exists ? [saved] : [], next_cursor: null } });
    });
    await page.goto(`/admin/${testShop.slug}/contatos`);
    const dialog = await fillContact(page);
    await dialog.getByRole("button", { name: "Cadastrar contato", exact: true }).click();
    await expect(
      dialog.getByRole("button", { name: "Conferir contato pelo telefone" }),
    ).toBeVisible();
    await expect(dialog.getByLabel("Nome", { exact: true })).toBeDisabled();
    await dialog.getByRole("button", { name: "Conferir contato pelo telefone" }).click();
    await expect(dialog.getByRole("heading", { name: "Contato encontrado" })).toBeVisible();
    await dialog.getByRole("button", { name: "Concluir", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Renato", exact: true })).toBeVisible();
    expect(posts).toBe(1);
    expect(checks).toBeGreaterThan(0);
  });
}

test("read failure is not an empty list and retry performs only GET", async ({ page }) => {
  await access(page);
  let failure = true,
    posts = 0;
  await page.route(base + "**", (route) => {
    if (route.request().method() !== "GET") posts++;
    return failure
      ? route.fulfill({ status: 503, json: { error: { code: "contact_unavailable" } } })
      : route.fulfill({ json: { items: [], next_cursor: null } });
  });
  await page.goto(`/admin/${testShop.slug}/contatos`);
  await expect(
    page.getByRole("alert").filter({ hasText: "Não foi possível confirmar a operação" }),
  ).toBeVisible();
  await expect(page.getByText("Sua lista de contatos começa aqui", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Novo contato", exact: true })).toBeDisabled();
  failure = false;
  await page.getByRole("button", { name: "Tentar novamente", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Sua lista de contatos começa aqui" }),
  ).toBeVisible();
  expect(posts).toBe(0);
});

test("foreign-tenant response is rejected and a revoked membership hides the form", async ({
  page,
}) => {
  await access(page);
  let foreign = true,
    revoked = false;
  await page.route(base + "**", (route) => {
    if (route.request().method() === "POST") {
      revoked = true;
      return route.fulfill({ status: 403, json: { error: { code: "contact_access_denied" } } });
    }
    return revoked
      ? route.fulfill({ status: 403, json: { error: { code: "contact_access_denied" } } })
      : route.fulfill({
          json: {
            items: foreign ? [{ ...first, shop_id: otherShop.shop_id }] : [first],
            next_cursor: null,
          },
        });
  });
  await page.goto(`/admin/${testShop.slug}/contatos`);
  await expect(
    page.getByRole("alert").filter({ hasText: "Não foi possível validar a resposta" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: first.name, exact: true })).toHaveCount(0);
  foreign = false;
  await page.getByRole("button", { name: "Tentar novamente", exact: true }).click();
  const dialog = await fillContact(page);
  await dialog.getByRole("button", { name: "Cadastrar contato", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("alert").filter({ hasText: "Seu acesso aos contatos" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: first.name, exact: true })).toHaveCount(0);
});

test("contacts are reachable through the mobile menu and remain readable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await access(page, "barber");
  await page.route(base + "**", (route) =>
    route.fulfill({ json: { items: [first], next_cursor: null } }),
  );
  await page.goto(`/admin/${testShop.slug}`);
  await page.getByRole("button", { name: "Mais", exact: true }).click();
  await page.getByRole("link", { name: "Contatos", exact: true }).click();
  await expect(page).toHaveURL(`/admin/${testShop.slug}/contatos`);
  await expect(page.getByRole("heading", { name: first.name, exact: true })).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBeTruthy();
  await page.screenshot({ path: "test-results/contacts-mobile.png", fullPage: true });
  const dialog = await fillContact(page);
  await expect(dialog.getByLabel("Sobrenome (opcional)")).toBeVisible();
  await page.screenshot({ path: "test-results/contact-form-mobile.png", fullPage: true });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBeTruthy();
});
