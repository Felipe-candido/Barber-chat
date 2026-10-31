import { expect, test, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

const base = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug;
const service = {
  id: "55555555-5555-4555-8555-555555555555",
  name: "Corte e finalização",
  description: "Lavagem e acabamento incluídos.",
  duration_minutes: 40,
  price_cents: 5500,
  currency: "BRL",
  active: true,
};
const professional = {
  id: "22222222-2222-4222-8222-222222222222",
  name: "João Carlos da Silva",
  active: true,
  service_ids: [service.id],
};
const account = {
  id: "44444444-4444-4444-8444-444444444444",
  professional_id: professional.id,
  email: "joao.carlos.equipe@example.com",
  user_id: "66666666-6666-4666-8666-666666666666",
  state: "linked",
  active: false,
};
async function setup(page: Page) {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(base + "/services", (route) => route.fulfill({ json: [service] }));
  await page.route(base + "/professionals", (route) => route.fulfill({ json: [professional] }));
  await page.route(base + "/accounts", (route) => route.fulfill({ json: [account] }));
}

for (const path of ["servicos", "barbeiros", "contas"]) {
  test(`${path} uses readable controls without horizontal overflow`, async ({ page }) => {
    await setup(page);
    await page.goto(`/admin/${testShop.slug}/${path}`);
    await expect(
      page.getByRole("heading", {
        name:
          path === "servicos"
            ? service.name
            : path === "barbeiros"
              ? "Equipe cadastrada"
              : "Acessos cadastrados",
        exact: true,
      }),
    ).toBeVisible();
    if (path === "contas") {
      await expect(page.getByText(account.email, { exact: true })).toBeVisible();
      await page.getByText("Substituir conta", { exact: true }).click();
      await page.getByLabel("Novo e-mail de acesso").fill("outro.barbeiro@example.com");
    }
    if (path === "barbeiros") await expect(page.getByLabel("Nome do barbeiro")).toBeVisible();
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 960 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      const controls = await page
        .getByRole("main")
        .locator('input:not([type="checkbox"]), select, textarea')
        .evaluateAll((elements) =>
          elements
            .filter((element) => element.getBoundingClientRect().height > 0)
            .map((element) => ({
              height: element.getBoundingClientRect().height,
              font: parseFloat(getComputedStyle(element).fontSize),
              label: (element as HTMLInputElement).labels?.length,
            })),
        );
      for (const control of controls) {
        expect(control.height).toBeGreaterThanOrEqual(44);
        expect(control.font).toBeGreaterThanOrEqual(16);
        expect(control.label).toBeGreaterThan(0);
      }
      await page.screenshot({
        path: `test-results/management-${path}-${width}.png`,
        fullPage: true,
      });
    }
  });
}

