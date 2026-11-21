import { expect, test, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop } from "./supabase-fixture";
import type { ProfessionalDetails } from "../src/lib/api/professional-edit";

const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}`;
const first = "22222222-2222-4222-8222-222222222222",
  second = "33333333-3333-4333-8333-333333333333";
const id = "55555555-5555-4555-8555-555555555555";
const initial: ProfessionalDetails = {
  id,
  name: "João",
  active: true,
  service_ids: [first],
  revision: "a".repeat(64),
  schedule: {
    interval_minutes: 60,
    buffer_minutes: 0,
    working_hours: [
      { weekday: 1, opens_at_minute: 540, closes_at_minute: 1080 },
      { weekday: 2, opens_at_minute: 540, closes_at_minute: 1080 },
    ],
  },
};
const services = [
  {
    id: first,
    name: "Corte",
    description: "",
    duration_minutes: 40,
    price_cents: 4500,
    currency: "BRL",
    active: true,
  },
  {
    id: second,
    name: "Barba",
    description: "",
    duration_minutes: 30,
    price_cents: 3000,
    currency: "BRL",
    active: true,
  },
];
async function setup(page: Page) {
  await installSupabaseSession(page);
  await mockAdminAccess(page);
  await page.route(base + "/services", (route) => route.fulfill({ json: services }));
  await page.route(base + "/professionals", (route) => route.fulfill({ json: [initial] }));
}
async function open(page: Page) {
  await page.goto(`/admin/${testShop.slug}/barbeiros`);
  await page.getByRole("button", { name: "Editar João", exact: true }).click();
  const d = page.getByRole("dialog", { name: "Editar barbeiro", exact: true });
  await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toHaveValue("João");
  return d;
}
async function day(page: Page, label: string) {
  const d = page.getByRole("dialog", { name: "Editar barbeiro", exact: true });
  await d.locator("summary").filter({ hasText: label }).click();
  return d;
}

test("owner edits services, daily working periods and lunch break, then sees persisted settings", async ({
  page,
}) => {
  await setup(page);
  let current = structuredClone(initial);
  const writes: unknown[] = [];
  await page.route(base + "/professionals", (route) => route.fulfill({ json: [current] }));
  await page.route(base + `/professionals/${id}`, async (route) => {
    if (route.request().method() === "PUT") {
      const body = route.request().postDataJSON();
      writes.push(body);
      current = { id, ...body, revision: "b".repeat(64) };
    }
    await route.fulfill({ json: current });
  });
  await page.setViewportSize({ width: 390, height: 844 });
  const d = await open(page);
  await d.getByLabel("Nome do barbeiro", { exact: true }).fill("João Silva");
  await d.getByRole("checkbox", { name: "Corte · 40 min", exact: true }).uncheck();
  await d.getByRole("checkbox", { name: "Barba · 30 min", exact: true }).check();
  await d.getByLabel("Espaço entre inícios (minutos)").fill("30");
  await d.getByLabel("Tempo extra entre atendimentos (minutos)").fill("10");
  await day(page, "Segunda-feira");
  await d.getByLabel("Fechamento 1 — Segunda-feira", { exact: true }).fill("12:00");
  await d.getByRole("button", { name: "Adicionar período — Segunda-feira", exact: true }).click();
  await d.getByLabel("Fechamento 2 — Segunda-feira", { exact: true }).fill("17:00");
  await day(page, "Terça-feira");
  await d.getByRole("checkbox", { name: "Atendimento: Terça-feira", exact: true }).uncheck();
  await day(page, "Quarta-feira");
  await d.getByRole("checkbox", { name: "Atendimento: Quarta-feira", exact: true }).check();
  await d.getByLabel("Abertura 1 — Quarta-feira", { exact: true }).fill("10:00");
  await d.getByLabel("Fechamento 1 — Quarta-feira", { exact: true }).fill("24:00");
  expect(await d.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(true);
  await page.screenshot({ path: "test-results/professional-edit-mobile.png", fullPage: true });
  await d.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(d).toHaveCount(0);
  expect(writes).toEqual([
    {
      name: "João Silva",
      active: true,
      service_ids: [second],
      revision: "a".repeat(64),
      schedule: {
        interval_minutes: 30,
        buffer_minutes: 10,
        working_hours: [
          { weekday: 1, opens_at_minute: 540, closes_at_minute: 720 },
          { weekday: 1, opens_at_minute: 780, closes_at_minute: 1020 },
          { weekday: 3, opens_at_minute: 600, closes_at_minute: 1440 },
        ],
      },
    },
  ]);
  await expect(
    page.getByLabel("Profissionais cadastrados").getByText("João Silva", { exact: true }),
  ).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "Editar João Silva", exact: true }).click();
  await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toHaveValue("João Silva");
  await expect(d.getByRole("checkbox", { name: "Barba · 30 min", exact: true })).toBeChecked();
  await day(page, "Quarta-feira");
  await expect(d.getByLabel("Fechamento 1 — Quarta-feira", { exact: true })).toHaveValue("24:00");
});

test("overlapping hours are rejected locally without losing the entered values", async ({
  page,
}) => {
  await setup(page);
  let writes = 0;
  await page.route(base + `/professionals/${id}`, async (route) => {
    if (route.request().method() === "PUT") writes++;
    await route.fulfill({ json: initial });
  });
  const d = await open(page);
  await day(page, "Segunda-feira");
  await d.getByRole("button", { name: "Adicionar período — Segunda-feira", exact: true }).click();
  await d.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(d.getByRole("alert")).toContainText("não podem se sobrepor");
  expect(writes).toBe(0);
  await expect(d.getByLabel("Fechamento 1 — Segunda-feira", { exact: true })).toHaveValue("18:00");
  await expect(d.getByLabel("Abertura 2 — Segunda-feira", { exact: true })).toHaveValue("13:00");
});

test("lost response freezes the draft and repeats the same edit while pending save resists dismissal", async ({
  page,
}) => {
  await setup(page);
  let current = structuredClone(initial);
  const writes: unknown[] = [];
  let release = () => {};
  const gate = new Promise<void>((resolve) => (release = resolve));
  await page.route(base + "/professionals", (route) => route.fulfill({ json: [current] }));
  await page.route(base + `/professionals/${id}`, async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ json: current });
      return;
    }
    const body = route.request().postDataJSON();
    writes.push(body);
    current = { id, ...body, revision: "b".repeat(64) };
    if (writes.length === 1) {
      await gate;
      await route.abort("failed");
    } else await route.fulfill({ json: current });
  });
  try {
    const d = await open(page);
    await d.getByLabel("Nome do barbeiro", { exact: true }).fill("João salvo");
    await d.getByRole("button", { name: "Salvar alterações", exact: true }).click();
    await expect.poll(() => writes.length).toBe(1);
    await page.keyboard.press("Escape");
    await expect(d).toBeVisible();
    await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toBeDisabled();
    release();
    await expect(d.getByRole("button", { name: "Repetir confirmação", exact: true })).toBeEnabled();
    await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toHaveValue("João salvo");
    await d.getByRole("button", { name: "Repetir confirmação", exact: true }).click();
    await expect(d).toHaveCount(0);
    expect(writes).toHaveLength(2);
    expect(writes[1]).toEqual(writes[0]);
  } finally {
    release();
  }
});

test("concurrent edit requires explicit reload and uses the current revision on the next save", async ({
  page,
}) => {
  await setup(page);
  let current = structuredClone(initial);
  const revisions: string[] = [];
  await page.route(base + `/professionals/${id}`, async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ json: current });
      return;
    }
    const body = route.request().postDataJSON();
    revisions.push(body.revision);
    if (revisions.length === 1) {
      current = { ...current, name: "Alteração de outra pessoa", revision: "b".repeat(64) };
      await route.fulfill({ status: 409, json: { error: { code: "professional_changed" } } });
    } else {
      current = { id, ...body, revision: "c".repeat(64) };
      await route.fulfill({ json: current });
    }
  });
  const d = await open(page);
  await d.getByLabel("Nome do barbeiro", { exact: true }).fill("Meu rascunho");
  await d.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(d.getByRole("alert")).toContainText("foi alterado");
  await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toHaveValue("Meu rascunho");
  await expect(d.getByRole("button", { name: "Salvar alterações", exact: true })).toBeDisabled();
  page.once("dialog", (dialog) => dialog.accept());
  await d.getByRole("button", { name: "Recarregar cadastro", exact: true }).click();
  await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toHaveValue(
    "Alteração de outra pessoa",
  );
  await d.getByLabel("Nome do barbeiro", { exact: true }).fill("Alteração revisada");
  await d.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(d).toHaveCount(0);
  expect(revisions).toEqual(["a".repeat(64), "b".repeat(64)]);
});

test("owner can clear all services and days and pause the professional without deleting the record", async ({
  page,
}) => {
  await setup(page);
  let current = structuredClone(initial);
  let sent: unknown;
  await page.route(base + "/professionals", (route) => route.fulfill({ json: [current] }));
  await page.route(base + `/professionals/${id}`, async (route) => {
    if (route.request().method() === "PUT") {
      sent = route.request().postDataJSON();
      current = { id, ...route.request().postDataJSON(), revision: "b".repeat(64) };
    }
    await route.fulfill({ json: current });
  });
  const d = await open(page);
  await d.getByRole("checkbox", { name: "Barbeiro ativo", exact: true }).uncheck();
  await d.getByRole("checkbox", { name: "Corte · 40 min", exact: true }).uncheck();
  for (const weekday of ["Segunda-feira", "Terça-feira"]) {
    await day(page, weekday);
    await d.getByRole("checkbox", { name: `Atendimento: ${weekday}`, exact: true }).uncheck();
  }
  await d.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(d).toHaveCount(0);
  expect(sent).toMatchObject({ active: false, service_ids: [], schedule: { working_hours: [] } });
  await expect(
    page.getByLabel("Profissionais cadastrados").getByText("João", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByLabel("Profissionais cadastrados").getByText("Inativo", { exact: true }),
  ).toBeVisible();
});

test("failed detail read offers recovery without showing an assumed schedule", async ({ page }) => {
  await setup(page);
  await page.route(base + `/professionals/${id}`, (route) =>
    route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } }),
  );
  await page.goto(`/admin/${testShop.slug}/barbeiros`);
  await page.getByRole("button", { name: "Editar João", exact: true }).click();
  const d = page.getByRole("dialog", { name: "Editar barbeiro", exact: true });
  await expect(d.getByRole("alert")).toBeVisible();
  await expect(d.getByLabel("Nome do barbeiro", { exact: true })).toHaveCount(0);
  await expect(d.getByRole("button", { name: "Salvar alterações", exact: true })).toHaveCount(0);
});
