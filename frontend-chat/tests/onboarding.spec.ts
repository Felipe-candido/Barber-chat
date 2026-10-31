import { test, expect, type Page } from "@playwright/test";
import {
  createTestSession,
  installSupabaseSession,
  mockAdminAccess,
  testEmail,
  testPassword,
  testShop,
} from "./supabase-fixture";

async function readySession(page: Page) {
  const session = await installSupabaseSession(page);
  await page.route("https://test-project.supabase.co/auth/v1/user", (route) =>
    route.fulfill({ json: session.response.user }),
  );
  return session;
}
async function fillShop(page: Page) {
  await page.getByLabel("Seu nome", { exact: true }).fill("Felipe");
  await page.getByLabel("Nome da barbearia").fill("Barbearia do Felipe");
  await page.getByLabel("CEP", { exact: true }).fill("01310-100");
  await page.getByLabel("Rua ou avenida").fill("Avenida Paulista");
  await page.getByLabel("Número", { exact: true }).fill("100");
  await page.getByLabel("Bairro", { exact: true }).fill("Bela Vista");
  await page.getByLabel("Cidade", { exact: true }).fill("São Paulo");
  await page.getByLabel("Estado", { exact: true }).selectOption("SP");
}
const registered = { ...testShop, timezone: "America/Sao_Paulo", replayed: false };

