import { expect, test, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, testShop, otherShop } from "./supabase-fixture";
import type { ReportCounts, ReportValues, ReportHistoryItem } from "../src/lib/api/reports";

test.use({ timezoneId: "Asia/Tokyo" });
const root = "/admin/" + testShop.slug + "/relatorios";
const endpoint = "http://127.0.0.1:8080/api/v1/admin/shops/" + testShop.slug + "/reports/activity";
const id = (n: number) => "55555555-5555-4555-8555-" + n.toString(16).padStart(12, "0");
const professional = id(1),
  service = id(2),
  product = id(3),
  shampoo = id(4);
const zero: ReportCounts = {
  scheduled: 0,
  confirmed: 0,
  completed: 0,
  completed_automatically: 0,
  cancelled: 0,
  no_show: 0,
};
const emptyValue: ReportValues = {
  currency: "BRL",
  scheduled_service_value_cents: 0,
  completed_service_value_cents: 0,
  product_sale_value_cents: 0,
  combined_value_cents: 0,
  completed_appointment_count: 0,
  product_sale_count: 0,
  product_unit_count: 0,
};
function period(url: URL) {
  const from = url.searchParams.get("from")!,
    through = url.searchParams.get("through")!,
    until = new Date(through + "T03:00:00Z");
  until.setUTCDate(until.getUTCDate() + 1);
  return {
    from_date: from,
    through_date: through,
    from_at: from + "T03:00:00Z",
    until_at: until.toISOString(),
    timezone: "America/Sao_Paulo",
  };
}
function overview(url: URL, empty = false) {
  const p = period(url),
    source = url.searchParams.get("source") || "all";
  const services = source !== "products",
    products = source !== "services" && url.searchParams.get("product_id") !== shampoo;
  const counts = services
    ? { ...zero, scheduled: 1, completed: 2, completed_automatically: 1, cancelled: 21 }
    : { ...zero };
  const s: ReportValues = {
    ...emptyValue,
    scheduled_service_value_cents: 5000,
    completed_service_value_cents: 10000,
    combined_value_cents: 10000,
    completed_appointment_count: 2,
  };
  const v: ReportValues = {
    ...emptyValue,
    product_sale_value_cents: products ? 9000 : 0,
    combined_value_cents: products ? 9000 : 0,
    product_sale_count: products ? 1 : 0,
    product_unit_count: products ? 3 : 0,
  };
  const total: ReportValues = {
    ...v,
    scheduled_service_value_cents: services ? 5000 : 0,
    completed_service_value_cents: services ? 10000 : 0,
    combined_value_cents: (services ? 10000 : 0) + v.product_sale_value_cents,
    completed_appointment_count: services ? 2 : 0,
  };
  return {
    period: p,
    source,
    observed_at: "2026-10-31T15:00:00Z",
    counts: empty ? zero : counts,
    values: empty ? [] : [total],
    by_day: empty ? [] : [{ local_date: p.from_date, counts, values: [total] }],
    by_professional: empty ? [] : [{ id: professional, label: "Palma", counts, values: [total] }],
    by_service:
      empty || !services ? [] : [{ id: service, label: "Corte histórico", counts, values: [s] }],
    by_product:
      empty || source === "services"
        ? []
        : [
            { id: product, label: "Pomada", counts: zero, values: [v] },
            { id: shampoo, label: "Shampoo desativado", counts: zero, values: [emptyValue] },
          ].filter(
            (g) =>
              !url.searchParams.get("product_id") || g.id === url.searchParams.get("product_id"),
          ),
  };
}
function history(url: URL, empty = false) {
  const p = period(url),
    source = url.searchParams.get("source") || "all";
  let items: Omit<ReportHistoryItem, "id">[] = Array.from({ length: 27 }, (_, i) => {
    const at = new Date(p.from_date + "T15:00:00Z");
    at.setUTCMinutes(at.getUTCMinutes() - i);
    const productRow = i === 2 || i === 24 || i === 25,
      saleStatus = i === 2 ? "recorded" : "cancelled",
      completed = i < 2;
    return {
      source: productRow ? "products" : "services",
      record_id: i >= 24 && i <= 25 ? id(200) : id(100 + i),
      item_id: productRow ? id(300 + i) : null,
      service_id: productRow ? null : service,
      product_id: productRow ? (i === 25 ? shampoo : product) : null,
      professional_id: professional,
      label: productRow ? (i === 25 ? "Shampoo anulado" : "Pomada") : "Corte histórico",
      professional_name: "Palma",
      currency: "BRL",
      appointment_status: productRow
        ? null
        : completed
          ? "completed"
          : i === 26
            ? "scheduled"
            : "cancelled",
      appointment_status_source: productRow ? null : i === 0 ? "automatic" : "staff",
      sale_status: productRow ? saleStatus : null,
      customer_name: "Ana Silva",
      customer_phone: "+5511912345678",
      occurred_at: at.toISOString(),
      quantity: i === 2 ? 3 : 1,
      unit_price_cents: productRow ? 3000 : 5000,
      value_cents: i === 2 ? 9000 : productRow ? 3000 : 5000,
      recorded_value_cents: i === 2 ? 9000 : completed ? 5000 : 0,
    };
  });
  items = items.filter(
    (r) =>
      (source === "all" || source === r.source) &&
      (!url.searchParams.get("service_id") ||
        r.source !== "services" ||
        r.service_id === url.searchParams.get("service_id")) &&
      (!url.searchParams.get("product_id") ||
        r.source !== "products" ||
        r.product_id === url.searchParams.get("product_id")) &&
      (!url.searchParams.get("appointment_status") ||
        r.source !== "services" ||
        r.appointment_status === url.searchParams.get("appointment_status")) &&
      (!url.searchParams.get("sale_status") ||
        r.source !== "products" ||
        r.sale_status === url.searchParams.get("sale_status")),
  );
  const offset = url.searchParams.has("cursor") ? 25 : 0;
  return {
    period: p,
    source,
    observed_at: "2026-10-31T15:00:00Z",
    items: empty ? [] : items.slice(offset, offset + 25),
    next_cursor: !empty && !offset && items.length > 25 ? "next-page" : "",
  };
}
async function setup(
  page: Page,
  options: { barber?: boolean; empty?: boolean; shops?: boolean; sessionUntil?: string } = {},
) {
  await page.clock.install({ time: new Date("2026-10-31T15:00:00Z") });
  const session = await installSupabaseSession(page);
  if (options.sessionUntil) {
    const claims = JSON.parse(
      Buffer.from(session.accessToken.split(".")[1], "base64url").toString(),
    );
    claims.exp = Math.floor(Date.parse(options.sessionUntil) / 1000);
    const parts = session.accessToken.split(".");
    parts[1] = Buffer.from(JSON.stringify(claims)).toString("base64url");
    session.accessToken = parts.join(".");
    session.response.access_token = session.accessToken;
    session.response.expires_at = claims.exp;
    await page.addInitScript(
      ({ value }) => localStorage.setItem("sb-test-project-auth-token", JSON.stringify(value)),
      { value: session.response },
    );
  }
  await mockAdminAccess(page, options.shops ? [testShop, otherShop] : [testShop]);
  if (options.barber)
    await page.route("**/api/v1/admin/shops/*/access", (route) =>
      route.fulfill({ json: { role: "barber", professional_id: professional } }),
    );
  const requests: URL[] = [];
  await page.route("**/api/v1/admin/shops/*/reports/activity**", async (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    const url = new URL(route.request().url());
    requests.push(url);
    await route.fulfill({
      json: url.pathname.endsWith("/history")
        ? history(url, options.empty)
        : overview(url, options.empty),
    });
  });
  return requests;
}
test("server totals stay independent of pagination and sale items share no key", async ({
  page,
}) => {
  const requests = await setup(page);
  await page.goto(root);
  const values = page.locator('[aria-label="Valores registrados no período"]');
  await expect(values).toContainText("R$ 190,00");
  await expect(values).toContainText("R$ 100,00");
  await expect(values).toContainText("R$ 90,00");
  await expect(page.getByText("25 registros carregados.", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Carregar mais atividades" }).click();
  await expect(page.getByText("27 registros carregados.", { exact: false })).toBeVisible();
  await expect(page.getByText("Shampoo anulado", { exact: false })).toBeVisible();
  await expect(values).toContainText("R$ 190,00");
  expect(
    requests
      .filter((u) => u.pathname.endsWith("/history"))
      .at(-1)
      ?.searchParams.get("cursor"),
  ).toBe("next-page");
  expect(requests.find((u) => !u.pathname.endsWith("/history"))?.searchParams.get("from")).toBe(
    "2026-10-01",
  );
  await page
    .locator("details")
    .filter({ has: page.getByText("Ver valores do gráfico em tabela", { exact: true }) })
    .locator("summary")
    .click();
  await expect(page.getByRole("table")).toBeVisible();
  await page.screenshot({ path: "test-results/reports-desktop.png", fullPage: true });
});

test("common filters reset pagination and exclusive source clears incompatible resource", async ({
  page,
}) => {
  const requests = await setup(page);
  await page.goto(root);
  await expect(page.getByRole("button", { name: "Carregar mais atividades" })).toBeVisible();
  await page.getByRole("button", { name: "Carregar mais atividades" }).click();
  await expect(page.getByText("27 registros carregados.", { exact: false })).toBeVisible();
  await page.getByText("Mais filtros", { exact: true }).click();
  await page.getByLabel("Telefone do cliente").fill("(11) 91234-5678");
  await page.getByLabel("Serviço", { exact: true }).selectOption(service);
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect
    .poll(() =>
      requests
        .filter((u) => u.pathname.endsWith("/history"))
        .at(-1)
        ?.searchParams.get("phone"),
    )
    .toBe("(11) 91234-5678");
  const last = requests.filter((u) => u.pathname.endsWith("/history")).at(-1)!;
  expect(last.searchParams.has("cursor")).toBe(false);
  expect(last.searchParams.get("phone_country")).toBe("BR");
  await page.getByLabel("Origem", { exact: true }).selectOption("products");
  await page.getByLabel("Produto", { exact: true }).selectOption(product);
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect
    .poll(() =>
      requests
        .filter((u) => u.pathname.endsWith("/history"))
        .at(-1)
        ?.searchParams.get("source"),
    )
    .toBe("products");
  const products = requests.filter((u) => u.pathname.endsWith("/history")).at(-1)!;
  expect(products.searchParams.has("service_id")).toBe(false);
  expect(products.searchParams.get("product_id")).toBe(product);
  await expect(page.getByLabel("Situação dos atendimentos")).toHaveCount(0);
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 90,00",
  );
});

test("history outcome controls affect history only and cancelled originals remain visible", async ({
  page,
}) => {
  const requests = await setup(page);
  await page.goto(root);
  await expect(page.getByText("25 registros carregados.", { exact: false })).toBeVisible();
  const before = requests.filter((u) => !u.pathname.endsWith("/history")).length;
  await page.getByLabel("Situação dos atendimentos").selectOption("completed");
  await expect(page.getByText("5 registros carregados.", { exact: false })).toBeVisible();
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 190,00",
  );
  expect(requests.filter((u) => !u.pathname.endsWith("/history")).length).toBe(before);
  await expect(page.getByText("Venda anulada", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("R$ 0,00 no total", { exact: true }).first()).toBeVisible();
  await page.getByLabel("Situação das vendas").selectOption("recorded");
  await expect(page.getByText("3 registros carregados.", { exact: false })).toBeVisible();
});

test("barber has a personal report and no global professional selector", async ({ page }) => {
  await setup(page, { barber: true });
  await page.goto(root);
  await expect(page.getByRole("heading", { name: "Relatórios", exact: true })).toBeVisible();
  await expect(
    page.getByText("Acompanhe os atendimentos e as vendas atribuídos a você."),
  ).toBeVisible();
  await expect(page.getByLabel("Barbeiro", { exact: true })).toHaveCount(0);
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 190,00",
  );
  await expect(
    page
      .getByRole("navigation", { name: "Navegação principal" })
      .getByRole("link", { name: "Relatórios", exact: true }),
  ).toHaveAttribute("aria-current", "page");
});

