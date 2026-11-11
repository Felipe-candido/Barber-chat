import { expect, test } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";
import type { CompletionSettings } from "../src/lib/api/appointment-completion";

const endpoint = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}/appointment-completion`;
const path = `/admin/${testShop.slug}/configuracoes`;
const initial: CompletionSettings = {
  shop_id: testShop.shop_id,
  mode: "automatic",
  version: 1,
  automatic_since: "2026-10-31T00:00:00Z",
  updated_at: "2026-10-31T00:00:00Z",
};

test("owner changes the completion policy and sees persisted manual mode", async ({ page }) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  let current = structuredClone(initial),
    writes = 0,
    ownerCalls = 0;
  await page.route("**/api/v1/admin/shops/*/settings", (route) => {
    ownerCalls++;
    return route.abort();
  });
  await page.route(endpoint, (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    if (route.request().method() === "PUT") {
      writes++;
      expect(route.request().postDataJSON()).toEqual({ mode: "manual", version: 1 });
      current = { ...current, mode: "manual", version: 2, automatic_since: null };
    }
    return route.fulfill({ json: current });
  });
  await page.goto(path);
  await expect(page.getByLabel("Modo de fechamento")).toHaveValue("automatic");
  await page.getByLabel("Modo de fechamento").selectOption("manual");
  await expect(
    page.getByText("Os atendimentos continuam em aberto", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Salvar modo de fechamento" }).click();
  await expect(page.getByText("Modo de fechamento salvo.")).toBeVisible();
  await page.reload();
  await expect(page.getByLabel("Modo de fechamento")).toHaveValue("manual");
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toHaveCount(0);
  expect(writes).toBe(1);
  expect(ownerCalls).toBeGreaterThan(0);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page
    .getByRole("region", { name: "Fechamento dos atendimentos" })
    .screenshot({ path: "test-results/completion-manual-mobile.png" });
  await page.getByLabel("Modo de fechamento").selectOption("automatic");
  await expect(page.getByText("Ao ativar o automático", { exact: false })).toBeVisible();
});

test("lost PUT response requires a readonly confirmation and never repeats the write", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let current = structuredClone(initial),
    writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() === "PUT") {
      writes++;
      current = { ...current, mode: "manual", version: 2, automatic_since: null };
      return route.abort("failed");
    }
    return route.fulfill({ json: current });
  });
  await page.goto(path);
  await expect(page.getByLabel("Modo de fechamento")).toHaveValue("automatic");
  await page.getByLabel("Modo de fechamento").selectOption("manual");
  await page.getByRole("button", { name: "Salvar modo de fechamento" }).click();
  await expect(
    page.getByText("O resultado pode ter sido salvo ou alterado", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Salvar modo de fechamento" })).toBeDisabled();
  await page.getByRole("button", { name: "Consultar modo atual" }).click();
  await expect(page.getByLabel("Modo de fechamento")).toHaveValue("manual");
  await expect(page.getByLabel("Modo de fechamento")).toBeEnabled();
  expect(writes).toBe(1);
});

test("failed initial GET never exposes an editable assumed default", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(endpoint, (route) =>
    route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } }),
  );
  await page.goto(path);
  await expect(page.getByRole("heading", { name: "Fechamento dos atendimentos" })).toBeVisible();
  await expect(page.getByLabel("Modo de fechamento")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Salvar modo de fechamento" })).toBeDisabled();
});

test("unavailable completion endpoint has contextual recovery and no assumed default", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let available = false,
    writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() === "PUT") writes++;
    return available ? route.fulfill({ json: initial }) : route.abort("failed");
  });
  await page.goto(path);
  await expect(
    page.getByText("Não foi possível carregar o modo de fechamento.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByLabel("Modo de fechamento")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Salvar modo de fechamento" })).toBeDisabled();
  available = true;
  await page.getByRole("button", { name: "Consultar modo atual" }).click();
  await expect(page.getByLabel("Modo de fechamento")).toHaveValue("automatic");
  expect(writes).toBe(0);
});

test("concurrent completion change requires reload before an explicit new save", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let current = structuredClone(initial),
    writes = 0;
  await page.route(endpoint, (route) => {
    if (route.request().method() === "PUT") {
      writes++;
      if (writes === 1) {
        current = { ...current, version: 2 };
        return route.fulfill({
          status: 409,
          json: { error: { code: "completion_settings_changed" } },
        });
      }
      expect(route.request().postDataJSON()).toEqual({ mode: "manual", version: 2 });
      current = { ...current, mode: "manual", version: 3, automatic_since: null };
    }
    return route.fulfill({ json: current });
  });
  await page.goto(path);
  await page.getByLabel("Modo de fechamento").selectOption("manual");
  await page.getByRole("button", { name: "Salvar modo de fechamento" }).click();
  await expect(page.getByText("O modo de fechamento mudou.", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "Salvar modo de fechamento" })).toBeDisabled();
  await page.getByRole("button", { name: "Consultar modo atual" }).click();
  await expect(page.getByLabel("Modo de fechamento")).toHaveValue("automatic");
  expect(writes).toBe(1);
  await page.getByLabel("Modo de fechamento").selectOption("manual");
  await page.getByRole("button", { name: "Salvar modo de fechamento" }).click();
  await expect(page.getByText("Modo de fechamento salvo.")).toBeVisible();
  expect(writes).toBe(2);
});
