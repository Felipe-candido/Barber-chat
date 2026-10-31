import { expect, test, type Page } from "@playwright/test";
import {
  emptyAgenda,
  installSupabaseSession,
  mockAdminAccess,
  otherShop,
  testIdentity,
  testShop,
} from "./supabase-fixture";
import type { Product } from "../src/lib/api/products";
import type { Sale, SaleInput } from "../src/lib/api/sales";

const base = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug;
const root = "/admin/" + testShop.slug;
const productID = "55555555-5555-4555-8555-555555555555",
  professionalID = "66666666-6666-4666-8666-666666666666",
  saleID = "77777777-7777-4777-8777-777777777777",
  itemID = "88888888-8888-4888-8888-888888888888",
  movementID = "99999999-9999-4999-8999-999999999999";
const product: Product = {
  id: productID,
  shop_id: testShop.shop_id,
  name: "Pomada modeladora",
  description: "Acabamento natural",
  price_cents: 3000,
  currency: "BRL",
  active: true,
  version: 1,
  quantity_on_hand: 5,
  minimum_quantity: 2,
  stock_version: 1,
  low_stock: false,
  created_at: "2026-10-01T12:00:00Z",
  updated_at: "2026-10-01T12:00:00Z",
};
function receipt(body: SaleInput, barber = false): Sale {
  return {
    id: saleID,
    shop_id: testShop.shop_id,
    recorded_by_user_id: testIdentity.user_id,
    professional_id: body.professional_id ?? (barber ? professionalID : null),
    appointment_id: body.appointment_id ?? null,
    professional_name: body.professional_id || barber ? "Palma" : "",
    items: body.items.map((i) => ({
      id: itemID,
      product_id: i.product_id,
      product_name: product.name,
      quantity: i.quantity,
      unit_price_cents: i.expected_unit_price_cents,
      line_total_cents: i.quantity * i.expected_unit_price_cents,
      currency: "BRL",
    })),
    status: "recorded",
    currency: "BRL",
    total_cents: body.items.reduce((sum, i) => sum + i.quantity * i.expected_unit_price_cents, 0),
    version: 1,
    sold_at: "2026-10-30T15:00:00Z",
    replayed: false,
    cancellation: null,
  };
}
const initialSale = () =>
  receipt({
    items: [
      {
        product_id: productID,
        quantity: 1,
        expected_product_version: 1,
        expected_unit_price_cents: 3000,
      },
    ],
  });
