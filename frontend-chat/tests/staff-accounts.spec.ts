import { expect, test } from "@playwright/test";
import {
  createTestSession,
  emptyAgenda,
  installSupabaseSession,
  mockAdminAccess,
  testShop,
} from "./supabase-fixture";

const base = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug;
const professional = {
  id: "22222222-2222-4222-8222-222222222222",
  name: "Palma",
  active: true,
  service_ids: ["55555555-5555-4555-8555-555555555555"],
};
const account = {
  id: "44444444-4444-4444-8444-444444444444",
  professional_id: professional.id,
  email: "palma@example.com",
  user_id: "66666666-6666-4666-8666-666666666666",
  state: "linked",
  active: true,
};

test("owner invites a separate login and can disable the shop membership", async ({ page }) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  let items: (typeof account)[] = [];
  const writes: unknown[] = [];
  await page.route(base + "/professionals", (r) => r.fulfill({ json: [professional] }));
  await page.route(base + "/accounts**", async (r) => {
    const request = r.request();
    expect(request.headers().authorization).toBe("Bearer " + session.accessToken);
    if (request.method() === "POST") {
      writes.push(request.postDataJSON());
      items = [{ ...account, active: !request.url().endsWith("/status") }];
      await r.fulfill({ status: request.url().endsWith("/status") ? 200 : 201, json: items[0] });
    } else await r.fulfill({ json: items });
  });
  await page.goto("/admin/" + testShop.slug + "/contas");
  await page.getByLabel("Barbeiro vinculado").selectOption(professional.id);
  await page.getByLabel("E-mail do barbeiro").fill(account.email);
  await page.getByRole("button", { name: "Criar conta e enviar convite" }).click();
  await expect(page.getByText("Acesso ativo", { exact: true })).toBeVisible();
  expect(writes).toEqual([{ professional_id: professional.id, email: account.email }]);
  await expect(
    page.getByLabel("Barbeiro vinculado").locator("option[value='" + professional.id + "']"),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Desativar acesso" }).click();
  await expect(page.getByText("Acesso inativo", { exact: true })).toBeVisible();
  expect(writes[1]).toEqual({ active: false });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
  await page.screenshot({ path: "test-results/staff-accounts-mobile.png", fullPage: true });
});

