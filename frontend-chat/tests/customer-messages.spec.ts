import { expect, test } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

test("unexpected server errors show a useful message without internal details", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(`**/api/v1/admin/shops/${testShop.slug}/services`, (route) =>
    route.fulfill({
      status: 500,
      json: {
        error: {
          code: "unexpected_database_failure",
          message: "SQL connection failed on internal-host",
        },
      },
    }),
  );
  await page.goto(`/admin/${testShop.slug}/servicos`);
  const alert = page.getByRole("main").getByRole("alert");
  await expect(alert).toContainText("Não foi possível concluir seu pedido");
  await expect(alert).not.toContainText(/API|HTTP|SQL|internal-host|database_failure/);
  await expect(page.getByRole("button", { name: "Tentar novamente" })).toBeVisible();
});

test("connection errors explain how to continue without configuration instructions", async ({
  page,
}) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(`**/api/v1/admin/shops/${testShop.slug}/services`, (route) =>
    route.abort("failed"),
  );
  await page.goto(`/admin/${testShop.slug}/servicos`);
  const alert = page.getByRole("main").getByRole("alert");
  await expect(alert).toContainText("Confira sua internet");
  await expect(alert).not.toContainText(/API|HTTP|CORS|backend|FRONTEND_ORIGIN/);
});
