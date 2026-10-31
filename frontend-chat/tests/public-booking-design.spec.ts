import { expect, test, type Page } from "@playwright/test";

test.use({ timezoneId: "Pacific/Honolulu" });
const base = "http://127.0.0.1:8080/api/v1/public/shops/teste";
const code = "ABCDEFGH-IJKLMNOP-QRSTUVWX-YZ234567";
const service = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Corte e barba",
  description: "Corte, acabamento e toalha quente.",
  duration_minutes: 80,
  price_cents: 9000,
  currency: "BRL",
  active: true,
};
const professional = { id: "22222222-2222-4222-8222-222222222222", name: "João Carlos" };
const appointment = {
  id: "33333333-3333-4333-8333-333333333333",
  shop_name: "Barbearia do Centro",
  status: "scheduled",
  service_name: service.name,
  duration_minutes: 80,
  price_cents: 9000,
  currency: "BRL",
  professional_id: professional.id,
  professional_name: professional.name,
  starts_at: "2026-10-05T12:30:00Z",
  ends_at: "2026-10-05T14:30:00Z",
  shop_timezone: "America/Sao_Paulo",
};

async function fixture(page: Page) {
  const attempts: { key: string | undefined; body: unknown }[] = [];
  const state = {
    cancelled: false,
    cancelRequests: 0,
    lookupFails: false,
    loseCancellationResponse: false,
    malformedCreation: false,
  };
  await page.route(base + "/**", async (route) => {
    const request = route.request();
    expect(request.headers().authorization).toBeUndefined();
    expect(request.url()).not.toContain(code);
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/services")) return route.fulfill({ json: [service] });
    if (path.endsWith("/professionals"))
      return route.fulfill({
        json: {
          shop_name: appointment.shop_name,
          shop_timezone: appointment.shop_timezone,
          today: "2026-10-05",
          last_booking_date: "2027-01-03",
          professionals: [professional],
        },
      });
    if (path.endsWith("/availability"))
      return route.fulfill({
        json: {
          shop_timezone: appointment.shop_timezone,
          slots: [{ starts_at: appointment.starts_at, ends_at: appointment.ends_at }],
        },
      });
    if (path.endsWith("/appointments")) {
      attempts.push({ key: request.headers()["idempotency-key"], body: request.postDataJSON() });
      return route.fulfill({
        status: 201,
        json: {
          appointment,
          code: state.malformedCreation && attempts.length === 1 ? "invalid-code" : code,
          replayed: attempts.length > 1,
        },
      });
    }
    if (path.endsWith("/lookup") || path.endsWith("/cancel")) {
      if (state.lookupFails && path.endsWith("/lookup"))
        return route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } });
      if (path.endsWith("/cancel")) {
        state.cancelRequests++;
        state.cancelled = true;
        if (state.loseCancellationResponse) return route.abort("failed");
      }
      return route.fulfill({
        json: {
          appointment: { ...appointment, status: state.cancelled ? "cancelled" : "scheduled" },
          can_cancel: !state.cancelled,
        },
      });
    }
    throw new Error("Unexpected public route " + path);
  });
  return { state, attempts };
}
async function schedule(page: Page) {
  await page.goto("/b/teste");
  await page.getByRole("button", { name: /Corte e barba/ }).click();
  await page
    .getByRole("combobox", { name: "Profissional", exact: true })
    .selectOption(professional.id);
  await page.getByRole("button", { name: "09:30", exact: true }).click();
}
async function review(page: Page) {
  await schedule(page);
  await page.getByLabel("Nome", { exact: true }).fill("Ana");
  await page.getByLabel("Sobrenome", { exact: true }).fill("Silva");
  await page.getByLabel("Telefone com DDD").fill("(11) 91234-5678");
  await page.getByRole("button", { name: "Revisar agendamento" }).click();
}
async function confirm(page: Page) {
  await review(page);
  await page.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Cancelar agendamento", exact: true }),
  ).toBeVisible();
}

