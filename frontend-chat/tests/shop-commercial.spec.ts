import { test, expect, type Page } from "@playwright/test";
import { installSupabaseSession, mockAdminAccess, otherShop, testShop } from "./supabase-fixture";
import {
  hasCurrentCoverage,
  parseCommercialStatus,
  type CommercialStatus,
} from "../src/lib/api/shop-commercial";

const path = `/admin/${testShop.slug}/configuracoes`;
const base = `http://127.0.0.1:8080/api/v1/admin/shops/${testShop.slug}`;
const initial = {
  ...testShop,
  timezone: "America/Sao_Paulo",
  phone: "",
  description: "",
  version: 1,
  address: {
    postal_code: "01310100",
    street: "Avenida Paulista",
    number: "100",
    complement: "",
    district: "Bela Vista",
    city: "São Paulo",
    state: "SP",
  },
};
function paid(requested = false): CommercialStatus {
  return {
    shop_id: testShop.shop_id,
    slug: testShop.slug,
    publication_managed: true,
    publication_requested: requested,
    public_booking_enabled: requested,
    can_publish: true,
    blockers: [],
    readiness: { address: true, service: true, professional: true, schedule: true },
    subscription: {
      status: "active",
      plan_key: "standard_monthly",
      operation_allowed: true,
      reason: "subscribed",
      coverage_until: new Date(Date.now() + 86_400_000).toISOString(),
      cancel_at_period_end: false,
    },
  };
}
function unpaid(requested = false): CommercialStatus {
  const result = paid(requested);
  return {
    ...result,
    public_booking_enabled: false,
    can_publish: false,
    blockers: ["subscription_required"],
    subscription: {
      ...result.subscription,
      status: "none",
      plan_key: "",
      operation_allowed: false,
      reason: "subscription_required",
      coverage_until: null,
    },
  };
}
const card = (page: Page) => page.getByRole("region", { name: "Assinatura e agendamento online" });
const publish = (page: Page) =>
  card(page).getByRole("button", { name: "Publicar barbearia", exact: true });
const copy = (page: Page) =>
  card(page).getByRole("button", { name: "Copiar link de agendamento", exact: true });
async function setup(page: Page) {
  const session = await installSupabaseSession(page);
  await mockAdminAccess(page, [testShop, otherShop]);
  await page.route(base + "/settings", (route) => route.fulfill({ json: initial }));
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (text: string) => {
          (window as Window & { copied?: string }).copied = text;
        },
      },
    });
  });
  return session;
}

test("commercial parser rejects other tenants, malformed coverage and inconsistent flags", () => {
  const status = paid();
  expect(parseCommercialStatus(status, testShop.shop_id, testShop.slug)).toEqual(status);
  expect(hasCurrentCoverage(status, Date.parse(status.subscription.coverage_until!))).toBe(false);
  for (const invalid of [
    null,
    {},
    { ...status, shop_id: otherShop.shop_id },
    { ...status, slug: otherShop.slug },
    { ...status, publication_managed: false },
    { ...status, public_booking_enabled: true },
    { ...status, readiness: { ...status.readiness, address: false } },
    { ...status, blockers: ["unknown"] },
    { ...status, subscription: { ...status.subscription, coverage_until: null } },
    { ...status, subscription: { ...status.subscription, coverage_until: "not a date" } },
  ])
    expect(() => parseCommercialStatus(invalid, testShop.shop_id, testShop.slug)).toThrow();
});

