import { test, expect } from "@playwright/test";
test.beforeEach(async ({ page }) => {
  await page.clock.install({ time: new Date("2026-09-22T10:00:00-03:00") });
});
test("service lifecycle persists and inactive services stay out of chat", async ({ page }) => {
  await page.goto("/admin/servicos");
  await page.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do serviço").fill("Corte de teste");
  await dialog.getByLabel("Descrição").fill("Cuidado especial de demonstração.");
  await dialog.getByLabel("Preço (R$)").fill("62.50");
  await dialog.getByLabel("Duração (minutos)").fill("45");
  await dialog.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  let card = page
    .locator("article")
    .filter({ has: page.getByRole("heading", { name: "Corte de teste", exact: true }) });
  await expect(card).toContainText("62,50");
  await page.reload();
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "Editar serviço" }).click();
  await dialog.getByLabel("Nome do serviço").fill("Corte exclusivo");
  await dialog.getByRole("switch").click();
  await dialog.getByRole("button", { name: "Salvar alterações" }).click();
  card = page
    .locator("article")
    .filter({ has: page.getByRole("heading", { name: "Corte exclusivo", exact: true }) });
  await expect(card).toContainText("Inativo");
  await page.goto("/chat");
  await expect(page.getByRole("button", { name: /Corte clássico/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Corte exclusivo/ })).toHaveCount(0);
  await page.goto("/admin/servicos");
  await page.getByRole("button", { name: "Excluir Corte exclusivo", exact: true }).click();
  await dialog.getByRole("button", { name: "Excluir serviço", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Corte exclusivo", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Corte clássico", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Corte exclusivo", exact: true })).toHaveCount(0);
});
test("manual booking can be edited and cancelled, keeping occupied times unavailable", async ({
  page,
}) => {
  await page.goto("/admin");
  await page.getByRole("button", { name: "Novo agendamento" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Nome do cliente").fill("Cliente Manual");
  await dialog.getByLabel("Telefone com DDD").fill("11988887777");
  await dialog.getByLabel("Data", { exact: true }).fill("2026-09-23");
  await dialog.getByRole("combobox", { name: "Horário", exact: true }).selectOption("12:00");
  await dialog.getByRole("button", { name: "Confirmar agendamento" }).click();
  await page.getByRole("button", { name: /Cliente Manual, Corte clássico, 12:00/ }).click();
  await expect(dialog).toContainText("Cliente Manual");
  await dialog.getByRole("button", { name: "Editar", exact: true }).click();
  await dialog.getByRole("combobox", { name: "Horário", exact: true }).selectOption("12:30");
  await dialog.getByRole("button", { name: "Salvar alterações" }).click();
  await page.getByRole("button", { name: "Novo agendamento" }).click();
  await dialog.getByLabel("Data", { exact: true }).fill("2026-09-23");
  await expect(
    dialog.getByRole("combobox", { name: "Horário", exact: true }).locator("option[value='12:30']"),
  ).toHaveCount(0);
  await dialog.getByRole("button", { name: "Fechar janela" }).click();
  await page.getByRole("button", { name: /Cliente Manual, Corte clássico, 12:30/ }).click();
  await dialog.getByRole("button", { name: "Cancelar", exact: true }).click();
  await dialog.getByRole("button", { name: "Sim, cancelar" }).click();
  await expect(page.getByRole("button", { name: /Cliente Manual, Corte clássico/ })).toHaveCount(0);
  await page.getByRole("button", { name: "Novo agendamento" }).click();
  await dialog.getByLabel("Data", { exact: true }).fill("2026-09-23");
  await expect(
    dialog.getByRole("combobox", { name: "Horário", exact: true }).locator("option[value='12:30']"),
  ).toHaveCount(1);
});
test("chat saves only on confirmation and reservation appears in admin", async ({ page }) => {
  await page.goto("/chat");
  await page.getByRole("button", { name: /Cabelo \+ barba/ }).click();
  await page.getByRole("button", { name: /Felipe Palma.*Escolher/ }).click();
  await page.getByLabel("Escolher outra data").fill("2026-09-24");
  await page.getByRole("button", { name: "12:00", exact: true }).click();
  await page.getByLabel("Seu nome").fill("Cliente Chat");
  await page.getByLabel("Telefone com DDD").fill("11999998888");
  await page.getByRole("button", { name: "Revisar agendamento" }).click();
  expect(await page.evaluate(() => localStorage.getItem("palma-barbearia-demo-v1"))).toBeNull();
  await page.getByRole("button", { name: "Confirmar meu agendamento" }).click();
  await expect(page.getByRole("heading", { name: "Seu horário está reservado." })).toBeVisible();
  await page.getByRole("link", { name: "Ver na agenda" }).click();
  const appointment = page.getByRole("button", { name: /Cliente Chat, Cabelo \+ barba, 12:00/ });
  await expect(appointment).toBeVisible();
  await appointment.click();
  await expect(page.getByRole("dialog")).toContainText("Agendado pelo chat");
});
test("calendar navigation, filters and month view work", async ({ page }) => {
  await page.goto("/admin");
  await expect(page.locator(".calendar-event")).toHaveCount(18);
  await page.getByLabel("Filtrar por profissional").selectOption("rafael");
  await expect(page.locator(".calendar-event")).toHaveCount(9);
  await page.getByLabel("Buscar na agenda").fill("Pedro");
  await expect(page.locator(".calendar-event")).toHaveCount(1);
  await page.getByLabel("Buscar na agenda").clear();
  await page.getByRole("button", { name: "Mês", exact: true }).click();
  await expect(page.locator(".month-cell")).toHaveCount(42);
  await page.getByRole("button", { name: "Próximo período" }).click();
  await expect(page.getByRole("heading", { name: "outubro de 2026" })).toBeVisible();
  await page.getByRole("button", { name: "Hoje", exact: true }).click();
  await expect(page.getByRole("heading", { name: "setembro de 2026" })).toBeVisible();
  await page.getByRole("button", { name: "Dia", exact: true }).click();
  await expect(page.locator(".day-column")).toHaveCount(1);
});
test("mobile layouts fit viewport and menu works", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  for (const path of ["/admin", "/admin/servicos", "/chat"]) {
    await page.goto(path);
    await expect(page.locator(".loading-screen")).toHaveCount(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBeTruthy();
    await page.screenshot({
      path: "test-results/" + path.replaceAll("/", "-") + "-mobile.png",
      fullPage: true,
    });
  }
  await page.goto("/admin");
  await page.getByRole("button", { name: "Abrir menu" }).click();
  await page.getByRole("navigation").getByRole("link", { name: "Serviços", exact: true }).click();
  await expect(page.getByRole("heading", { name: /Seu catálogo de serviços/ })).toBeVisible();
  await page.getByRole("button", { name: "Adicionar serviço", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("desktop screens have no client errors", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const path of ["/admin", "/admin/servicos", "/chat"]) {
    await page.goto(path);
    await expect(page.locator(".loading-screen")).toHaveCount(0);
    await page.screenshot({
      path: "test-results/" + path.replaceAll("/", "-") + "-desktop.png",
      fullPage: true,
    });
  }
  expect(errors).toEqual([]);
});
