import { test, expect } from "@playwright/test";
import { securityHeaders } from "../src/lib/security-headers";

test("HTML uses fresh server nonces and rejects a caller's CSP override", async ({ request }) => {
  const first = await request.get("/login", {
    headers: { "x-nonce": "attacker", "content-security-policy": "script-src 'unsafe-inline'" },
  });
  const second = await request.get("/login");
  expect(first.ok()).toBe(true);
  expect(second.ok()).toBe(true);
  const csp = first.headers()["content-security-policy"];
  const nonce = csp.match(/'nonce-([^']+)'/)?.[1];
  expect(nonce).toBeTruthy();
  expect(nonce).not.toBe("attacker");
  expect(second.headers()["content-security-policy"]).not.toContain(`nonce-${nonce}`);
  const scripts = (await first.text()).match(/<script\b[^>]*>/g) ?? [];
  expect(scripts.length).toBeGreaterThan(0);
  for (const tag of scripts) {
    if (!/type="application\/(?:ld\+)?json"/.test(tag)) expect(tag).toContain(`nonce="${nonce}"`);
  }
  expect(csp).toContain("frame-ancestors 'none'");
  expect(csp.match(/script-src[^;]+/)?.[0]).not.toMatch(/unsafe-inline|unsafe-eval/);
  expect(first.headers()["x-frame-options"]).toBe("DENY");
  expect(first.headers()["x-content-type-options"]).toBe("nosniff");
  expect(first.headers()["referrer-policy"]).toBe("no-referrer");
  expect(first.headers()["cache-control"]).toContain("no-store");
});

test("production origins are restricted and HTTPS policy preserves local tests", () => {
  const policy = securityHeaders(
    "test-nonce",
    "https://api.example",
    "https://project.supabase.co",
    false,
    true,
  );
  expect(policy).toContain("upgrade-insecure-requests");
  expect(policy).toContain("wss://project.supabase.co");
  expect(policy).not.toContain("'unsafe-eval'");
  expect(() =>
    securityHeaders("test-nonce", "http://api.example", "https://project.supabase.co", false, true),
  ).toThrow();
  expect(() =>
    securityHeaders(
      "test-nonce",
      "https://user:password@api.example",
      "https://project.supabase.co",
      false,
      true,
    ),
  ).toThrow();
  expect(
    securityHeaders(
      "test-nonce",
      "http://127.0.0.1:8080",
      "https://project.supabase.co",
      false,
      false,
    ),
  ).not.toContain("upgrade-insecure-requests");
});

test("login hydrates and accepts input without CSP violations", async ({ page }) => {
  await page.addInitScript(() => {
    const violations: string[] = [];
    Object.defineProperty(window, "__securityViolations", { value: violations });
    document.addEventListener("securitypolicyviolation", (event) =>
      violations.push(event.violatedDirective),
    );
  });
  await page.goto("/login");
  await page.locator('input[type="email"]').fill("user@example.test");
  await expect(page.locator('input[type="email"]')).toHaveValue("user@example.test");
  expect(
    await page.evaluate(
      () => (window as unknown as { __securityViolations: string[] }).__securityViolations,
    ),
  ).toEqual([]);
});