test("empty is distinct from an API failure and failed history can restart", async ({ page }) => {
  await setup(page, { empty: true });
  await page.goto(root);
  await expect(page.getByRole("heading", { name: "Nenhuma atividade encontrada" })).toBeVisible();
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 0,00",
  );
  await page.route(endpoint + "/history**", (route) =>
    route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } }),
  );
  await page.getByRole("button", { name: "Atualizar relatório" }).click();
  await expect(page.getByRole("button", { name: "Recarregar histórico" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Nenhuma atividade encontrada" })).toHaveCount(0);
  await page.route(endpoint + "/history**", (route) =>
    route.fulfill({ json: history(new URL(route.request().url()), true) }),
  );
  await page.getByRole("button", { name: "Recarregar histórico" }).click();
  await expect(page.getByRole("heading", { name: "Nenhuma atividade encontrada" })).toBeVisible();
  await page.route(endpoint + "?**", (route) =>
    route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } }),
  );
  await page.getByRole("button", { name: "Atualizar relatório" }).click();
  await expect(
    page.getByText("Não foi possível carregar o resumo.", { exact: false }),
  ).toBeVisible();
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toHaveCount(0);
});

test("invalid amounts and cross-professional history fail closed", async ({ page }) => {
  await setup(page, { barber: true });
  await page.route(endpoint + "/history**", (route) => {
    const body = history(new URL(route.request().url()));
    body.items[0].professional_id = id(900);
    return route.fulfill({ json: body });
  });
  await page.goto(root);
  await expect(page.getByRole("button", { name: "Recarregar histórico" })).toBeVisible();
  await expect(page.getByText("Ana Silva", { exact: true })).toHaveCount(0);
  await page.route(endpoint + "?**", (route) => {
    const body = overview(new URL(route.request().url()));
    body.values[0].combined_value_cents = 1;
    return route.fulfill({ json: body });
  });
  await page.getByRole("button", { name: "Atualizar relatório" }).click();
  await expect(
    page.getByText("Os dados do relatório estão inconsistentes.", { exact: false }).first(),
  ).toBeVisible();
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toHaveCount(0);
});

