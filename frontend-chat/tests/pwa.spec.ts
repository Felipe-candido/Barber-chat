import { test, expect } from "@playwright/test";

test("manifest, Apple metadata and opaque PNG icons are available", async ({ request, page }) => {
  const response = await request.get("/manifest.webmanifest");
  expect(response.ok()).toBe(true);
  expect(response.headers()["content-type"]).toContain("manifest+json");
  const manifest = await response.json();
  expect(manifest).toMatchObject({
    id: "/",
    short_name: "Na Regua",
    start_url: "/admin",
    scope: "/",
    display: "standalone",
    lang: "pt-BR",
  });
  expect(manifest.icons.some((icon: { purpose: string }) => icon.purpose === "maskable")).toBe(
    true,
  );
  for (const icon of [
    ...manifest.icons,
    { src: "/icons/apple-touch-icon.png", sizes: "180x180" },
  ]) {
    const png = await request.get(icon.src);
    expect(png.ok()).toBe(true);
    expect(png.headers()["content-type"]).toContain("image/png");
    const bytes = await png.body();
    expect(bytes.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
    expect(`${bytes.readUInt32BE(16)}x${bytes.readUInt32BE(20)}`).toBe(icon.sizes);
  }
  await page.goto("/login");
  await expect(page.locator('link[rel="manifest"]')).toHaveAttribute(
    "href",
    "/manifest.webmanifest",
  );
  await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveAttribute(
    "href",
    "/icons/apple-touch-icon.png",
  );
  await expect(page.locator('meta[name="mobile-web-app-capable"]')).toHaveAttribute(
    "content",
    "yes",
  );
});

test("worker serves offline explanation without caching private pages or replaying writes", async ({
  page,
  context,
  request,
}) => {
  const response = await request.get("/sw.js");
  expect(response.headers()["cache-control"]).toContain("no-store");
  expect(response.headers()["content-type"]).toContain("javascript");
  expect(response.headers()["service-worker-allowed"]).toBe("/");
  await page.goto("/login");
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready;
    if (!navigator.serviceWorker.controller) {
      await new Promise<void>((resolve) =>
        navigator.serviceWorker.addEventListener("controllerchange", () => resolve(), {
          once: true,
        }),
      );
    }
  });
  const cached = await page.evaluate(async () => {
    const keys = await caches.keys();
    return Promise.all(
      keys
        .filter((key) => key.startsWith("na-regua-offline-"))
        .map(async (key) =>
          (await (await caches.open(key)).keys()).map((entry) => new URL(entry.url).pathname),
        ),
    );
  });
  expect(cached).toHaveLength(1);
  expect(cached[0].sort()).toEqual(
    [
      "/offline.html",
      "/offline.css",
      "/icons/icon-192.png",
      "/icons/icon-512.png",
      "/icons/maskable-512.png",
      "/icons/apple-touch-icon.png",
    ].sort(),
  );
  await context.setOffline(true);
  const apiFailed = await page.evaluate(async () => {
    try {
      await fetch("/api/v1/pwa-test");
      return false;
    } catch {
      return true;
    }
  });
  const writeFailed = await page.evaluate(async () => {
    try {
      await fetch("/api/v1/pwa-test", { method: "POST", body: "no offline booking" });
      return false;
    } catch {
      return true;
    }
  });
  expect(apiFailed).toBe(true);
  expect(writeFailed).toBe(true);
  await page.goto("/admin?offline-test=1");
  await expect(page.getByRole("heading", { name: "Você está sem conexão" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Tentar novamente" })).toHaveAttribute(
    "href",
    "/admin",
  );
  expect(await page.locator("main").evaluate((element) => getComputedStyle(element).maxWidth)).toBe(
    "448px",
  );
  await context.setOffline(false);
  await page.getByRole("link", { name: "Tentar novamente" }).click();
  await expect(page.getByRole("heading", { name: "Você está sem conexão" })).toHaveCount(0);
});

test.beforeEach(async ({ page }) => {
  // Browser eligibility varies by platform; test the native event deterministically.
  await page.addInitScript(() =>
    window.addEventListener("beforeinstallprompt", (event) => {
      if (event.isTrusted) {
        event.preventDefault();
        event.stopImmediatePropagation();
      }
    }),
  );
});

test("download invokes a captured prompt once and hides after installation", async ({ page }) => {
  await page.goto("/login");
  const button = page.getByRole("button", { name: "Baixar app" });
  await expect(button).toBeVisible();
  await page.evaluate(() => {
    const event = new Event("beforeinstallprompt", { cancelable: true });
    Object.defineProperties(event, {
      prompt: {
        value: async () => {
          document.body.dataset.promptCalls = String(
            Number(document.body.dataset.promptCalls ?? 0) + 1,
          );
        },
      },
      userChoice: { value: Promise.resolve({ outcome: "accepted" }) },
    });
    window.dispatchEvent(event);
  });
  await button.click();
  await expect(page.locator("body")).toHaveAttribute("data-prompt-calls", "1");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.evaluate(() => window.dispatchEvent(new Event("appinstalled")));
  await expect(button).toHaveCount(0);
});

test("dismissed prompt is consumed and the next click provides manual instructions", async ({
  page,
}) => {
  await page.goto("/login");
  const button = page.getByRole("button", { name: "Baixar app" });
  await expect(button).toBeVisible();
  await page.evaluate(() => {
    const event = new Event("beforeinstallprompt", { cancelable: true });
    Object.defineProperties(event, {
      prompt: { value: async () => {} },
      userChoice: { value: Promise.resolve({ outcome: "dismissed" }) },
    });
    window.dispatchEvent(event);
  });
  await button.click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await button.click();
  await expect(page.getByRole("dialog", { name: "Instalar Na Regua" })).toBeVisible();
  await page.getByRole("button", { name: "Entendi" }).click();
  await expect(button).toBeFocused();
});

test("iPhone has home-screen instructions and installed apps hide the control", async ({
  page,
  context,
}) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "userAgent", {
      get: () => "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)",
    }),
  );
  await page.goto("/login");
  await page.getByRole("button", { name: "Baixar app" }).click();
  await expect(page.getByText("Adicionar à Tela de Início", { exact: true })).toBeVisible();
  const installedPage = await context.newPage();
  await installedPage.addInitScript(() =>
    Object.defineProperty(navigator, "standalone", { get: () => true }),
  );
  await installedPage.goto("/login");
  await expect(installedPage.locator('input[type="email"]')).toBeVisible();
  await expect(installedPage.getByRole("button", { name: "Baixar app" })).toHaveCount(0);
});

test("landing page offers installation on a narrow viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  const button = page.getByRole("button", { name: "Baixar app" });
  await button.scrollIntoViewIfNeeded();
  await button.click();
  await expect(page.getByRole("dialog", { name: "Instalar Na Regua" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});