test("signup sends credentials only to Supabase and waits for email confirmation", async ({
  page,
}) => {
  const session = createTestSession();
  let authBody: Record<string, unknown> = {};
  let apiCalls = 0;
  await page.route("http://127.0.0.1:8080/**", (route) => {
    apiCalls++;
    return route.abort();
  });
  await page.route("https://test-project.supabase.co/auth/v1/signup**", (route) => {
    authBody = route.request().postDataJSON();
    expect(route.request().url()).toContain(
      encodeURIComponent("http://127.0.0.1:3100/cadastro/barbearia"),
    );
    return route.fulfill({ json: { ...session.response.user, email_confirmed_at: null } });
  });
  await page.goto("/cadastro");
  await page.getByLabel("Seu nome", { exact: true }).fill("Felipe");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  await page.getByLabel("Senha", { exact: true }).fill(testPassword);
  await page.getByRole("button", { name: "Criar conta", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Confira seu e-mail" })).toBeVisible();
  expect(authBody).toMatchObject({
    email: testEmail,
    password: testPassword,
    data: { display_name: "Felipe" },
    code_challenge_method: "s256",
  });
  expect(authBody).not.toHaveProperty("role");
  expect(apiCalls).toBe(0);
  const storage = await page.evaluate(() =>
    JSON.stringify({ local: { ...localStorage }, tab: { ...sessionStorage } }),
  );
  expect(storage).not.toContain(testPassword);
  expect(storage).not.toContain(session.accessToken);
});

test("signup validation focuses the summary and links to the invalid field", async ({ page }) => {
  await page.goto("/cadastro");
  await page.getByRole("button", { name: "Criar conta", exact: true }).click();
  const summary = page.getByRole("alert").filter({ hasText: "Confira os campos abaixo" });
  await expect(summary).toBeFocused();
  await summary.getByRole("link", { name: /^Senha:/ }).click();
  await expect(page.getByLabel("Senha", { exact: true })).toBeFocused();
});

test("signup rejects seven characters and accepts an eight-character password", async ({
  page,
}) => {
  const session = createTestSession();
  const passwords: string[] = [];
  await page.route("https://test-project.supabase.co/auth/v1/signup**", (route) => {
    passwords.push(route.request().postDataJSON().password);
    return route.fulfill({ json: { ...session.response.user, email_confirmed_at: null } });
  });
  await page.goto("/cadastro");
  await page.getByLabel("Seu nome", { exact: true }).fill("Felipe");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  const password = page.getByLabel("Senha", { exact: true });
  await expect(password).toHaveAttribute("minlength", "8");
  await expect(password).toHaveAttribute("maxlength", "128");
  await password.fill("Ab1!xyz");
  await page.getByRole("button", { name: "Criar conta", exact: true }).click();
  await expect(password).toHaveAttribute("aria-invalid", "true");
  await expect(
    page.getByRole("alert").filter({ hasText: "Confira os campos abaixo" }),
  ).toContainText("Use uma senha de 8 a 128 caracteres.");
  expect(passwords).toEqual([]);
  await password.fill("Ab1!xyzQ");
  await page.getByRole("button", { name: "Criar conta", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Confira seu e-mail" })).toBeVisible();
  expect(passwords).toEqual(["Ab1!xyzQ"]);
});

test("signup with an already confirmed session continues without creating a shop automatically", async ({
  page,
}) => {
  const session = createTestSession();
  await page.route("https://test-project.supabase.co/auth/v1/signup**", (route) =>
    route.fulfill({ json: session.response }),
  );
  await page.route("https://test-project.supabase.co/auth/v1/user", (route) =>
    route.fulfill({ json: session.response.user }),
  );
  let writes = 0;
  await page.route("http://127.0.0.1:8080/api/v1/onboarding/shops", (route) => {
    writes++;
    return route.abort();
  });
  await page.goto("/cadastro");
  await page.getByLabel("Seu nome", { exact: true }).fill("Felipe");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  await page.getByLabel("Senha", { exact: true }).fill(testPassword);
  await page.getByRole("button", { name: "Criar conta", exact: true }).click();
  await expect(page).toHaveURL("/cadastro/barbearia");
  await expect(page.getByLabel("Nome da barbearia")).toBeVisible();
  expect(writes).toBe(0);
});

test("callback without PKCE verifier cannot silently use a previously logged in account", async ({
  page,
}) => {
  await readySession(page);
  await page.goto("/cadastro/barbearia?code=unrecognized-code");
  await expect(
    page.getByRole("alert").filter({ hasText: "Não foi possível validar o link" }),
  ).toBeVisible();
  await expect(page.getByLabel("Nome da barbearia")).toHaveCount(0);
  await expect(page).toHaveURL("/cadastro/barbearia");
});

test("draft from another account is cleared rather than reused", async ({ page }) => {
  await readySession(page);
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "barberchat-shop-registration-v1",
      JSON.stringify({
        userID: "99999999-9999-4999-8999-999999999999",
        key: "ab".repeat(32),
        input: { name: "Other account private draft" },
      }),
    ),
  );
  await page.goto("/cadastro/barbearia");
  await expect(page.getByLabel("Nome da barbearia")).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Criar minha barbearia", exact: true }),
  ).toBeEnabled();
  expect(
    await page.evaluate(() => sessionStorage.getItem("barberchat-shop-registration-v1")),
  ).toBeNull();
});

test("anonymous or unconfirmed users cannot reach the shop form", async ({ page }) => {
  await page.goto("/cadastro/barbearia");
  await expect(page.getByRole("link", { name: "Entrar com minha conta" })).toBeVisible();
  await expect(page.getByLabel("Nome da barbearia")).toHaveCount(0);
  const session = createTestSession();
  session.response.user.email_confirmed_at = "";
  await page.evaluate(
    (value) => localStorage.setItem("sb-test-project-auth-token", JSON.stringify(value)),
    session.response,
  );
  await page.route("https://test-project.supabase.co/auth/v1/user", (route) =>
    route.fulfill({ json: session.response.user }),
  );
  await page.reload();
  await expect(
    page.getByText("Confirme seu e-mail antes de continuar.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByLabel("Nome da barbearia")).toHaveCount(0);
});

test("confirmed signup callback exchanges PKCE once, removes code and ignores redirect targets", async ({
  page,
}) => {
  const session = createTestSession();
  let exchanges = 0;
  await page.addInitScript(() =>
    localStorage.setItem(
      "sb-test-project-auth-token-code-verifier",
      JSON.stringify("test-pkce-verifier"),
    ),
  );
  await page.route("https://test-project.supabase.co/auth/v1/token**", (route) => {
    exchanges++;
    expect(route.request().postDataJSON()).toMatchObject({
      auth_code: "confirmed-code",
      code_verifier: "test-pkce-verifier",
    });
    return route.fulfill({ json: session.response });
  });
  await page.route("https://test-project.supabase.co/auth/v1/user", (route) =>
    route.fulfill({ json: session.response.user }),
  );
  await page.goto("/cadastro/barbearia?code=confirmed-code&next=https://evil.example");
  await expect(page.getByLabel("Nome da barbearia")).toBeVisible();
  await expect(page).toHaveURL("/cadastro/barbearia");
  expect(exchanges).toBe(1);
});

test("required business address blocks submission, then provisions shop without sending user IDs or credentials", async ({
  page,
}) => {
  const session = await readySession(page);
  await mockAdminAccess(page);
  await page.route(`http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}/services`, (route) =>
    route.fulfill({ json: [] }),
  );
  let calls = 0;
  await page.route("http://127.0.0.1:8080/api/v1/onboarding/shops", (route) => {
    calls++;
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    expect(route.request().headers()["idempotency-key"]).toMatch(/^[0-9a-f]{64}$/);
    const body = route.request().postDataJSON();
    expect(body).toMatchObject({
      display_name: "Felipe",
      slug: testShop.slug,
      address: {
        postal_code: "01310-100",
        city: "São Paulo",
        state: "SP",
        number: "100",
        complement: "",
      },
    });
    for (const key of [
      "email",
      "password",
      "user_id",
      "owner_id",
      "role",
      "active",
      "publication_requested",
    ])
      expect(body).not.toHaveProperty(key);
    return route.fulfill({ status: 201, json: registered });
  });
  await page.goto("/cadastro/barbearia");
  await page.getByLabel("Seu nome", { exact: true }).fill("Felipe");
  await page.getByLabel("Nome da barbearia").fill("Barbearia do Felipe");
  await page.getByRole("button", { name: "Criar minha barbearia", exact: true }).click();
  await expect(page.locator("#registration-postal_code")).toHaveAttribute("aria-invalid", "true");
  expect(calls).toBe(0);
  await fillShop(page);
  await page.getByRole("button", { name: "Criar minha barbearia", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Sua barbearia está criada" })).toBeVisible();
  await expect(page.getByText(/O link de agendamento continua privado/)).toBeVisible();
  expect(calls).toBe(1);
  expect(
    await page.evaluate(() => sessionStorage.getItem("barberchat-shop-registration-v1")),
  ).toBeNull();
  await page.getByRole("button", { name: "Abrir painel e cadastrar serviços" }).click();
  await expect(page).toHaveURL(`/admin/${testShop.slug}/servicos`);
});

test("uncertain registration survives reload and recovers identical body and key", async ({
  page,
}) => {
  await readySession(page);
  const attempts: { key: string; body: string | null }[] = [];
  await page.route("http://127.0.0.1:8080/api/v1/onboarding/shops", (route) => {
    attempts.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postData(),
    });
    if (attempts.length === 1)
      return route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } });
    return route.fulfill({ json: { ...registered, replayed: true } });
  });
  await page.goto("/cadastro/barbearia");
  await fillShop(page);
  await page.getByRole("button", { name: "Criar minha barbearia", exact: true }).click();
  await expect(page.getByRole("button", { name: "Recuperar cadastro", exact: true })).toBeEnabled();
  await expect(page.getByLabel("Nome da barbearia")).toBeDisabled();
  await page.reload();
  await expect(page.getByLabel("Cidade", { exact: true })).toHaveValue("São Paulo");
  await page.getByRole("button", { name: "Recuperar cadastro", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Sua barbearia está criada" })).toBeVisible();
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
});

test("occupied slug permits correction after a definite rejection", async ({ page }) => {
  await readySession(page);
  await page.route("http://127.0.0.1:8080/api/v1/onboarding/shops", (route) =>
    route.fulfill({ status: 409, json: { error: { code: "shop_slug_taken" } } }),
  );
  await page.goto("/cadastro/barbearia");
  await fillShop(page);
  await page.getByRole("button", { name: "Criar minha barbearia", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "já está em uso" })).toBeVisible();
  await expect(page.getByLabel("Endereço do link")).toBeEnabled();
  expect(
    await page.evaluate(() => sessionStorage.getItem("barberchat-shop-registration-v1")),
  ).toBeNull();
});

test("login without local provisioning offers onboarding instead of recreating auth account", async ({
  page,
}) => {
  const session = createTestSession();
  await page.route("https://test-project.supabase.co/auth/v1/token**", (route) =>
    route.fulfill({ json: session.response }),
  );
  await page.route("http://127.0.0.1:8080/api/v1/admin/me", (route) =>
    route.fulfill({ status: 403, json: { error: { code: "access_denied" } } }),
  );
  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  await page.getByLabel("Senha", { exact: true }).fill(testPassword);
  await page.getByRole("button", { name: "Entrar no painel", exact: true }).click();
  await expect(
    page.getByRole("link", { name: "Concluir cadastro da minha barbearia" }),
  ).toBeVisible();
});

test("signup and address form fit mobile and desktop without overflow", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 960 });
    await page.goto("/cadastro");
    await expect(page.getByRole("heading", { name: "Comece pela sua conta" })).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    ).toBeTruthy();
    await page.screenshot({ path: `test-results/onboarding-account-${width}.png`, fullPage: true });
  }
  await readySession(page);
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 960 });
    await page.goto("/cadastro/barbearia");
    await expect(page.getByLabel("Nome da barbearia")).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    ).toBeTruthy();
    await page.screenshot({ path: `test-results/onboarding-shop-${width}.png`, fullPage: true });
  }
  expect(errors).toEqual([]);
});
