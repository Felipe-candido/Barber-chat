import { expect, test, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}/services`;
const initial = {
  id: "55555555-5555-4555-8555-555555555555",
  name: "Corte clássico",
  description: "Tesoura e máquina",
  duration_minutes: 30,
  price_cents: 4500,
  currency: "BRL",
  active: true,
};

async function setup(page: Page) {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
}

test("owner edits the existing service and confirms deletion before it is removed", async ({
  page,
}) => {
  await setup(page);
  let items = [initial];
  let deletes = 0;
  const bodies: unknown[] = [];
  await page.route(base, (route) => route.fulfill({ json: items }));
  await page.route(base + "/" + initial.id, async (route) => {
    if (route.request().method() === "PUT") {
      const body = route.request().postDataJSON();
      bodies.push(body);
      items = [{ ...initial, ...body }];
      await route.fulfill({ json: items[0] });
    } else if (route.request().method() === "DELETE") {
      deletes++;
      items = [];
      await route.fulfill({ status: 204 });
    } else await route.abort();
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/admin/${testShop.slug}/servicos`);
  await page.getByRole("button", { name: `Editar ${initial.name}`, exact: true }).click();
  const editor = page.getByRole("dialog", { name: "Editar serviço", exact: true });
  await expect(editor.getByLabel("Nome do serviço")).toHaveValue(initial.name);
  await expect(editor.getByLabel("Preço (R$)")).toHaveValue("45,00");
  await expect(editor.getByLabel("Duração (minutos)")).toHaveValue("30");
  await editor.getByLabel("Nome do serviço").fill("Corte e barba");
  await editor.getByLabel("Descrição").fill("Corte completo");
  await editor.getByLabel("Preço (R$)").fill("65,90");
  await editor.getByLabel("Duração (minutos)").fill("50");
  await editor.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Corte e barba", exact: true })).toBeVisible();
  expect(bodies).toEqual([
    {
      name: "Corte e barba",
      description: "Corte completo",
      price_cents: 6590,
      duration_minutes: 50,
    },
  ]);
  await page.reload();
  await page.getByRole("button", { name: "Excluir Corte e barba", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "Excluir serviço?", exact: true });
  await expect(confirmation).toContainText("histórico serão mantidos");
  await confirmation.getByRole("button", { name: "Manter serviço" }).click();
  expect(deletes).toBe(0);
  await page.getByRole("button", { name: "Excluir Corte e barba", exact: true }).click();
  await confirmation.getByRole("button", { name: "Excluir serviço", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Nenhum serviço ativo" })).toBeVisible();
  expect(deletes).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("barber can read services without management buttons", async ({ page }) => {
  await setup(page);
  await page.route(`**/api/v1/admin/shops/${testShop.slug}/access`, (route) =>
    route.fulfill({
      json: { role: "barber", professional_id: "22222222-2222-4222-8222-222222222222" },
    }),
  );
  await page.route(base, (route) => route.fulfill({ json: [initial] }));
  await page.goto(`/admin/${testShop.slug}/servicos`);
  await expect(page.getByRole("heading", { name: initial.name, exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: /Adicionar serviço|Editar Corte|Excluir Corte/ }),
  ).toHaveCount(0);
});

test("failed editing keeps the entered data and requires checking before resending", async ({
  page,
}) => {
  await setup(page);
  let writes = 0;
  await page.route(base, (route) => route.fulfill({ json: [initial] }));
  await page.route(base + "/" + initial.id, async (route) => {
    writes++;
    await route.abort("failed");
  });
  await page.goto(`/admin/${testShop.slug}/servicos`);
  await page.getByRole("button", { name: `Editar ${initial.name}`, exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Editar serviço", exact: true });
  await dialog.getByLabel("Nome do serviço").fill("Meu corte");
  await dialog.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(dialog.getByRole("status")).toContainText("atualize a lista");
  await expect(dialog.getByLabel("Nome do serviço")).toHaveValue("Meu corte");
  await expect(
    dialog.getByRole("button", { name: "Salvar alterações", exact: true }),
  ).toBeDisabled();
  expect(writes).toBe(1);
});
