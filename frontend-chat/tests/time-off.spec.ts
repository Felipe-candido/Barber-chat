import { expect, test, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";
import type { TimeOffBlock } from "../src/lib/api/time-off";

test.use({ timezoneId: "Asia/Tokyo" });

const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}`;
const first = { id: "66666666-6666-4666-8666-666666666666", name: "João" };
const second = { id: "77777777-7777-4777-8777-777777777777", name: "Pedro" };
const data = (blocks: TimeOffBlock[] = [], professionals = [first, second]) => ({
  today: "2026-11-19",
  max_date: "2028-11-19",
  shop_timezone: "America/Sao_Paulo",
  professionals,
  blocks,
});
async function setup(page: Page) {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
}
async function open(page: Page) {
  await page.goto(`/admin/${testShop.slug}`);
  await page.getByRole("button", { name: "Folgas e férias", exact: true }).click();
  return page.getByRole("dialog", { name: "Folgas e férias", exact: true });
}

test("owner blocks inclusive vacation dates in shop time and confirms removal on mobile", async ({
  page,
}) => {
  await setup(page);
  await page.setViewportSize({ width: 390, height: 844 });
  let blocks: TimeOffBlock[] = [];
  let deletes = 0;
  const writes: { url: string; body: unknown }[] = [];
  await page.route(base + "/time-off", (route) => route.fulfill({ json: data(blocks) }));
  await page.route(base + "/professionals/*/time-off/*", async (route) => {
    const parts = new URL(route.request().url()).pathname.split("/");
    if (route.request().method() === "PUT") {
      const body = route.request().postDataJSON();
      writes.push({ url: route.request().url(), body });
      const end = new Date(body.through_date + "T00:00:00Z");
      end.setUTCDate(end.getUTCDate() + 1);
      blocks = [
        {
          id: parts.at(-1)!,
          professional_id: parts.at(-3)!,
          professional_name: second.name,
          starts_at: body.from_date + "T03:00:00Z",
          ends_at: end.toISOString().slice(0, 10) + "T03:00:00Z",
        },
      ];
      await route.fulfill({ json: blocks[0] });
    } else {
      deletes++;
      blocks = [];
      await route.fulfill({ status: 204 });
    }
  });
  const dialog = await open(page);
  await dialog.getByLabel("Barbeiro", { exact: true }).selectOption(second.id);
  await dialog.getByLabel("Primeiro dia sem atendimento").fill("2026-11-24");
  await dialog.getByLabel("Último dia sem atendimento").fill("2026-11-30");
  await dialog.getByRole("button", { name: "Bloquear período", exact: true }).click();
  const periods = dialog.getByRole("region", { name: "Períodos sem atendimento" });
  await expect(periods).toContainText("Pedro");
  await expect(periods).toContainText(/24.*nov.*2026/);
  await expect(periods).toContainText(/30.*nov.*2026/);
  expect(writes).toHaveLength(1);
  expect(writes[0].body).toEqual({ from_date: "2026-11-24", through_date: "2026-11-30" });
  expect(writes[0].url).toContain(`/professionals/${second.id}/time-off/`);
  await periods.getByRole("button", { name: "Remover período" }).click();
  await periods.getByRole("button", { name: "Manter folga" }).click();
  expect(deletes).toBe(0);
  await periods.getByRole("button", { name: "Remover período" }).click();
  await periods.getByRole("button", { name: "Liberar dias" }).click();
  await expect(periods).toContainText("Nenhuma folga programada");
  expect(deletes).toBe(1);
  expect(await dialog.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(true);
  await page.screenshot({ path: "test-results/time-off-mobile.png", fullPage: true });
});

test("barber manages only their own days even in a browser using another timezone", async ({
  page,
}) => {
  await setup(page);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.route(base + "/access", (route) =>
    route.fulfill({ json: { role: "barber", professional_id: first.id } }),
  );
  await page.route(base + "/time-off", (route) => route.fulfill({ json: data([], [first]) }));
  let body: unknown;
  let path = "";
  await page.route(base + "/professionals/*/time-off/*", async (route) => {
    body = route.request().postDataJSON();
    path = route.request().url();
    await route.fulfill({
      json: {
        id: path.split("/").at(-1),
        professional_id: first.id,
        professional_name: first.name,
        starts_at: "2026-11-19T03:00:00Z",
        ends_at: "2026-11-20T03:00:00Z",
      },
    });
  });
  const dialog = await open(page);
  await expect(dialog.getByText("Suas folgas:", { exact: false })).toBeVisible();
  await expect(dialog.getByLabel("Barbeiro", { exact: true })).toHaveCount(0);
  await expect(dialog.getByLabel("Primeiro dia sem atendimento")).toHaveValue("2026-11-19");
  await dialog.getByRole("button", { name: "Bloquear período", exact: true }).click();
  await expect.poll(() => body).toEqual({ from_date: "2026-11-19", through_date: "2026-11-19" });
  expect(path).toContain(`/professionals/${first.id}/`);
});

test("appointments prevent a block without clearing the dates or claiming success", async ({
  page,
}) => {
  await setup(page);
  await page.route(base + "/time-off", (route) => route.fulfill({ json: data() }));
  let writes = 0;
  await page.route(base + "/professionals/*/time-off/*", async (route) => {
    writes++;
    await route.fulfill({ status: 409, json: { error: { code: "time_off_has_appointments" } } });
  });
  const dialog = await open(page);
  await dialog.getByLabel("Primeiro dia sem atendimento").fill("2026-12-01");
  await dialog.getByRole("button", { name: "Bloquear período", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Já há atendimentos");
  await expect(dialog.getByLabel("Primeiro dia sem atendimento")).toHaveValue("2026-12-01");
  await expect(dialog.getByLabel("Primeiro dia sem atendimento")).toBeEnabled();
  expect(writes).toBe(1);
});

test("pending save resists dismissal and a lost response retries the exact same resource", async ({
  page,
}) => {
  await setup(page);
  await page.route(base + "/time-off", (route) => route.fulfill({ json: data() }));
  let release = () => {};
  const gate = new Promise<void>((resolve) => (release = resolve));
  const attempts: { url: string; body: unknown }[] = [];
  await page.route(base + "/professionals/*/time-off/*", async (route) => {
    attempts.push({ url: route.request().url(), body: route.request().postDataJSON() });
    if (attempts.length === 1) {
      await gate;
      await route.abort("failed");
    } else
      await route.fulfill({
        json: {
          id: route.request().url().split("/").at(-1),
          professional_id: first.id,
          professional_name: first.name,
          starts_at: "2026-11-19T03:00:00Z",
          ends_at: "2026-11-20T03:00:00Z",
        },
      });
  });
  try {
    const dialog = await open(page);
    await dialog.getByRole("button", { name: "Bloquear período", exact: true }).click();
    await expect.poll(() => attempts.length).toBe(1);
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel("Primeiro dia sem atendimento")).toBeDisabled();
    release();
    await expect(
      dialog.getByRole("button", { name: "Repetir confirmação", exact: true }),
    ).toBeEnabled();
    await expect(dialog.getByLabel("Primeiro dia sem atendimento")).toBeDisabled();
    await dialog.getByRole("button", { name: "Repetir confirmação", exact: true }).click();
    await expect.poll(() => attempts.length).toBe(2);
    expect(attempts[0]).toEqual(attempts[1]);
  } finally {
    release();
  }
});

test("foreign professional data is rejected before a barber sees management controls", async ({
  page,
}) => {
  await setup(page);
  await page.route(base + "/access", (route) =>
    route.fulfill({ json: { role: "barber", professional_id: first.id } }),
  );
  await page.route(base + "/time-off", (route) => route.fulfill({ json: data() }));
  const dialog = await open(page);
  await expect(dialog.getByRole("alert")).toContainText("Não conseguimos confirmar as folgas");
  await expect(dialog.getByRole("button", { name: "Bloquear período", exact: true })).toHaveCount(
    0,
  );
  await expect(dialog.getByText("Pedro", { exact: true })).toHaveCount(0);
});