test("public catalog adapts to mobile, tablet and desktop without borrowing the original brand", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/b/teste");
  await expect(page.getByRole("button", { name: /Corte e barba/ })).toBeVisible();
  await expect(page.getByText("Agendamento online", { exact: true })).toBeVisible();
  await expect(page.getByRole("img", { name: /Palma/i })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /Painel|Login|agenda/i })).toHaveCount(0);
  await expect(
    page.getByLabel("Etapas do agendamento").locator('[aria-current="step"]'),
  ).toContainText("Serviço");
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 960 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const button = await page.getByRole("button", { name: /Corte e barba/ }).boundingBox();
    expect(button!.height).toBeGreaterThanOrEqual(44);
    await page.screenshot({ path: `test-results/public-catalog-${width}.png`, fullPage: true });
  }
});

test("progress, focus, shop identity and customer draft follow the real booking steps", async ({
  page,
}) => {
  const { attempts } = await fixture(page);
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/b/teste");
  await page.getByRole("button", { name: /Corte e barba/ }).click();
  await expect(page.getByRole("heading", { name: "Escolha profissional e horário" })).toBeFocused();
  await expect(
    page.getByLabel("Etapas do agendamento").locator('[aria-current="step"]'),
  ).toContainText("Horário");
  await expect(page.getByRole("banner")).toContainText(appointment.shop_name);
  await expect(page.getByLabel("Data do agendamento")).toHaveValue("2026-10-05");
  await expect(page.getByLabel("Data do agendamento")).toHaveAttribute("max", "2027-01-03");
  await page
    .getByRole("combobox", { name: "Profissional", exact: true })
    .selectOption(professional.id);
  await page.screenshot({ path: "test-results/public-schedule-320.png", fullPage: true });
  await page.getByRole("button", { name: "09:30", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Seus dados para o agendamento" })).toBeFocused();
  await expect(
    page.getByLabel("Etapas do agendamento").locator('[aria-current="step"]'),
  ).toContainText("Seus dados");
  await page.getByLabel("Nome", { exact: true }).fill("Ana");
  await page.getByLabel("Sobrenome", { exact: true }).fill("Silva");
  await page.getByLabel("Telefone com DDD").fill("(11) 91234-5678");
  await page.screenshot({ path: "test-results/public-contact-320.png", fullPage: true });
  await page.getByRole("button", { name: "Revisar agendamento" }).click();
  await expect(page.getByRole("heading", { name: "Confira antes de confirmar" })).toBeFocused();
  await expect(
    page.getByLabel("Etapas do agendamento").locator('[aria-current="step"]'),
  ).toContainText("Confirmação");
  await expect(page.getByText("09:30–11:30", { exact: true })).toBeVisible();
  await page.screenshot({ path: "test-results/public-review-320.png", fullPage: true });
  expect(attempts).toHaveLength(0);
  await page.getByRole("button", { name: "Corrigir meus dados" }).click();
  await expect(page.getByLabel("Nome", { exact: true })).toHaveValue("Ana");
  await page.getByRole("button", { name: "Escolher outro horário" }).click();
  await page.getByRole("button", { name: "09:30", exact: true }).click();
  await expect(page.getByLabel("Telefone com DDD")).toHaveValue("(11) 91234-5678");
  await page.getByRole("button", { name: "Escolher outro horário" }).click();
  await page.getByRole("button", { name: "Escolher outro serviço" }).click();
  await expect(page.getByRole("button", { name: /Corte e barba/ })).toBeFocused();
});

test("malformed creation responses keep the exact retry key and block navigation", async ({
  page,
}) => {
  const { state, attempts } = await fixture(page);
  state.malformedCreation = true;
  await review(page);
  await page.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
  await expect(page.getByRole("button", { name: "Repetir confirmação" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Já tenho um código" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Agendar horário", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Corrigir meus dados" })).toBeDisabled();
  await page.getByRole("button", { name: "Repetir confirmação" }).click();
  await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
  expect(attempts).toHaveLength(2);
  expect(attempts[0]).toEqual(attempts[1]);
});

test("receipt exposes the full code and supports clipboard and keyboard cancellation", async ({
  page,
}) => {
  await fixture(page);
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          (window as Window & { copiedTestCode?: string }).copiedTestCode = value;
        },
      },
    });
  });
  await page.setViewportSize({ width: 320, height: 900 });
  await confirm(page);
  const receipt = page.getByRole("region", { name: "Comprovante do agendamento" });
  await expect(receipt.getByRole("heading", { name: "Agendamento registrado!" })).toBeFocused();
  const codeField = page.getByLabel("Código do agendamento");
  await expect(codeField).toHaveValue(code);
  expect(await codeField.evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(
    true,
  );
  await page.getByRole("button", { name: "Copiar código", exact: true }).click();
  expect(
    await page.evaluate(() => (window as Window & { copiedTestCode?: string }).copiedTestCode),
  ).toBe(code);
  await expect(page.getByRole("status").filter({ hasText: "Código copiado" })).toBeVisible();
  await page.screenshot({ path: "test-results/public-receipt-320.png", fullPage: true });
  await page.getByRole("button", { name: "Cancelar agendamento", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Cancelar este agendamento?" })).toBeFocused();
  await page.getByRole("button", { name: "Manter agendamento" }).click();
  await expect(
    page.getByRole("button", { name: "Cancelar agendamento", exact: true }),
  ).toBeFocused();
});

test("clipboard failure selects the code without claiming it was copied", async ({ page }) => {
  await fixture(page);
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("Clipboard denied");
        },
      },
    });
  });
  await confirm(page);
  await page.getByRole("button", { name: "Copiar código", exact: true }).click();
  const input = page.getByLabel("Código do agendamento");
  await expect(input).toBeFocused();
  expect(
    await input.evaluate((element) => {
      const textarea = element as HTMLTextAreaElement;
      return textarea.value.slice(textarea.selectionStart, textarea.selectionEnd);
    }),
  ).toBe(code);
  await expect(page.getByRole("status")).toContainText("Ctrl+C");
  await expect(page.getByText("Código copiado.", { exact: false })).toHaveCount(0);
});