test("checkout return is navigation only; unpaid owners keep configuration without a public link", async ({
  page,
}) => {
  await setup(page);
  const state = unpaid();
  state.blockers.push("service_required", "professional_required", "schedule_required");
  state.readiness.service = state.readiness.professional = state.readiness.schedule = false;
  let writes = 0;
  await page.route(base + "/commercial-status", (route) => {
    expect(route.request().method()).toBe("GET");
    return route.fulfill({ json: state });
  });
  await page.route(base + "/publication", (route) => {
    writes++;
    return route.abort();
  });
  await page.goto(path + "?checkout=returned");
  await expect(card(page)).toContainText("O retorno do Checkout não confirma pagamento");
  await expect(card(page)).toContainText("Sem assinatura confirmada");
  await expect(card(page)).toContainText("Cadastre pelo menos um serviço");
  await expect(publish(page)).toBeDisabled();
  await expect(copy(page)).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Abrir agendamento", exact: true })).toHaveCount(0);
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toBeEnabled();
  await page.getByLabel("Nome da barbearia", { exact: true }).fill("Rascunho mantido");
  await card(page).getByRole("button", { name: "Atualizar situação" }).click();
  await expect(card(page)).toContainText("Sem assinatura confirmada");
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toHaveValue(
    "Rascunho mantido",
  );
  expect(writes).toBe(0);
});

test("owner publishes with explicit confirmation, accepts 204 and shares only a confirmed selected-shop link", async ({
  page,
}) => {
  const session = await setup(page);
  let current = paid();
  const writes: unknown[] = [];
  await page.route(base + "/commercial-status", (route) => {
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    return route.fulfill({ json: current });
  });
  await page.route(base + "/publication", (route) => {
    expect(route.request().method()).toBe("PUT");
    expect(route.request().headers().authorization).toBe("Bearer " + session.accessToken);
    writes.push(route.request().postDataJSON());
    current = paid(true);
    return route.fulfill({ status: 204 });
  });
  await page.goto(path);
  await expect(publish(page)).toBeEnabled();
  await expect(copy(page)).toHaveCount(0);
  await publish(page).click();
  const dialog = page.getByRole("dialog", { name: "Publicar barbearia?", exact: true });
  await expect(dialog).toContainText("não inicia cobrança");
  expect(writes).toEqual([]);
  await dialog
    .getByRole("button", { name: "Confirmar publicação", exact: true })
    .evaluate((button) => {
      (button as HTMLButtonElement).click();
      (button as HTMLButtonElement).click();
    });
  await expect(card(page)).toContainText("Publicação confirmada na API.");
  expect(writes).toEqual([{ requested: true }]);
  await expect(card(page).getByRole("link", { name: "Abrir agendamento" })).toHaveAttribute(
    "href",
    "/b/" + testShop.slug,
  );
  await copy(page).click();
  expect(await page.evaluate(() => (window as Window & { copied?: string }).copied)).toBe(
    "http://127.0.0.1:3100/b/" + testShop.slug,
  );
  expect(await page.evaluate(() => JSON.stringify({ ...sessionStorage }))).not.toContain(
    "standard_monthly",
  );
});