test("ambiguous invitation stays visible for explicit recovery, not automatic retry", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let items: object[] = [];
  let posts = 0;
  await page.route(base + "/professionals", (r) => r.fulfill({ json: [professional] }));
  await page.route(base + "/accounts**", async (r) => {
    if (r.request().method() === "POST") {
      posts++;
      if (r.request().url().endsWith("/link")) {
        expect(r.request().postDataJSON()).toEqual({ user_id: account.user_id });
        items = [account];
        await r.fulfill({ json: account });
      } else {
        items = [{ ...account, user_id: null, state: "review", active: false }];
        await r.abort("failed");
      }
    } else await r.fulfill({ json: items });
  });
  await page.goto("/admin/" + testShop.slug + "/contas");
  await page.getByLabel("Barbeiro vinculado").selectOption(professional.id);
  await page.getByLabel("E-mail do barbeiro").fill(account.email);
  await page.getByRole("button", { name: "Criar conta e enviar convite" }).click();
  await expect(page.getByText("Vínculo precisa ser conferido", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Criar conta e enviar convite" })).toBeDisabled();
  expect(posts).toBe(1);
  await page.getByText("Conferir conta existente no Supabase", { exact: true }).click();
  await page.getByLabel("UUID da conta Supabase").fill(account.user_id);
  await page.getByRole("button", { name: "Concluir vínculo" }).click();
  await expect(page.getByText("Acesso ativo", { exact: true })).toBeVisible();
  expect(posts).toBe(2);
});

test("barber sees a simple menu, cannot open accounts, and creates only for their professional", async ({
  page,
}) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(base + "/access", (r) =>
    r.fulfill({ json: { role: "barber", professional_id: professional.id } }),
  );
  let accountsCalls = 0;
  await page.route(base + "/accounts**", (r) => {
    accountsCalls++;
    return r.fulfill({ status: 403, json: { error: { code: "access_denied" } } });
  });
  await page.goto("/admin/" + testShop.slug + "/contas");
  await expect(
    page.getByRole("heading", { name: "Acesso restrito ao proprietário" }),
  ).toBeVisible();
  expect(accountsCalls).toBe(0);
  await expect(page.getByRole("link", { name: "Contas de acesso", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Barbeiros", exact: true })).toHaveCount(0);
  const service = {
    id: professional.service_ids[0],
    name: "Corte",
    description: "",
    duration_minutes: 40,
    price_cents: 5000,
    currency: "BRL",
    active: true,
  };
  await page.route(base + "/services", (r) => r.fulfill({ json: [service] }));
  await page.goto("/admin/" + testShop.slug + "/servicos");
  await expect(page.getByRole("heading", { name: "Corte", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Adicionar serviço", exact: true })).toHaveCount(0);
  const appointment = {
    id: account.id,
    shop_name: testShop.name,
    status: "scheduled",
    service_name: "Corte",
    duration_minutes: 40,
    price_cents: 5000,
    currency: "BRL",
    professional_id: professional.id,
    professional_name: professional.name,
    starts_at: "2026-10-05T12:30:00Z",
    ends_at: "2026-10-05T13:30:00Z",
    shop_timezone: "America/Sao_Paulo",
  };
  await page.route(base + "/booking/**", (r) =>
    r.fulfill({
      json: r.request().url().includes("/professionals?")
        ? {
            shop_name: testShop.name,
            shop_timezone: "America/Sao_Paulo",
            today: "2026-10-05",
            last_booking_date: "2027-01-03",
            professionals: [professional, { id: account.user_id, name: "Outro barbeiro" }],
          }
        : {
            shop_timezone: "America/Sao_Paulo",
            slots: [{ starts_at: appointment.starts_at, ends_at: appointment.ends_at }],
          },
    }),
  );
  let posts = 0;
  let reads = 0;
  const attempts: unknown[] = [];
  await page.route(base + "/appointments**", async (r) => {
    if (r.request().method() === "POST") {
      posts++;
      expect(r.request().headers().authorization).toBe("Bearer " + session.accessToken);
      expect(r.request().headers()["idempotency-key"]).toMatch(/^[a-f0-9]{64}$/);
      expect(r.request().postDataJSON().professional_id).toBe(professional.id);
      attempts.push({
        key: r.request().headers()["idempotency-key"],
        body: r.request().postDataJSON(),
      });
      if (posts === 1) return r.abort("failed");
      await r.fulfill({
        status: 201,
        json: { appointment, code: "ABCDEFGH-IJKLMNOP-QRSTUVWX-YZ234567", replayed: false },
      });
    } else {
      reads++;
      await r.fulfill({ json: emptyAgenda });
    }
  });
  await page.goto("/admin/" + testShop.slug);
  await page.getByRole("button", { name: "Novo agendamento" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("combobox", { name: "Serviço", exact: true }).selectOption(service.id);
  await expect(dialog.getByRole("combobox", { name: "Profissional", exact: true })).toBeDisabled();
  await expect(dialog.getByRole("combobox", { name: "Profissional", exact: true })).toHaveValue(
    professional.id,
  );
  await expect(dialog.getByText("Outro barbeiro", { exact: true })).toHaveCount(0);
  await dialog.getByRole("button", { name: "09:30", exact: true }).click();
  await dialog.getByLabel("Nome", { exact: true }).fill("Ana");
  await dialog.getByLabel("Sobrenome", { exact: true }).fill("Silva");
  await dialog.getByLabel("Telefone com DDD").fill("11912345678");
  await dialog.getByRole("button", { name: "Revisar agendamento" }).click();
  await dialog.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(dialog.getByRole("heading", { name: "Agendamento registrado!" })).toBeVisible();
  expect(posts).toBe(2);
  expect(attempts[0]).toEqual(attempts[1]);
  await expect.poll(() => reads).toBeGreaterThan(1);
});

test("invitation opens in a fresh browser and password goes only to Supabase", async ({ page }) => {
  const session = createTestSession();
  await mockAdminAccess(page);
  let submitted: unknown;
  await page.route("https://test-project.supabase.co/auth/v1/user", async (r) => {
    expect(r.request().headers().authorization).toBe("Bearer " + session.accessToken);
    if (r.request().method() === "PUT") submitted = r.request().postDataJSON();
    await r.fulfill({ json: session.response.user });
  });
  await page.goto(
    "/definir-senha#access_token=" +
      session.accessToken +
      "&refresh_token=test-refresh-token&expires_in=3600&token_type=bearer&type=invite",
  );
  const password = page.getByLabel("Nova senha", { exact: true });
  const confirmation = page.getByLabel("Confirmar senha", { exact: true });
  await expect(password).toHaveAttribute("minlength", "8");
  await expect(confirmation).toHaveAttribute("minlength", "8");
  await password.fill("Ab1!xyz");
  await confirmation.fill("Ab1!xyz");
  await page.getByRole("button", { name: "Salvar senha" }).click();
  await expect(password).toHaveAttribute("aria-invalid", "true");
  expect(submitted).toBeUndefined();
  await password.fill("Ab1!xyzQ");
  await confirmation.fill("Ab1!xyzQ");
  await page.getByRole("button", { name: "Salvar senha" }).click();
  await expect(page.getByRole("status")).toContainText("Senha definida");
  expect(submitted).toMatchObject({ password: "Ab1!xyzQ" });
  expect(await page.evaluate(() => location.hash)).toBe("");
  expect(await page.evaluate(() => JSON.stringify({ ...localStorage }))).not.toContain("Ab1!xyzQ");
});

test("owner replaces an inactive login without recreating the professional", async ({ page }) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  let items = [{ ...account, active: false }];
  const replacement = {
    ...account,
    id: "77777777-7777-4777-8777-777777777777",
    email: "correct@example.com",
  };
  const writes: unknown[] = [];
  await page.route(base + "/professionals", (r) => r.fulfill({ json: [professional] }));
  await page.route(base + "/accounts**", async (r) => {
    if (r.request().method() === "POST") {
      expect(r.request().url()).toBe(base + "/accounts/" + account.id + "/replace");
      expect(r.request().headers().authorization).toBe("Bearer " + session.accessToken);
      writes.push(r.request().postDataJSON());
      items = [replacement];
      await r.fulfill({ status: 201, json: replacement });
    } else await r.fulfill({ json: items });
  });
  await page.goto("/admin/" + testShop.slug + "/contas");
  await expect(page.getByText("Acesso inativo", { exact: true })).toBeVisible();
  await page.getByText("Substituir conta", { exact: true }).click();
  const submit = page.getByRole("button", { name: "Substituir e enviar convite" });
  await expect(submit).toBeDisabled();
  await page.getByLabel("Novo e-mail de acesso").fill(account.email.toUpperCase());
  await page.getByLabel("Confirmo a remoção do vínculo antigo nesta barbearia.").check();
  await expect(submit).toBeDisabled();
  await page.getByLabel("Novo e-mail de acesso").fill("CORRECT@example.com");
  await expect(submit).toBeDisabled();
  await page.getByLabel("Confirmo a remoção do vínculo antigo nesta barbearia.").check();
  await submit.click();
  await expect(page.getByText(replacement.email, { exact: true })).toBeVisible();
  await expect(page.getByText(account.email, { exact: true })).toHaveCount(0);
  await expect(page.getByText("Acesso ativo", { exact: true })).toBeVisible();
  expect(writes).toEqual([{ email: replacement.email }]);
  await expect(
    page.getByLabel("Barbeiro vinculado").locator("option[value='" + professional.id + "']"),
  ).toHaveCount(0);
});

test("failed replacement refreshes the new review request and never resends automatically", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let items: object[] = [{ ...account, active: false }];
  let posts = 0;
  await page.route(base + "/professionals", (r) => r.fulfill({ json: [professional] }));
  await page.route(base + "/accounts**", async (r) => {
    if (r.request().method() === "POST") {
      posts++;
      items = [
        {
          ...account,
          id: "77777777-7777-4777-8777-777777777777",
          email: "correct@example.com",
          user_id: null,
          state: "review",
          active: false,
        },
      ];
      await r.abort("failed");
    } else await r.fulfill({ json: items });
  });
  await page.goto("/admin/" + testShop.slug + "/contas");
  await page.getByText("Substituir conta", { exact: true }).click();
  await page.getByLabel("Novo e-mail de acesso").fill("correct@example.com");
  await page.getByLabel("Confirmo a remoção do vínculo antigo nesta barbearia.").check();
  await page.getByRole("button", { name: "Substituir e enviar convite" }).click();
  await expect(page.getByText("Vínculo precisa ser conferido", { exact: true })).toBeVisible();
  await expect(page.getByText("correct@example.com", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Atualizar contas" }).click();
  await expect(page.getByText("Vínculo precisa ser conferido", { exact: true })).toBeVisible();
  expect(posts).toBe(1);
  await expect(page.getByRole("button", { name: "Criar conta e enviar convite" })).toBeDisabled();
});