test("service search has an actionable empty state and preserves the shop scope", async ({
  page,
}) => {
  await setup(page);
  await page.goto(`/admin/${testShop.slug}/servicos`);
  await expect(page.getByRole("heading", { name: service.name, exact: true })).toBeVisible();
  await page.getByLabel("Buscar serviço").fill("não existe");
  await expect(page.getByRole("heading", { name: "Nenhum resultado" })).toBeVisible();
  await page.getByRole("button", { name: "Limpar busca" }).click();
  await expect(page.getByRole("heading", { name: service.name, exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Ver como cliente" })).toHaveAttribute(
    "href",
    `/b/${testShop.slug}`,
  );
});

test("service creation keeps validation editable and blocks a second uncertain send", async ({
  page,
}) => {
  await setup(page);
  let posts = 0;
  await page.route(base + "/services", async (route) => {
    if (route.request().method() === "POST") {
      posts++;
      await route.abort("failed");
    } else await route.fulfill({ json: [service] });
  });
  await page.setViewportSize({ width: 320, height: 740 });
  await page.goto(`/admin/${testShop.slug}/servicos`);
  const trigger = page.getByRole("button", { name: "Adicionar serviço", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Adicionar serviço", exact: true });
  await dialog.getByLabel("Nome do serviço").fill("   ");
  await dialog.getByLabel("Preço (R$)").fill("40,00");
  await dialog.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Informe um nome");
  await expect(dialog.getByLabel("Nome do serviço")).toBeEnabled();
  expect(posts).toBe(0);
  await dialog.getByLabel("Nome do serviço").fill("Novo corte");
  await dialog.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  await expect(dialog.getByRole("status")).toContainText("resultado pode ter sido salvo");
  await expect(
    dialog.getByRole("button", { name: "Adicionar serviço", exact: true }),
  ).toBeDisabled();
  await expect(dialog.getByLabel("Nome do serviço")).toHaveValue("Novo corte");
  expect(posts).toBe(1);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: "test-results/management-service-uncertain-320.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("uncertain professional creation stays locked while the owner checks the list", async ({
  page,
}) => {
  await setup(page);
  let posts = 0;
  await page.route(base + "/professionals", async (route) => {
    if (route.request().method() === "POST") {
      posts++;
      await route.abort("failed");
    } else await route.fulfill({ json: posts ? [professional] : [] });
  });
  await page.goto(`/admin/${testShop.slug}/barbeiros`);
  await page.getByLabel("Nome do barbeiro").fill(professional.name);
  await page.getByRole("checkbox", { name: `${service.name} · 40 min` }).check();
  await page.getByRole("button", { name: "Cadastrar barbeiro", exact: true }).click();
  await expect(page.getByLabel("Nome do barbeiro")).toBeDisabled();
  await expect(page.getByRole("status")).toContainText("resultado pode ter sido salvo");
  await page.getByRole("button", { name: "Atualizar barbeiros" }).click();
  await expect(
    page.getByLabel("Profissionais cadastrados").getByText(professional.name),
  ).toBeVisible();
  expect(posts).toBe(1);
  await page.getByRole("button", { name: "Já conferi a lista; encerrar esta tentativa" }).click();
  await expect(page.getByLabel("Nome do barbeiro")).toBeEnabled();
  await expect(page.getByLabel("Nome do barbeiro")).toHaveValue("");
});

test("failed account refresh blocks changes instead of showing stale access controls", async ({
  page,
}) => {
  await setup(page);
  let failed = false;
  let posts = 0;
  await page.route(base + "/accounts**", async (route) => {
    if (route.request().method() === "POST") posts++;
    await route.fulfill(
      failed
        ? { status: 503, json: { error: { code: "temporarily_unavailable" } } }
        : { json: [account] },
    );
  });
  await page.goto(`/admin/${testShop.slug}/contas`);
  await expect(page.getByText(account.email, { exact: true })).toBeVisible();
  failed = true;
  await page.getByRole("button", { name: "Atualizar contas" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "temporariamente indisponível",
  );
  await expect(page.getByRole("button", { name: "Ativar acesso", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Criar conta e enviar convite" })).toBeDisabled();
  expect(posts).toBe(0);
});

test("account recovery preserves the entered UUID after a validation failure and refresh", async ({
  page,
}) => {
  await setup(page);
  await page.route(base + "/accounts", (route) =>
    route.fulfill({ json: [{ ...account, user_id: null, state: "review" }] }),
  );
  await page.goto(`/admin/${testShop.slug}/contas`);
  const summary = page.getByText("Conferir conta existente no Supabase", { exact: true });
  await summary.focus();
  await expect(summary).toBeFocused();
  await page.keyboard.press("Enter");
  const input = page.getByLabel("UUID da conta Supabase");
  await input.fill("uuid-incompleto");
  await page.getByRole("button", { name: "Concluir vínculo" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Informe o UUID");
  await expect(input).toHaveValue("uuid-incompleto");
  await expect(input).toBeVisible();
  await page.getByRole("button", { name: "Atualizar contas" }).click();
  await expect(input).toHaveValue("uuid-incompleto");
  await expect(input).toBeVisible();
});