test("failed status refresh keeps the saved receipt but blocks cancellation from stale data", async ({
  page,
}) => {
  const { state } = await fixture(page);
  await confirm(page);
  state.lookupFails = true;
  await page.getByRole("button", { name: "Atualizar situação" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "temporariamente indisponível",
  );
  await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
  await expect(page.getByRole("button", { name: "Cancelar agendamento", exact: true })).toHaveCount(
    0,
  );
  expect(state.cancelRequests).toBe(0);
  state.lookupFails = false;
  await page.getByRole("button", { name: "Atualizar situação" }).click();
  await expect(
    page.getByRole("button", { name: "Cancelar agendamento", exact: true }),
  ).toBeVisible();
});

test("lost cancellation response is resolved by a read, never an automatic second cancellation", async ({
  page,
}) => {
  const { state } = await fixture(page);
  state.loseCancellationResponse = true;
  await confirm(page);
  await page.getByRole("button", { name: "Cancelar agendamento", exact: true }).click();
  await page.getByRole("button", { name: "Sim, cancelar agendamento" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Não foi possível acessar a API",
  );
  await expect(page.getByRole("button", { name: "Sim, cancelar agendamento" })).toHaveCount(0);
  expect(state.cancelRequests).toBe(1);
  await page.getByRole("button", { name: "Atualizar situação" }).click();
  await expect(page.getByRole("status")).toContainText("O horário foi liberado");
  expect(state.cancelRequests).toBe(1);
});

test("a late availability response cannot replace the newly selected professional's times", async ({
  page,
}) => {
  await fixture(page);
  const other = { id: "44444444-4444-4444-8444-444444444444", name: "Outro profissional" };
  await page.route(base + "/professionals?**", (route) =>
    route.fulfill({
      json: {
        shop_name: appointment.shop_name,
        shop_timezone: appointment.shop_timezone,
        today: "2026-10-05",
        last_booking_date: "2027-01-03",
        professionals: [professional, other],
      },
    }),
  );
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let firstRequested = false;
  let firstFinished = false;
  await page.route(base + "/availability?**", async (route) => {
    if (new URL(route.request().url()).searchParams.get("professional_id") === professional.id) {
      firstRequested = true;
      await gate;
      await route
        .fulfill({
          json: {
            shop_timezone: appointment.shop_timezone,
            slots: [{ starts_at: appointment.starts_at, ends_at: appointment.ends_at }],
          },
        })
        .catch(() => {});
      firstFinished = true;
    } else
      await route.fulfill({
        json: {
          shop_timezone: appointment.shop_timezone,
          slots: [{ starts_at: "2026-10-05T14:30:00Z", ends_at: "2026-10-05T16:30:00Z" }],
        },
      });
  });
  try {
    await page.goto("/b/teste");
    await page.getByRole("button", { name: /Corte e barba/ }).click();
    const select = page.getByRole("combobox", { name: "Profissional", exact: true });
    await select.selectOption(professional.id);
    await expect.poll(() => firstRequested).toBe(true);
    await select.selectOption(other.id);
    await expect(page.getByRole("button", { name: "11:30", exact: true })).toBeVisible();
    release();
    await expect.poll(() => firstFinished).toBe(true);
    await expect(page.getByRole("button", { name: "09:30", exact: true })).toHaveCount(0);
  } finally {
    release();
  }
});

test("pending booking confirmation cannot be submitted twice or abandoned by switching modes", async ({
  page,
}) => {
  await fixture(page);
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let posts = 0;
  await page.route(base + "/appointments", async (route) => {
    posts++;
    await gate;
    await route.fulfill({ status: 201, json: { appointment, code, replayed: false } });
  });
  try {
    await review(page);
    await page.getByRole("button", { name: "Confirmar agendamento", exact: true }).click();
    await expect(page.getByRole("button", { name: "Confirmando…", exact: true })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Já tenho um código" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Corrigir meus dados" })).toBeDisabled();
    expect(posts).toBe(1);
    release();
    await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
    expect(posts).toBe(1);
  } finally {
    release();
  }
});