test("dirty configuration blocks publication, while unpaid unpublication preserves the draft and does not cancel billing", async ({
  page,
}) => {
  await setup(page);
  let current = paid();
  const writes: unknown[] = [];
  await page.route(base + "/commercial-status", (route) => route.fulfill({ json: current }));
  await page.route(base + "/publication", (route) => {
    writes.push(route.request().postDataJSON());
    current = unpaid(false);
    return route.fulfill({ status: 204 });
  });
  await page.goto(path);
  await expect(publish(page)).toBeEnabled();
  await page.getByLabel("Nome da barbearia", { exact: true }).fill("Nome em edição");
  await expect(publish(page)).toBeDisabled();
  current = unpaid(true);
  await card(page).getByRole("button", { name: "Atualizar situação" }).click();
  await expect(card(page)).toContainText("A intenção de publicação foi mantida");
  await card(page).getByRole("button", { name: "Retirar do ar", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Retirar barbearia do ar?" });
  await expect(dialog).toContainText("A assinatura não será cancelada");
  await dialog.getByRole("button", { name: "Confirmar retirada do ar" }).click();
  await expect(card(page)).toContainText("Barbearia retirada do ar.");
  await expect(copy(page)).toHaveCount(0);
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toHaveValue("Nome em edição");
  expect(writes).toEqual([{ requested: false }]);
});

test("uncertain publication is recovered by GET without repeating PUT or losing edits", async ({
  page,
}) => {
  await setup(page);
  let current = paid();
  let writes = 0;
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(base + "/commercial-status", (route) => route.fulfill({ json: current }));
  await page.route(base + "/publication", async (route) => {
    writes++;
    current = paid(true);
    await gate;
    await route.abort();
  });
  await page.goto(path);
  await expect(publish(page)).toBeEnabled();
  await publish(page).click();
  await page.getByRole("button", { name: "Confirmar publicação" }).click();
  await expect(card(page)).toContainText("Atualizando publicação");
  await page.getByLabel("Nome da barbearia", { exact: true }).fill("Edição durante envio");
  release();
  await expect(card(page).getByRole("button", { name: "Conferir situação na API" })).toBeVisible();
  await expect(copy(page)).toHaveCount(0);
  await card(page).getByRole("button", { name: "Conferir situação na API" }).click();
  await expect(card(page)).toContainText("Nenhuma ação foi reenviada.");
  await expect(copy(page)).toBeVisible();
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toHaveValue(
    "Edição durante envio",
  );
  expect(writes).toBe(1);
});

for (const mode of [
  "read-error",
  "wrong-tenant",
  "wrong-slug",
  "expired",
  "inconsistent",
] as const) {
  test(`commercial ${mode} cannot enable publication or sharing and leaves settings usable`, async ({
    page,
  }) => {
    await setup(page);
    const current = paid(true);
    if (mode === "wrong-tenant") current.shop_id = otherShop.shop_id;
    if (mode === "wrong-slug") current.slug = otherShop.slug;
    if (mode === "expired")
      current.subscription.coverage_until = new Date(Date.now() - 1000).toISOString();
    if (mode === "inconsistent") current.publication_managed = false;
    await page.route(base + "/commercial-status", (route) =>
      mode === "read-error"
        ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
        : route.fulfill({ json: current }),
    );
    await page.goto(path);
    await expect(card(page).getByRole("alert")).toBeVisible();
    await expect(copy(page)).toHaveCount(0);
    await expect(publish(page)).toHaveCount(0);
    await expect(page.getByLabel("Nome da barbearia", { exact: true })).toBeEnabled();
  });
}

test("barber does not read commercial status or obtain publishing controls", async ({ page }) => {
  await setup(page);
  let calls = 0;
  await page.route(base + "/access", (route) =>
    route.fulfill({
      json: { role: "barber", professional_id: "66666666-6666-4666-8666-666666666666" },
    }),
  );
  await page.route(base + "/commercial-status", (route) => {
    calls++;
    return route.abort();
  });
  await page.goto(path);
  await expect(
    page.getByText("Somente o proprietário pode consultar e editar", { exact: false }),
  ).toBeVisible();
  await expect(card(page)).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Gerenciar link de agendamento" })).toHaveCount(0);
  expect(calls).toBe(0);
});

test("a confirmed 204 followed by a failed status read never exposes a stale link or retries publication", async ({
  page,
}) => {
  await setup(page);
  let current = paid();
  let writes = 0;
  let reads = 0;
  await page.route(base + "/commercial-status", (route) =>
    ++reads === 2
      ? route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } })
      : route.fulfill({ json: current }),
  );
  await page.route(base + "/publication", (route) => {
    writes++;
    current = paid(true);
    return route.fulfill({ status: 204 });
  });
  await page.goto(path);
  await expect(publish(page)).toBeEnabled();
  await publish(page).click();
  await page.getByRole("button", { name: "Confirmar publicação" }).click();
  await expect(card(page).getByRole("button", { name: "Conferir situação na API" })).toBeVisible();
  await expect(copy(page)).toHaveCount(0);
  await card(page).getByRole("button", { name: "Conferir situação na API" }).click();
  await expect(copy(page)).toBeVisible();
  expect(writes).toBe(1);
});