async function setup(page: Page, barber = false) {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page);
  if (barber)
    await page.route(base + "/access", (route) =>
      route.fulfill({ json: { role: "barber", professional_id: professionalID } }),
    );
  await page.route(base + "/professionals", (route) =>
    route.fulfill({ json: [{ id: professionalID, name: "Palma", active: true, service_ids: [] }] }),
  );
  await page.route(base + "/products**", (route) =>
    route.fulfill({ json: { items: [product], next_cursor: null } }),
  );
  await page.route(base + "/sales**", (route) =>
    route.fulfill({ json: { items: [], next_cursor: "", timezone: "America/Sao_Paulo" } }),
  );
  return session;
}
async function selectAndReview(page: Page, amount = "1") {
  await page.getByRole("button", { name: "Registrar venda", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Adicionar " + product.name, exact: true }).click();
  await dialog.getByLabel("Quantidade de " + product.name).fill(amount);
  await dialog.getByRole("button", { name: "Revisar venda", exact: true }).click();
  return dialog;
}

test("owner creates a product with integer cents and edits using its version", async ({ page }) => {
  const session = await setup(page);
  let current = product,
    created = false,
    posts = 0,
    puts = 0;
  await page.route(base + "/products**", async (route) => {
    const r = route.request();
    expect(r.headers().authorization).toBe("Bearer " + session.accessToken);
    if (r.method() === "POST") {
      posts++;
      const body = r.postDataJSON();
      expect(body).toEqual({
        name: "Cera",
        description: "Fixação forte",
        price_cents: 3990,
        initial_quantity: 8,
        minimum_quantity: 2,
      });
      expect(r.headers()["idempotency-key"]).toMatch(/^[a-f0-9]{64}$/);
      current = { ...product, ...body, quantity_on_hand: 8 };
      created = true;
      return route.fulfill({
        status: 201,
        json: { product: current, movement_id: movementID, replayed: false },
      });
    }
    if (r.method() === "PUT") {
      puts++;
      expect(r.postDataJSON()).toMatchObject({
        name: "Cera",
        price_cents: 4500,
        active: false,
        expected_version: 1,
      });
      current = { ...current, ...r.postDataJSON(), version: 2 };
      return route.fulfill({ json: current });
    }
    return route.fulfill({ json: { items: created ? [current] : [], next_cursor: null } });
  });
  await page.goto(root + "/produtos");
  await page.getByRole("button", { name: "Adicionar produto", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do produto").fill("Cera");
  await dialog.getByLabel("Descrição (opcional)").fill("Fixação forte");
  await dialog.getByLabel("Preço de venda (R$)").fill("39,90");
  await dialog.getByLabel("Quantidade inicial").fill("8");
  await dialog.getByLabel("Estoque mínimo").fill("2");
  await dialog.getByRole("button", { name: "Adicionar produto", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Cera", exact: true })).toBeVisible();
  expect(posts).toBe(1);
  await page.getByRole("button", { name: "Editar", exact: true }).click();
  dialog = page.getByRole("dialog");
  await dialog.getByLabel("Preço de venda (R$)").fill("45,00");
  await dialog.getByLabel("Produto ativo").uncheck();
  await dialog.getByRole("button", { name: "Salvar produto", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(puts).toBe(1);
});

test("product creation survives a lost response and reload without a new key", async ({ page }) => {
  await setup(page);
  const sent: { key: string; body: unknown }[] = [];
  await page.route(base + "/products", async (route) => {
    if (route.request().method() !== "POST")
      return route.fulfill({ json: { items: [product], next_cursor: null } });
    const r = route.request();
    sent.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
    return sent.length === 1
      ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
      : route.fulfill({
          json: { product: { ...product, name: "Cera" }, movement_id: null, replayed: true },
        });
  });
  await page.goto(root + "/produtos");
  await page.getByRole("button", { name: "Adicionar produto", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do produto").fill("Cera");
  await dialog.getByLabel("Preço de venda (R$)").fill("20,00");
  await dialog.getByRole("button", { name: "Adicionar produto", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "Adicionar produto", exact: true }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Nome do produto")).toHaveValue("Cera");
  await expect(dialog.getByLabel("Nome do produto")).toBeDisabled();
  expect(sent.length).toBe(1);
  await dialog.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(dialog).toHaveCount(0);
  expect(sent).toHaveLength(2);
  expect(sent[1]).toEqual(sent[0]);
});

test("stock receipt recovery preserves the delta and minimum uses stock version", async ({
  page,
}) => {
  await setup(page);
  let current = product,
    posts = 0;
  const sent: { key: string; body: unknown }[] = [];
  await page.route(base + "/products/**", async (route) => {
    const r = route.request();
    if (r.method() === "PUT") {
      expect(r.postDataJSON()).toEqual({ minimum_quantity: 4, expected_stock_version: 2 });
      current = { ...current, minimum_quantity: 4, low_stock: false, stock_version: 3 };
      return route.fulfill({ json: current });
    }
    if (r.method() === "POST") {
      posts++;
      sent.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
      expect(r.postDataJSON()).toEqual({
        operation: "receipt",
        quantity_delta: 3,
        reason: "Compra",
      });
      current = { ...product, quantity_on_hand: 8, stock_version: 2 };
      return posts === 1
        ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
        : route.fulfill({ json: { product: current, movement_id: movementID, replayed: true } });
    }
    return route.fulfill({
      json: {
        items: posts
          ? [
              {
                id: movementID,
                product_id: productID,
                actor_id: testIdentity.user_id,
                kind: "receipt",
                quantity_delta: 3,
                reason: "Compra",
                created_at: "2026-10-31T12:00:00Z",
                sale_item_id: null,
                reverses_movement_id: null,
              },
            ]
          : [],
        next_cursor: null,
      },
    });
  });
  await page.goto(root + "/produtos");
  await page.getByRole("button", { name: "Estoque", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Quantidade", { exact: true }).fill("3");
  await dialog.getByLabel("Motivo da movimentação").fill("Compra");
  await dialog.getByRole("button", { name: "Registrar movimentação" }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await dialog.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(dialog.getByText("8 unidades disponíveis")).toBeVisible();
  expect(sent[1]).toEqual(sent[0]);
  await dialog.locator("summary", { hasText: "Alterar estoque mínimo" }).click();
  await dialog.getByLabel("Estoque mínimo", { exact: true }).fill("4");
  await dialog.getByRole("button", { name: "Salvar estoque mínimo" }).click();
  await expect(dialog.getByText("Mínimo: 4")).toBeVisible();
});

test("stale product edit asks for a fresh read before another write", async ({ page }) => {
  await setup(page);
  let writes = 0;
  await page.route(base + "/products/" + productID, (route) => {
    writes++;
    return route.fulfill({ status: 409, json: { error: { code: "product_version_conflict" } } });
  });
  await page.goto(root + "/produtos");
  await page.getByRole("button", { name: "Editar", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Preço de venda (R$)").fill("42,00");
  await dialog.getByRole("button", { name: "Salvar produto" }).click();
  await expect(dialog.getByRole("button", { name: "Conferir lista atual" })).toBeVisible();
  await expect(dialog.getByLabel("Preço de venda (R$)")).toBeDisabled();
  expect(writes).toBe(1);
});

test("sale is reviewed before sending and returns its original item receipt", async ({ page }) => {
  const session = await setup(page);
  let saved: Sale | null = null,
    posts = 0;
  await page.route(base + "/sales**", (route) => {
    if (route.request().method() === "POST") {
      posts++;
      const body = route.request().postDataJSON();
      expect(body).toEqual({
        items: [
          {
            product_id: productID,
            quantity: 2,
            expected_product_version: 1,
            expected_unit_price_cents: 3000,
          },
        ],
      });
      expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
      saved = receipt(body);
      return route.fulfill({ status: 201, json: saved });
    }
    return route.fulfill({
      json: { items: saved ? [saved] : [], next_cursor: "", timezone: "America/Sao_Paulo" },
    });
  });
  await page.goto(root + "/vendas");
  const dialog = await selectAndReview(page, "2");
  expect(posts).toBe(0);
  await expect(dialog.getByRole("region", { name: "Revisão da venda" })).toContainText("R$ 60,00");
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "Comprovante da venda" })).toContainText(
    "2 un. × R$ 30,00",
  );
  expect(posts).toBe(1);
});

test("changed price requires another selection and explicit confirmation", async ({ page }) => {
  await setup(page);
  let p = product;
  const requests: { key: string; body: SaleInput }[] = [];
  await page.route(base + "/products**", (route) =>
    route.fulfill({ json: { items: [p], next_cursor: null } }),
  );
  await page.route(base + "/sales", (route) => {
    const r = route.request();
    requests.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
    if (requests.length === 1) {
      p = { ...product, price_cents: 3500, version: 2 };
      return route.fulfill({ status: 409, json: { error: { code: "sale_price_changed" } } });
    }
    return route.fulfill({ status: 201, json: receipt(r.postDataJSON()) });
  });
  await page.goto(root + "/vendas");
  const dialog = await selectAndReview(page);
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(
    dialog.getByRole("button", { name: "Limpar seleção e atualizar produtos" }),
  ).toBeVisible();
  expect(requests.length).toBe(1);
  await dialog.getByRole("button", { name: "Limpar seleção e atualizar produtos" }).click();
  await dialog.getByRole("button", { name: "Adicionar " + product.name, exact: true }).click();
  await dialog.getByRole("button", { name: "Revisar venda", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "Revisão da venda" })).toContainText("R$ 35,00");
  expect(requests.length).toBe(1);
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "Comprovante da venda" })).toBeVisible();
  expect(requests[1].key).not.toBe(requests[0].key);
  expect(requests[1].body.items[0]).toMatchObject({
    expected_product_version: 2,
    expected_unit_price_cents: 3500,
  });
});

for (const malformed of [false, true])
  test(`sale recovery after ${malformed ? "foreign receipt" : "lost response"} and reload keeps its key and body`, async ({
    page,
  }) => {
    await setup(page);
    const requests: { key: string; body: SaleInput }[] = [];
    await page.route(base + "/sales", (route) => {
      const r = route.request();
      requests.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
      const saved = receipt(r.postDataJSON());
      if (requests.length === 1)
        return malformed
          ? route.fulfill({
              status: 201,
              json: {
                ...saved,
                shop_id: "44444444-4444-4444-8444-444444444444",
                professional_name: "Private other shop",
              },
            })
          : route.fulfill({ status: 503, json: { error: { code: "sale_unavailable" } } });
      return route.fulfill({ json: { ...saved, replayed: true } });
    });
    await page.goto(root + "/vendas");
    let dialog = await selectAndReview(page);
    await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
    await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
    await expect(page.getByText("Private other shop")).toHaveCount(0);
    if (!malformed) {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.screenshot({
        path: "test-results/commerce-sale-recovery-mobile.png",
        fullPage: true,
      });
    }
    await page.reload();
    await page.getByRole("button", { name: "Registrar venda", exact: true }).click();
    dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Confirmar venda", exact: true })).toHaveCount(
      0,
    );
    expect(requests.length).toBe(1);
    await dialog.getByRole("button", { name: "Repetir confirmação" }).click();
    await expect(dialog.getByRole("heading", { name: "Venda recuperada" })).toBeVisible();
    expect(requests).toHaveLength(2);
    expect(requests[1]).toEqual(requests[0]);
  });

for (const restock of [true, false])
  test(`owner cancellation requires an explicit stock decision (${restock})`, async ({ page }) => {
    await setup(page);
    let sale = initialSale(),
      posts = 0;
    await page.route(base + "/sales**", (route) => {
      const r = route.request(),
        url = new URL(r.url());
      if (r.method() === "POST") {
        posts++;
        expect(r.postDataJSON()).toEqual({
          expected_version: 1,
          reason: "Registro incorreto",
          restock,
        });
        sale = {
          ...sale,
          status: "cancelled",
          version: 2,
          cancellation: {
            actor_id: testIdentity.user_id,
            reason: "Registro incorreto",
            restock,
            cancelled_at: "2026-10-31T15:00:00Z",
          },
        };
        return route.fulfill({ json: sale });
      }
      return route.fulfill({
        json: url.pathname.endsWith(saleID)
          ? sale
          : { items: [sale], next_cursor: "", timezone: "America/Sao_Paulo" },
      });
    });
    await page.goto(root + "/vendas");
    await page.getByRole("button", { name: "Ver venda", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("summary", { hasText: "Anular venda" }).click();
    await dialog.getByLabel("Motivo da anulação").fill("Registro incorreto");
    await dialog.getByRole("button", { name: "Confirmar anulação" }).click();
    expect(posts).toBe(0);
    await dialog
      .getByRole("radio", { name: restock ? /Devolver ao estoque/ : /Manter fora do estoque/ })
      .check();
    await dialog.getByRole("button", { name: "Confirmar anulação" }).click();
    await expect(
      dialog.getByText(
        restock
          ? "As quantidades foram devolvidas ao estoque."
          : "As quantidades continuam retiradas do estoque.",
      ),
    ).toBeVisible();
    expect(posts).toBe(1);
  });

test("sales filters and pagination preserve the shop period and status", async ({ page }) => {
  await setup(page);
  let initial = true;
  const queries: URLSearchParams[] = [];
  await page.route(base + "/sales**", (route) => {
    const q = new URL(route.request().url()).searchParams;
    queries.push(q);
    const more = q.get("cursor") === "next-page";
    return route.fulfill({
      json: {
        items: more
          ? [{ ...initialSale(), id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }]
          : [initialSale()],
        next_cursor: initial && !more ? "next-page" : "",
        timezone: "America/Sao_Paulo",
      },
    });
  });
  await page.goto(root + "/vendas");
  await page.getByRole("button", { name: "Carregar mais vendas" }).click();
  await expect(page.getByRole("button", { name: "Ver venda", exact: true })).toHaveCount(2);
  const previous = queries.at(-1)!;
  expect(previous.get("cursor")).toBe("next-page");
  initial = false;
  await page.getByLabel("De", { exact: true }).fill("2026-10-01");
  await page.getByLabel("Até", { exact: true }).fill("2026-10-31");
  await page.getByLabel("Situação da venda").selectOption("recorded");
  await page.getByRole("button", { name: "Filtrar vendas" }).click();
  await expect.poll(() => queries.at(-1)?.get("status")).toBe("recorded");
  expect(queries.at(-1)?.get("cursor")).toBeNull();
  expect(queries.at(-1)?.get("from")).toBe("2026-10-01");
  expect(queries.at(-1)?.get("through")).toBe("2026-10-31");
});

for (const barber of [false, true])
  test(`subscription denial preserves sale review for ${barber ? "barber" : "owner"}`, async ({
    page,
  }) => {
    await setup(page, barber);
    let posts = 0;
    await page.route(base + "/sales", (route) => {
      posts++;
      return route.fulfill({
        status: barber ? 403 : 402,
        json: { error: { code: "shop_subscription_required" } },
      });
    });
    await page.goto(root + "/vendas");
    const dialog = await selectAndReview(page);
    await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
    await expect(dialog.getByRole("region", { name: "Revisão da venda" })).toBeVisible();
    if (barber) {
      await expect(dialog.getByText(/Peça ao proprietário para regularizar/)).toBeVisible();
      await expect(dialog.getByRole("link", { name: "Gerenciar assinatura" })).toHaveCount(0);
    } else
      await expect(dialog.getByRole("link", { name: "Gerenciar assinatura" })).toHaveAttribute(
        "target",
        "_blank",
      );
    expect(posts).toBe(1);
  });

test("barber reads products and sells in own scope without owner controls", async ({ page }) => {
  await setup(page, true);
  let posts = 0;
  await page.route(base + "/sales**", (route) => {
    if (route.request().method() === "POST") {
      posts++;
      const body = route.request().postDataJSON();
      expect(body.professional_id).toBeUndefined();
      return route.fulfill({ status: 201, json: receipt(body, true) });
    }
    return route.fulfill({
      json: new URL(route.request().url()).pathname.endsWith(saleID)
        ? { ...initialSale(), professional_id: professionalID, professional_name: "Palma" }
        : {
            items: [
              { ...initialSale(), professional_id: professionalID, professional_name: "Palma" },
            ],
            next_cursor: "",
            timezone: "America/Sao_Paulo",
          },
    });
  });
  await page.goto(root + "/produtos");
  await expect(page.getByRole("heading", { name: product.name, exact: true })).toBeVisible();
  for (const name of ["Adicionar produto", "Editar", "Estoque"])
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(0);
  await expect(page.getByLabel("Incluir inativos")).toHaveCount(0);
  await page.goto(root + "/vendas");
  await expect(page.getByLabel("Filtrar por barbeiro")).toHaveCount(0);
  await page.getByRole("button", { name: "Ver venda", exact: true }).click();
  await expect(
    page.getByRole("dialog").locator("summary", { hasText: "Anular venda" }),
  ).toHaveCount(0);
  await page.getByRole("dialog").getByRole("button", { name: "Fechar", exact: true }).click();
  const dialog = await selectAndReview(page);
  await expect(dialog.getByLabel("Barbeiro responsável")).toHaveCount(0);
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "Comprovante da venda" })).toContainText("Palma");
  expect(posts).toBe(1);
});

test("agenda opens the shared sale form with appointment and professional fixed", async ({
  page,
}) => {
  await setup(page);
  const appointment = {
    id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
    shop_name: testShop.name,
    status: "scheduled",
    service_name: "Corte",
    duration_minutes: 40,
    price_cents: 5000,
    currency: "BRL",
    professional_id: professionalID,
    professional_name: "Palma",
    starts_at: "2026-10-03T12:30:00Z",
    ends_at: "2026-10-03T13:30:00Z",
    shop_timezone: "America/Sao_Paulo",
    customer_name: "Cliente Teste",
    customer_phone: "+5511912345678",
  };
  await page.route(base + "/appointments*", (route) =>
    route.fulfill({ json: { ...emptyAgenda, appointments: [appointment] } }),
  );
  await page.route(base + "/sales", (route) => {
    const body = route.request().postDataJSON();
    expect(body.appointment_id).toBe(appointment.id);
    expect(body.professional_id).toBe(professionalID);
    expect(JSON.stringify(body)).not.toContain(appointment.customer_phone);
    return route.fulfill({ status: 201, json: receipt(body) });
  });
  await page.goto(root);
  await page.locator(".agenda-list-item").click();
  await page.getByRole("button", { name: "Registrar produtos neste atendimento" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Agendamento de Cliente Teste");
  await expect(dialog.getByLabel("Barbeiro responsável")).toHaveCount(0);
  await dialog.getByRole("button", { name: "Adicionar " + product.name, exact: true }).click();
  await dialog.getByRole("button", { name: "Revisar venda", exact: true }).click();
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "Comprovante da venda" })).toContainText(
    appointment.id,
  );
});

test("products and sale dialogs fit mobile and keep keyboard focus", async ({ page }) => {
  await setup(page);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(root + "/produtos");
  await expect(page.getByRole("heading", { name: product.name, exact: true })).toBeVisible();
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 920 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Adicionar produto", exact: true }).click();
  await page.getByLabel("Nome do produto").fill("Cera");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Adicionar produto", exact: true })).toBeFocused();
  await page.screenshot({ path: "test-results/commerce-products-mobile.png", fullPage: true });
  await page.goto(root + "/vendas");
  const dialog = await selectAndReview(page, "2");
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 920 });
    expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "test-results/commerce-sale-review-mobile.png", fullPage: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: "test-results/commerce-sale-review-desktop.png", fullPage: true });
  expect(errors).toEqual([]);
});

test("insufficient stock does not retry automatically and refresh disables an empty product", async ({
  page,
}) => {
  await setup(page);
  let current = product,
    posts = 0;
  await page.route(base + "/products**", (route) =>
    route.fulfill({ json: { items: [current], next_cursor: null } }),
  );
  await page.route(base + "/sales", (route) => {
    posts++;
    current = { ...product, quantity_on_hand: 0, stock_version: 2, low_stock: true };
    return route.fulfill({ status: 409, json: { error: { code: "insufficient_stock" } } });
  });
  await page.goto(root + "/vendas");
  const dialog = await selectAndReview(page);
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(dialog.getByText(/O estoque é insuficiente/)).toBeVisible();
  await dialog.getByRole("button", { name: "Limpar seleção e atualizar produtos" }).click();
  await expect(
    dialog.getByRole("button", { name: "Adicionar " + product.name, exact: true }),
  ).toBeDisabled();
  expect(posts).toBe(1);
  await expect(dialog.getByRole("region", { name: "Comprovante da venda" })).toHaveCount(0);
});

test("a saved cancellation can be recovered after reload without choosing restock again", async ({
  page,
}) => {
  await setup(page);
  let sale = initialSale();
  const requests: { key: string; body: unknown }[] = [];
  await page.route(base + "/sales**", (route) => {
    const r = route.request();
    if (r.method() === "POST") {
      requests.push({ key: r.headers()["idempotency-key"], body: r.postDataJSON() });
      sale = {
        ...sale,
        status: "cancelled",
        version: 2,
        cancellation: {
          actor_id: testIdentity.user_id,
          reason: "Devolução",
          restock: true,
          cancelled_at: "2026-10-31T15:00:00Z",
        },
      };
      return requests.length === 1
        ? route.fulfill({ status: 503, json: { error: { code: "sale_unavailable" } } })
        : route.fulfill({ json: { ...sale, replayed: true } });
    }
    return route.fulfill({
      json: new URL(r.url()).pathname.endsWith(saleID)
        ? sale
        : { items: [sale], next_cursor: "", timezone: "America/Sao_Paulo" },
    });
  });
  await page.goto(root + "/vendas");
  await page.getByRole("button", { name: "Ver venda", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.locator("summary", { hasText: "Anular venda" }).click();
  await dialog.getByLabel("Motivo da anulação").fill("Devolução");
  await dialog.getByRole("radio", { name: /Devolver ao estoque/ }).check();
  await dialog.getByRole("button", { name: "Confirmar anulação" }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "Ver venda", exact: true }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  expect(requests.length).toBe(1);
  await dialog.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toHaveCount(0);
  expect(requests).toHaveLength(2);
  expect(requests[1]).toEqual(requests[0]);
});

test("pending sale is isolated by shop while the same user switches units", async ({ page }) => {
  await setup(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  const otherBase = "http://127.0.0.1:8080/api/v1/admin/shops/" + otherShop.slug;
  await page.route(otherBase + "/products**", (route) =>
    route.fulfill({
      json: {
        items: [{ ...product, shop_id: otherShop.shop_id, name: "Produto da outra unidade" }],
        next_cursor: null,
      },
    }),
  );
  await page.route(otherBase + "/sales**", (route) =>
    route.fulfill({ json: { items: [], next_cursor: "", timezone: "America/Sao_Paulo" } }),
  );
  await page.route(otherBase + "/professionals", (route) => route.fulfill({ json: [] }));
  let posts = 0;
  await page.route(base + "/sales", (route) => {
    posts++;
    return route.fulfill({ status: 503, json: { error: { code: "sale_unavailable" } } });
  });
  await page.goto(root + "/vendas");
  let dialog = await selectAndReview(page);
  await dialog.getByRole("button", { name: "Confirmar venda", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await page.goto("/admin/" + otherShop.slug + "/vendas");
  await page.getByRole("button", { name: "Registrar venda", exact: true }).click();
  dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("button", { name: "Adicionar Produto da outra unidade", exact: true }),
  ).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Repetir confirmação" })).toHaveCount(0);
  await page.goto(root + "/vendas");
  await page.getByRole("button", { name: "Registrar venda", exact: true }).click();
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "Repetir confirmação" }),
  ).toBeVisible();
  expect(posts).toBe(1);
});