test("switching shops discards contact filters and prior history", async ({ page }) => {
  const requests = await setup(page, { shops: true });
  await page.goto(root);
  await expect(page.getByText("25 registros carregados.", { exact: false })).toBeVisible();
  await page.getByText("Mais filtros", { exact: true }).click();
  await page.getByLabel("Nome do cliente").fill("Ana");
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect
    .poll(() =>
      requests
        .filter((u) => u.pathname.endsWith("/history"))
        .at(-1)
        ?.searchParams.get("customer_name"),
    )
    .toBe("Ana");
  await page.goto("/admin/" + otherShop.slug + "/relatorios");
  await expect(page.getByRole("heading", { name: "Relatórios", exact: true })).toBeVisible();
  await expect(page.getByText("25 registros carregados.", { exact: false })).toBeVisible();
  const other = requests.filter((u) => u.pathname.includes(otherShop.slug)).at(-1)!;
  expect(other.searchParams.has("customer_name")).toBe(false);
  expect(other.searchParams.has("cursor")).toBe(false);
});

test("mobile and dark reports preserve navigation and readable graph without page overflow", async ({
  page,
}) => {
  await setup(page);
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto(root);
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 190,00",
  );
  await expect(page.getByRole("navigation", { name: "Navegação rápida" })).toBeVisible();
  await page.getByRole("button", { name: "Mais", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Menu do painel" });
  await expect(dialog.getByRole("link", { name: "Relatórios", exact: true })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await page.keyboard.press("Escape");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await expect(
    page.getByRole("img", { name: "Valores de serviços e produtos por mês, em BRL" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("img", { name: "Valores de serviços e produtos por dia, em BRL" }),
  ).toBeVisible();
  await page.screenshot({ path: "test-results/reports-mobile-dark.png", fullPage: true });
  await page.screenshot({ path: "test-results/reports-mobile-viewport.png" });
  await page.getByRole("heading", { name: "Evolução dos valores" }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: "test-results/reports-mobile-chart.png" });
});

test("shop month follows server timezone and long periods group the graph", async ({ page }) => {
  const requests = await setup(page, { sessionUntil: "2026-11-02T12:00:00Z" });
  await page.clock.setFixedTime(new Date("2026-11-01T01:30:00Z"));
  await page.goto(root);
  await expect(page.getByLabel("De", { exact: true })).toHaveValue("2026-10-01");
  await expect(page.getByLabel("Até", { exact: true })).toHaveValue("2026-10-31");
  expect(requests.some((u) => u.searchParams.get("from") === "2026-11-01")).toBe(true);
  await page.getByLabel("De", { exact: true }).fill("2026-07-01");
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect(
    page.getByRole("img", { name: "Valores de serviços e produtos por mês, em BRL" }),
  ).toBeVisible();
  const before = requests.length;
  await page.getByLabel("De", { exact: true }).fill("2025-01-01");
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Consulte até 366 dias por vez." }),
  ).toBeVisible();
  expect(requests.length).toBe(before);
});

test("a delayed summary cannot replace newer filter results", async ({ page }) => {
  await setup(page);
  await page.goto(root);
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 190,00",
  );
  let release!: () => void,
    done!: () => void,
    blocked = false;
  const gate = new Promise<void>((resolve) => (release = resolve)),
    completed = new Promise<void>((resolve) => (done = resolve));
  await page.route(endpoint + "?**", async (route) => {
    const u = new URL(route.request().url());
    if (u.searchParams.get("customer_name") === "Ana") {
      blocked = true;
      await gate;
      await route.fulfill({ json: overview(u) }).catch(() => {});
      done();
    } else await route.fulfill({ json: overview(u, true) });
  });
  await page.route(endpoint + "/history**", (route) =>
    route.fulfill({ json: history(new URL(route.request().url()), true) }),
  );
  await page.getByText("Mais filtros", { exact: true }).click();
  await page.getByLabel("Nome do cliente").fill("Ana");
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect.poll(() => blocked).toBe(true);
  await page.getByLabel("Nome do cliente").fill("Bruno");
  await page.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toContainText(
    "R$ 0,00",
  );
  release();
  await completed;
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  await expect(page.locator('[aria-label="Valores registrados no período"]')).not.toContainText(
    "R$ 190,00",
  );
});

test("microsecond ordering is preserved and a revoked read hides prior private data", async ({
  page,
}) => {
  await setup(page);
  await page.route(endpoint + "/history**", (route) => {
    const u = new URL(route.request().url()),
      body = history(u);
    body.items = body.items.slice(0, 2);
    body.next_cursor = "";
    body.items[0].record_id = id(1);
    body.items[1].record_id = id(9000);
    body.items[0].occurred_at = u.searchParams.get("from") + "T15:00:00.000002Z";
    body.items[1].occurred_at = u.searchParams.get("from") + "T15:00:00.000001Z";
    return route.fulfill({ json: body });
  });
  await page.goto(root);
  await expect(page.getByText("2 registros carregados.", { exact: false })).toBeVisible();
  await page.route(endpoint + "/history**", (route) =>
    route.fulfill({ status: 403, json: { error: { code: "access_denied" } } }),
  );
  await page.getByRole("button", { name: "Atualizar relatório" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Seu acesso mudou." })).toBeVisible();
  await expect(page.locator('[aria-label="Valores registrados no período"]')).toHaveCount(0);
  await expect(page.getByText("Ana Silva", { exact: true })).toHaveCount(0);
});

test("a completed free service is not presented as a forecast", async ({ page }) => {
  await setup(page);
  await page.route(endpoint + "?**", (route) => {
    const body = overview(new URL(route.request().url()));
    const counts = { ...zero, completed: 1, completed_automatically: 1 },
      values = [{ ...emptyValue, completed_appointment_count: 1 }];
    body.counts = counts;
    body.values = values;
    body.by_day = [{ local_date: body.period.from_date, counts, values }];
    body.by_professional = [{ id: professional, label: "Palma", counts, values }];
    body.by_service = [{ id: service, label: "Corte gratuito", counts, values }];
    body.by_product = [];
    return route.fulfill({ json: body });
  });
  await page.route(endpoint + "/history**", (route) => {
    const body = history(new URL(route.request().url()));
    body.items = body.items.slice(0, 1);
    body.next_cursor = "";
    Object.assign(body.items[0], {
      label: "Corte gratuito",
      unit_price_cents: 0,
      value_cents: 0,
      recorded_value_cents: 0,
    });
    return route.fulfill({ json: body });
  });
  await page.goto(root);
  const historyRegion = page.getByRole("region", { name: "Histórico de atividades" });
  await expect(historyRegion.getByText("Corte gratuito", { exact: true })).toBeVisible();
  await expect(historyRegion.getByText("Concluído automaticamente", { exact: true })).toBeVisible();
  await expect(historyRegion.getByText("Previsão", { exact: true })).toHaveCount(0);
});