test("server rejects stale readiness and GET explains blockers without another PUT", async ({
  page,
}) => {
  await setup(page);
  let current = paid();
  let writes = 0;
  await page.route(base + "/commercial-status", (route) => route.fulfill({ json: current }));
  await page.route(base + "/publication", (route) => {
    writes++;
    current = unpaid();
    return route.fulfill({ status: 409, json: { error: { code: "publication_blocked" } } });
  });
  await page.goto(path);
  await expect(publish(page)).toBeEnabled();
  await publish(page).click();
  await page.getByRole("button", { name: "Confirmar publicação" }).click();
  await expect(card(page).getByRole("alert")).toContainText("ainda não pode ser publicada");
  await card(page).getByRole("button", { name: "Conferir situação na API" }).click();
  await expect(publish(page)).toBeDisabled();
  await expect(card(page)).toContainText("Sem assinatura confirmada");
  expect(writes).toBe(1);
});

test("revoked commercial access hides actions and clipboard failure offers a manual path", async ({
  page,
}) => {
  await setup(page);
  let revoked = false;
  await page.route(base + "/commercial-status", (route) =>
    revoked
      ? route.fulfill({ status: 403, json: { error: { code: "access_denied" } } })
      : route.fulfill({ json: paid(true) }),
  );
  await page.goto(path);
  await expect(copy(page)).toBeVisible();
  await page.evaluate(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("denied");
        },
      },
    });
  });
  await copy(page).click();
  await expect(card(page)).toContainText("Não foi possível copiar.");
  revoked = true;
  await card(page).getByRole("button", { name: "Atualizar situação" }).click();
  await expect(card(page).getByRole("link", { name: "Validar acesso à barbearia" })).toBeVisible();
  await expect(copy(page)).toHaveCount(0);
  await expect(card(page).getByRole("button", { name: "Retirar do ar", exact: true })).toHaveCount(
    0,
  );
});

test("saving configuration refreshes readiness without publishing automatically", async ({
  page,
}) => {
  await setup(page);
  let current = paid();
  current.can_publish = false;
  current.readiness.address = false;
  current.blockers = ["address_required"];
  let reads = 0;
  let publications = 0;
  await page.route(base + "/commercial-status", (route) => {
    reads++;
    return route.fulfill({ json: current });
  });
  await page.route(base + "/settings", (route) => {
    if (route.request().method() === "PUT") {
      current = paid();
      return route.fulfill({ json: { ...initial, ...route.request().postDataJSON(), version: 2 } });
    }
    return route.fulfill({ json: initial });
  });
  await page.route(base + "/publication", (route) => {
    publications++;
    return route.abort();
  });
  await page.goto(path);
  await expect(card(page)).toContainText("Complete e salve o endereço");
  await page.getByLabel("Nome da barbearia", { exact: true }).fill("Nome atualizado");
  await page.getByRole("button", { name: "Salvar alterações", exact: true }).click();
  await expect(publish(page)).toBeEnabled();
  await expect(page.getByLabel("Nome da barbearia", { exact: true })).toHaveValue(
    "Nome atualizado",
  );
  expect(reads).toBeGreaterThan(1);
  expect(publications).toBe(0);
  await expect(copy(page)).toHaveCount(0);
});

test("legacy shops keep their public link without being described as Stripe subscribers", async ({
  page,
}) => {
  await setup(page);
  await page.goto(path);
  await expect(card(page)).toContainText("Acesso anterior mantido");
  await expect(card(page)).toContainText("Nenhuma assinatura foi iniciada ou cobrada");
  await expect(copy(page)).toBeVisible();
  await expect(publish(page)).toHaveCount(0);
  await expect(card(page).getByRole("button", { name: "Retirar do ar", exact: true })).toHaveCount(
    0,
  );
});

test("commercial block fits mobile and desktop and respects the shop timezone", async ({
  page,
}) => {
  await setup(page);
  const current = paid(true);
  current.subscription.cancel_at_period_end = true;
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.route(base + "/commercial-status", (route) => route.fulfill({ json: current }));
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(path);
    await expect(copy(page)).toBeVisible();
    await expect(card(page)).toContainText("America/Sao_Paulo");
    await expect(card(page)).toContainText("cancelamento programado");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await card(page).screenshot({ path: `test-results/commercial-${width}.png` });
  }
  await page.setViewportSize({ width: 390, height: 1000 });
  await page.evaluate(() => {
    document.documentElement.style.fontSize = "200%";
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
