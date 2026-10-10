import { expect, test } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";

const service = {
  id: "22222222-2222-4222-8222-222222222222",
  name: "Corte",
  description: "",
  duration_minutes: 40,
  price_cents: 5000,
  currency: "BRL",
  active: true,
};

test("admin registers a barber with services and initial calendar in the selected shop", async ({
  page,
}) => {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  const items: object[] = [];
  let submitted: unknown;
  await page.route(
    "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug + "/**",
    async (route) => {
      const request = route.request();
      if (request.url().endsWith("/access")) return route.fallback();
      if (request.method() === "OPTIONS") {
        await route.fulfill({
          status: 204,
          headers: {
            "Access-Control-Allow-Origin": "http://127.0.0.1:3100",
            "Access-Control-Allow-Methods": "GET, POST",
            "Access-Control-Allow-Headers": "Content-Type, Authorization",
          },
        });
        return;
      }
      expect(request.headers().authorization).toBe("Bearer " + session.accessToken);
      if (request.url().endsWith("/services")) {
        await route.fulfill({ json: [service] });
        return;
      }
      if (request.method() === "POST") {
        submitted = request.postDataJSON();
        const item = {
          id: "44444444-4444-4444-8444-444444444444",
          name: "Barbeiro novo",
          active: true,
          service_ids: [service.id],
        };
        items.push(item);
        await route.fulfill({ status: 201, json: item });
        return;
      }
      await route.fulfill({ json: items });
    },
  );
  await page.goto("/admin/" + testShop.slug + "/barbeiros");
  await page.getByLabel("Nome do barbeiro").fill("Barbeiro novo");
  await page.getByRole("checkbox", { name: "Corte · 40 min" }).check();
  await page.getByRole("button", { name: "Cadastrar barbeiro", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Barbeiro cadastrado" })).toBeVisible();
  await expect(
    page.getByLabel("Profissionais cadastrados").getByText("Barbeiro novo"),
  ).toBeVisible();
  expect(submitted).toEqual({
    name: "Barbeiro novo",
    service_ids: [service.id],
    schedule: {
      interval_minutes: 60,
      buffer_minutes: 0,
      weekdays: [1, 2, 3, 4, 5, 6],
      opens_at_minute: 570,
      closes_at_minute: 1110,
    },
  });
  await expect(page.getByRole("link", { name: "Testar agendamento no chat" })).toHaveAttribute(
    "href",
    "/b/" + testShop.slug,
  );
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
});

test("a barber requires an existing active service", async ({ page }) => {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route("http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug + "/**", (route) =>
    route.request().url().endsWith("/access") ? route.fallback() : route.fulfill({ json: [] }),
  );
  await page.goto("/admin/" + testShop.slug + "/barbeiros");
  await expect(page.getByText("Cadastre primeiro um", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "Cadastrar barbeiro", exact: true })).toHaveCount(
    0,
  );
});

test("slow professional creation keeps one submission pending beyond ten seconds", async ({
  page,
}) => {
  await page.clock.install();
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let submissions = 0;
  const item = {
    id: "44444444-4444-4444-8444-444444444444",
    name: "Cadastro lento",
    active: true,
    service_ids: [service.id],
  };
  await page.route(
    "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug + "/**",
    async (route) => {
      const request = route.request();
      if (request.url().endsWith("/access")) return route.fallback();
      if (request.method() === "OPTIONS") {
        await route.fulfill({
          status: 204,
          headers: {
            "Access-Control-Allow-Origin": "http://127.0.0.1:3100",
            "Access-Control-Allow-Methods": "GET, POST",
            "Access-Control-Allow-Headers": "Content-Type, Authorization",
          },
        });
      } else if (request.url().endsWith("/services")) {
        await route.fulfill({ json: [service] });
      } else if (request.method() === "POST") {
        submissions++;
        await gate;
        await route.fulfill({ status: 201, json: item }).catch(() => {});
      } else {
        await route.fulfill({ json: submissions ? [item] : [] });
      }
    },
  );
  try {
    await page.goto("/admin/" + testShop.slug + "/barbeiros");
    await page.getByLabel("Nome do barbeiro").fill("Cadastro lento");
    await page.getByRole("checkbox", { name: "Corte · 40 min" }).check();
    await page.getByRole("button", { name: "Cadastrar barbeiro", exact: true }).click();
    await expect.poll(() => submissions).toBe(1);
    await page.clock.fastForward(11_000);
    await expect(page.getByRole("button", { name: "Cadastrando…", exact: true })).toBeDisabled();
    await expect(
      page.getByRole("alert").filter({ hasText: "demorou mais que o esperado" }),
    ).toHaveCount(0);
    release();
    await expect(page.getByRole("status").filter({ hasText: "Barbeiro cadastrado" })).toBeVisible();
    expect(submissions).toBe(1);
  } finally {
    release();
  }
});