test("catalog failure never becomes a fake empty result and lookup remains available", async ({
  page,
}) => {
  await fixture(page);
  await page.route(base + "/services", (route) =>
    route.fulfill({ status: 503, json: { error: { code: "temporarily_unavailable" } } }),
  );
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/b/teste");
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "temporariamente indisponível",
  );
  await expect(page.getByRole("heading", { name: "Nenhum serviço disponível" })).toHaveCount(0);
  await page.screenshot({ path: "test-results/public-catalog-error-320.png", fullPage: true });
  await page.getByRole("button", { name: "Já tenho um código" }).click();
  await expect(page.getByRole("heading", { name: "Consultar meu agendamento" })).toBeFocused();
  await expect(page.getByLabel("Etapas do agendamento")).toHaveCount(0);
  await page.getByLabel("Código recebido").fill(code.toLowerCase());
  await page.getByRole("button", { name: "Consultar agendamento", exact: true }).click();
  await expect(page.getByLabel("Código do agendamento")).toHaveValue(code);
  await expect(page.getByRole("banner")).toContainText(appointment.shop_name);
  await page.screenshot({ path: "test-results/public-lookup-320.png", fullPage: true });
});

test("cancellation in flight keeps the lookup and mode controls locked", async ({ page }) => {
  await fixture(page);
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let cancels = 0;
  await page.route(base + "/appointments/cancel", async (route) => {
    cancels++;
    await gate;
    await route.fulfill({
      json: { appointment: { ...appointment, status: "cancelled" }, can_cancel: false },
    });
  });
  try {
    await page.goto("/b/teste");
    await page.getByRole("button", { name: "Já tenho um código" }).click();
    await page.getByLabel("Código recebido").fill(code);
    await page.getByRole("button", { name: "Consultar agendamento", exact: true }).click();
    await page.getByRole("button", { name: "Cancelar agendamento", exact: true }).click();
    await page.getByRole("button", { name: "Sim, cancelar agendamento" }).click();
    await expect(page.getByRole("button", { name: "Cancelando…" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Agendar horário", exact: true })).toBeDisabled();
    await expect(
      page.getByRole("button", { name: "Consultar agendamento", exact: true }),
    ).toBeDisabled();
    await expect(page.getByLabel("Código recebido")).toBeDisabled();
    expect(cancels).toBe(1);
    release();
    await expect(page.getByRole("status")).toContainText("O horário foi liberado");
    await expect(page.getByLabel("Código recebido")).toBeEnabled();
  } finally {
    release();
  }
});
