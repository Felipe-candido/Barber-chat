import { test, expect, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, otherShop, testShop } from "./supabase-fixture";
import {
  canonicalSettings,
  mergeSettings,
  settingsDraft,
  validateSettingsDraft,
} from "../src/lib/shop-settings";
import type { SettingsUpdate, ShopSettings } from "../src/lib/api/shop-settings";

const path = `/admin/${testShop.slug}/configuracoes`;
const endpoint = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}/settings`;
const initial: ShopSettings = {
  ...testShop,
  timezone: "America/Sao_Paulo",
  phone: "+5511912345678",
  description: "Cortes e barba com horário marcado.",
  version: 1,
  address: {
    postal_code: "01310100",
    street: "Avenida Paulista",
    number: "100",
    complement: "Sala 2",
    district: "Bela Vista",
    city: "São Paulo",
    state: "SP",
  },
};
const save = (page: Page) => page.getByRole("button", { name: "Salvar alterações", exact: true });
const name = (page: Page) => page.getByLabel("Nome da barbearia", { exact: true });
async function setup(page: Page) {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  return session;
}
function saved(body: SettingsUpdate, current = initial): ShopSettings {
  return { ...current, ...body, version: body.version + 1 };
}

test("settings helpers normalize inputs and preserve concurrent remote changes in untouched fields", () => {
  const base = settingsDraft(initial);
  const local = structuredClone(base);
  local.name = "My change";
  const remote = structuredClone(base);
  remote.phone = "+5511987654321";
  remote.address.number = "200";
  const result = mergeSettings(base, remote, local);
  expect(result).toMatchObject({
    name: "My change",
    phone: "+5511987654321",
    address: { number: "200" },
  });
  expect(
    canonicalSettings({
      ...base,
      phone: "(11) 91234-5678",
      description: " Description\r\nsecond line ",
    }),
  ).toMatchObject({ phone: initial.phone, description: "Description\nsecond line" });
  expect(
    validateSettingsDraft({
      ...base,
      phone: "bad phone",
      address: { ...base.address, state: "ZZ" },
    }),
  ).toMatchObject({ phone: expect.any(String), state: expect.any(String) });
  expect(
    validateSettingsDraft({ ...base, description: "x".repeat(1001) }).description,
  ).toBeTruthy();
  expect(validateSettingsDraft(base)).toEqual({});
});

test("owner opens settings from navigation and saves only allowed fields using the current version", async ({
  page,
}) => {
  const session = await setup(page);
  let current = structuredClone(initial);
  const writes: SettingsUpdate[] = [];
  await page.route(endpoint, (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    if (route.request().method() === "PUT") {
      const body: SettingsUpdate = route.request().postDataJSON();
      writes.push(body);
      expect(Object.keys(body).sort()).toEqual([
        "address",
        "description",
        "name",
        "phone",
        "version",
      ]);
      expect(route.request().headers()).not.toHaveProperty("idempotency-key");
      current = saved(body, current);
    }
    return route.fulfill({ json: current });
  });
  await page.goto(`/admin/${testShop.slug}`);
  await page
    .getByRole("navigation", { name: "Navegação principal" })
    .getByRole("link", { name: "Configurações", exact: true })
    .click();
  await expect(page).toHaveURL(path);
  await expect(name(page)).toHaveValue(initial.name);
  await expect(save(page)).toBeDisabled();
  await expect(page.getByRole("textbox", { name: /slug|fuso|versão/i })).toHaveCount(0);
  await name(page).fill("Barbearia Renovada");
  await page.getByLabel("Telefone comercial (opcional)", { exact: true }).fill("(11) 98765-4321");
  await save(page).click();
  await expect(page.getByRole("status").filter({ hasText: "Configurações salvas." })).toBeVisible();
  expect(writes[0]).toMatchObject({
    version: 1,
    name: "Barbearia Renovada",
    phone: "+5511987654321",
  });
  await expect(page.locator(".topbar")).toContainText("Barbearia Renovada");
  await expect(page.getByText("Nenhuma alteração pendente.")).toBeVisible();
  await page.getByLabel("Complemento (opcional)", { exact: true }).fill("Sala 3");
  await save(page).click();
  await expect(save(page)).toBeDisabled();
  expect(writes).toHaveLength(2);
  expect(writes[1].version).toBe(2);
  expect(await page.evaluate(() => JSON.stringify({ ...sessionStorage }))).not.toContain(
    "Barbearia Renovada",
  );
});

test("barber cannot see the settings menu or mount the form through a direct URL", async ({
  page,
}) => {
  await setup(page);
  await page.route("**/api/v1/admin/shops/*/access", (route) =>
    route.fulfill({
      json: { role: "barber", professional_id: "66666666-6666-4666-8666-666666666666" },
    }),
  );
  let calls = 0;
  await page.route(endpoint, (route) => {
    calls++;
    return route.abort();
  });
  await page.goto(path);
  await expect(
    page.getByText("Somente o proprietário pode consultar e editar", { exact: false }),
  ).toBeVisible();
  await expect(name(page)).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Configurações", exact: true })).toHaveCount(0);
  expect(calls).toBe(0);
});

test("failed initial read never becomes an editable empty form and retries only GET", async ({
  page,
}) => {
  await setup(page);
  const methods: string[] = [];
  await page.route(endpoint, (route) => {
    methods.push(route.request().method());
    return methods.length === 1
      ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
      : route.fulfill({ json: initial });
  });
  await page.goto(path);
  await expect(page.getByText("Não foi possível carregar as configurações.")).toBeVisible();
  await expect(name(page)).toHaveCount(0);
  await page.getByRole("button", { name: "Tentar novamente", exact: true }).click();
  await expect(name(page)).toHaveValue(initial.name);
  expect(methods).toEqual(["GET", "GET"]);
});

test("legacy shop requires an address and errors link to the corresponding field", async ({
  page,
}) => {
  await setup(page);
  let writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() === "PUT") {
      writes++;
      return route.fulfill({ json: saved(route.request().postDataJSON()) });
    }
    return route.fulfill({ json: { ...initial, address: null } });
  });
  await page.goto(path);
  await expect(page.getByLabel("CEP", { exact: true })).toHaveValue("");
  await name(page).fill("Legacy with address");
  await save(page).click();
  const summary = page
    .getByRole("alert")
    .filter({ hasText: "Confira os campos abaixo antes de salvar." });
  await expect(summary).toBeFocused();
  await summary.getByRole("link", { name: /^CEP:/ }).click();
  await expect(page.getByLabel("CEP", { exact: true })).toBeFocused();
  expect(writes).toBe(0);
  for (const [label, value] of [
    ["CEP", "01310-100"],
    ["Rua ou avenida", "Avenida Paulista"],
    ["Número", "S/N"],
    ["Bairro", "Bela Vista"],
    ["Cidade", "São Paulo"],
  ])
    await page.getByLabel(label, { exact: true }).fill(value);
  await page.getByLabel("Estado", { exact: true }).selectOption("SP");
  await save(page).click();
  await expect(page.getByRole("status").filter({ hasText: "Configurações salvas." })).toBeVisible();
  expect(writes).toBe(1);
});

test("definite validation rejection preserves the draft and permits correction", async ({
  page,
}) => {
  await setup(page);
  await page.route(endpoint, (route) =>
    route.request().method() === "GET"
      ? route.fulfill({ json: initial })
      : route.fulfill({ status: 422, json: { error: { code: "invalid_shop_settings" } } }),
  );
  await page.goto(path);
  await name(page).fill("Keep this draft");
  await save(page).click();
  await expect(page.getByRole("alert").filter({ hasText: "Confira o nome" })).toBeVisible();
  await expect(name(page)).toHaveValue("Keep this draft");
  await expect(name(page)).toBeEnabled();
  await expect(save(page)).toBeEnabled();
});

test("pending PUT disables duplicate submissions, editing, reload and discard", async ({
  page,
}) => {
  await setup(page);
  let writes = 0;
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(endpoint, async (route) => {
    if (route.request().method() === "GET") return route.fulfill({ json: initial });
    writes++;
    await pending;
    await route.fulfill({ json: saved(route.request().postDataJSON()) });
  });
  await page.goto(path);
  await name(page).fill("Pending name");
  await save(page).click();
  await expect(page.getByRole("button", { name: "Salvando…", exact: true })).toBeDisabled();
  await expect(name(page)).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Descartar alterações", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("button", { name: "Recarregar dados", exact: true })).toBeDisabled();
  await page.locator("form").dispatchEvent("submit");
  expect(writes).toBe(1);
  release();
  await expect(page.getByRole("status").filter({ hasText: "Configurações salvas." })).toBeVisible();
  expect(writes).toBe(1);
});

test("conflict requires review and an explicit save, preserving remote changes in untouched fields", async ({
  page,
}) => {
  await setup(page);
  const remote = { ...initial, version: 2, name: "Remote shop name", phone: "+5511987654321" };
  let reads = 0;
  const writes: SettingsUpdate[] = [];
  await page.route(endpoint, (route) => {
    if (route.request().method() === "GET")
      return route.fulfill({ json: ++reads === 1 ? initial : remote });
    writes.push(route.request().postDataJSON());
    return writes.length === 1
      ? route.fulfill({ status: 409, json: { error: { code: "shop_settings_conflict" } } })
      : route.fulfill({ json: saved(writes[1], remote) });
  });
  await page.goto(path);
  await page.getByLabel("Descrição (opcional)", { exact: true }).fill("My new description");
  await save(page).click();
  await expect(name(page)).toBeDisabled();
  await page.getByRole("button", { name: "Conferir dados na API", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Confira antes de continuar" })).toBeVisible();
  expect(writes).toHaveLength(1);
  await page.getByRole("button", { name: "Revisar minhas alterações", exact: true }).click();
  await expect(name(page)).toHaveValue(remote.name);
  await expect(page.getByLabel("Telefone comercial (opcional)", { exact: true })).toHaveValue(
    remote.phone,
  );
  await expect(page.getByLabel("Descrição (opcional)", { exact: true })).toHaveValue(
    "My new description",
  );
  expect(writes).toHaveLength(1);
  await save(page).click();
  await expect(page.getByRole("status").filter({ hasText: "Configurações salvas." })).toBeVisible();
  expect(writes[1]).toMatchObject({
    version: 2,
    name: remote.name,
    phone: remote.phone,
    description: "My new description",
  });
});

test("conflicting draft can be discarded in favor of the API without another PUT", async ({
  page,
}) => {
  await setup(page);
  const remote = { ...initial, version: 2, name: "Another saved name" };
  let reads = 0;
  let writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() === "GET")
      return route.fulfill({ json: ++reads === 1 ? initial : remote });
    writes++;
    return route.fulfill({ status: 409, json: { error: { code: "shop_settings_conflict" } } });
  });
  await page.goto(path);
  await name(page).fill("Local name");
  await save(page).click();
  await page.getByRole("button", { name: "Conferir dados na API" }).click();
  await expect(page.getByRole("table")).toContainText("Another saved name");
  await expect(page.getByRole("table")).toContainText("Local name");
  await page.getByRole("button", { name: "Usar dados atuais da API" }).click();
  await expect(name(page)).toHaveValue(remote.name);
  await expect(save(page)).toBeDisabled();
  expect(writes).toBe(1);
});

test("lost PUT response is recovered by a read that confirms the current data, never automatic resend", async ({
  page,
}) => {
  await setup(page);
  let current = initial;
  let writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() === "GET") return route.fulfill({ json: current });
    writes++;
    current = saved(route.request().postDataJSON());
    return route.abort("failed");
  });
  await page.goto(path);
  await name(page).fill("Saved but response lost");
  await save(page).click();
  await expect(name(page)).toBeDisabled();
  await page.getByRole("button", { name: "Conferir dados na API" }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Dados conferidos na API." }),
  ).toBeVisible();
  await expect(name(page)).toHaveValue(current.name);
  await expect(save(page)).toBeDisabled();
  expect(writes).toBe(1);
});

test("failed confirmation read keeps the uncertain draft locked until a successful read and review", async ({
  page,
}) => {
  await setup(page);
  let reads = 0;
  let writes = 0;
  const sentVersions: number[] = [];
  await page.route(endpoint, (route) => {
    if (route.request().method() === "GET")
      return ++reads === 2
        ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
        : route.fulfill({ json: initial });
    writes++;
    const body: SettingsUpdate = route.request().postDataJSON();
    sentVersions.push(body.version);
    return writes === 1
      ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
      : route.fulfill({ json: saved(body) });
  });
  await page.goto(path);
  await name(page).fill("Unconfirmed name");
  await save(page).click();
  await page.getByRole("button", { name: "Conferir dados na API" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "temporariamente indisponível" }),
  ).toBeVisible();
  await expect(name(page)).toHaveValue("Unconfirmed name");
  await expect(name(page)).toBeDisabled();
  await page.getByRole("button", { name: "Conferir dados na API" }).click();
  await expect(page.getByRole("heading", { name: "Confira antes de continuar" })).toBeVisible();
  expect(writes).toBe(1);
  await page.getByRole("button", { name: "Revisar minhas alterações" }).click();
  expect(writes).toBe(1);
  await save(page).click();
  await expect(page.getByRole("status").filter({ hasText: "Configurações salvas." })).toBeVisible();
  expect(sentVersions).toEqual([1, 1]);
});

test("malformed write response is not success and revoked access hides the private form", async ({
  page,
}) => {
  await setup(page);
  let calls = 0;
  await page.route(endpoint, (route) => {
    if (++calls === 1) return route.fulfill({ json: initial });
    if (route.request().method() === "PUT")
      return route.fulfill({
        json: { ...initial, name: "Wrong tenant", shop_id: otherShop.shop_id, version: 2 },
      });
    return route.fulfill({ status: 403, json: { error: { code: "access_denied" } } });
  });
  await page.goto(path);
  await name(page).fill("My draft");
  await save(page).click();
  await expect(page.getByText("Configurações salvas.", { exact: true })).toHaveCount(0);
  await expect(name(page)).toBeDisabled();
  await page.getByRole("button", { name: "Conferir dados na API" }).click();
  await expect(page.getByRole("link", { name: "Escolher barbearia", exact: true })).toBeVisible();
  await expect(name(page)).toHaveCount(0);
});

test("discard and reload require confirmation while a dirty navigation can be canceled", async ({
  page,
}) => {
  await setup(page);
  let writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() !== "GET") writes++;
    return route.fulfill({ json: initial });
  });
  await page.goto(path);
  await name(page).fill("Unsaved name");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page
    .getByRole("navigation", { name: "Navegação principal" })
    .getByRole("link", { name: "Agenda", exact: true })
    .click();
  await expect(page).toHaveURL(path);
  await expect(name(page)).toHaveValue("Unsaved name");
  await page.getByRole("button", { name: "Descartar alterações", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Descartar alterações?", exact: true });
  await dialog.getByRole("button", { name: "Continuar editando" }).click();
  await expect(name(page)).toHaveValue("Unsaved name");
  await page.getByRole("button", { name: "Recarregar dados", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Descartar e recarregar" }).click();
  await expect(name(page)).toHaveValue(initial.name);
  expect(writes).toBe(0);
});

test("settings remains readable at mobile, tablet, desktop and enlarged text sizes", async ({
  page,
}) => {
  await setup(page);
  await page.route(endpoint, (route) => route.fulfill({ json: initial }));
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(path);
    await expect(name(page)).toHaveValue(initial.name);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: `test-results/settings-${width}.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 390, height: 1000 });
  await page.evaluate(() => {
    document.documentElement.style.fontSize = "200%";
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});
