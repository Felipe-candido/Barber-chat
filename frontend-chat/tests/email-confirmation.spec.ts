import { createHash } from "node:crypto";
import { test, expect } from "@playwright/test";
import {
  createTestSession,
  installSupabaseSession,
  testEmail,
  testPassword,
} from "./supabase-fixture";

const resendURL = "https://test-project.supabase.co/auth/v1/resend**";

test("resend uses signup PKCE without credentials or API writes and completes the callback once", async ({
  page,
}) => {
  const session = createTestSession();
  let resends = 0,
    exchanges = 0,
    apiCalls = 0,
    signupCalls = 0;
  let challenge = "",
    redirect = "";
  await page.route("http://127.0.0.1:8080/**", (route) => {
    apiCalls++;
    return route.abort();
  });
  await page.route("https://test-project.supabase.co/auth/v1/signup**", (route) => {
    signupCalls++;
    return route.abort();
  });
  await page.route(resendURL, (route) => {
    resends++;
    const body = route.request().postDataJSON();
    expect(body).toMatchObject({ type: "signup", email: testEmail, code_challenge_method: "s256" });
    for (const field of ["password", "role", "user_id", "access_token", "refresh_token"])
      expect(body).not.toHaveProperty(field);
    challenge = body.code_challenge;
    redirect = new URL(route.request().url()).searchParams.get("redirect_to")!;
    const url = new URL(redirect);
    expect(url.origin + url.pathname).toBe("http://127.0.0.1:3100/cadastro/barbearia");
    return route.fulfill({ json: {} });
  });
  await page.route("https://test-project.supabase.co/auth/v1/token**", (route) => {
    exchanges++;
    const body = route.request().postDataJSON();
    expect(body.auth_code).toBe("resent-confirmation-code");
    expect(createHash("sha256").update(body.code_verifier).digest("base64url")).toBe(challenge);
    return route.fulfill({ json: session.response });
  });
  await page.route("https://test-project.supabase.co/auth/v1/user", (route) =>
    route.fulfill({ json: session.response.user }),
  );
  await page.goto("/confirmar-email");
  await page.getByRole("button", { name: "Reenviar confirmação", exact: true }).click();
  await expect(
    page.getByText("Informe o e-mail usado no cadastro.", { exact: true }),
  ).toBeVisible();
  expect(resends).toBe(0);
  await page.getByLabel("E-mail do cadastro").fill("  " + testEmail + "  ");
  await page.getByRole("button", { name: "Reenviar confirmação", exact: true }).click();
  await expect(page.getByRole("status")).toContainText(
    "Se houver um cadastro aguardando confirmação",
  );
  await expect(page.getByRole("button", { name: /Reenviar confirmação em/ })).toBeDisabled();
  expect(resends).toBe(1);
  expect(signupCalls).toBe(0);
  expect(apiCalls).toBe(0);
  const callback = new URL(redirect);
  callback.searchParams.set("code", "resent-confirmation-code");
  callback.searchParams.set("next", "https://untrusted.example");
  await page.goto(callback.href);
  await expect(page.getByLabel("Nome da barbearia")).toBeVisible();
  await expect(page).toHaveURL("/cadastro/barbearia");
  expect(exchanges).toBe(1);
  expect(apiCalls).toBe(0);
});

test("signup offers a prefilled resend with cooldown instead of recreating the account", async ({
  page,
}) => {
  await page.clock.install();
  const session = createTestSession();
  let signups = 0,
    resends = 0;
  await page.route("https://test-project.supabase.co/auth/v1/signup**", (route) => {
    signups++;
    return route.fulfill({ json: { ...session.response.user, email_confirmed_at: null } });
  });
  await page.route(resendURL, (route) => {
    resends++;
    return route.fulfill({ json: {} });
  });
  await page.goto("/cadastro");
  await page.getByLabel("Seu nome", { exact: true }).fill("Felipe");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  await page.getByLabel("Senha", { exact: true }).fill(testPassword);
  await page.getByRole("button", { name: "Criar conta", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Confira seu e-mail" })).toBeVisible();
  await expect(page.getByLabel("E-mail do cadastro")).toHaveValue(testEmail);
  await expect(page.getByRole("button", { name: /Reenviar confirmação em/ })).toBeDisabled();
  expect(resends).toBe(0);
  await page.clock.fastForward(61_000);
  await page.getByRole("button", { name: "Reenviar confirmação", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Se houver um cadastro" })).toBeVisible();
  expect(signups).toBe(1);
  expect(resends).toBe(1);
});

test("unconfirmed login offers confirmation instead of a wrong-password message", async ({
  page,
}) => {
  let apiCalls = 0;
  await page.route("http://127.0.0.1:8080/**", (route) => {
    apiCalls++;
    return route.abort();
  });
  await page.route("https://test-project.supabase.co/auth/v1/token**", (route) =>
    route.fulfill({
      status: 400,
      json: { code: 400, error_code: "email_not_confirmed", msg: "Email not confirmed" },
    }),
  );
  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill(testEmail);
  await page.getByLabel("Senha", { exact: true }).fill(testPassword);
  await page.getByRole("button", { name: "Entrar no painel" }).click();
  await expect(page.locator(".login-form-error")).toContainText(
    "Confirme seu e-mail antes de entrar",
  );
  await expect(page.locator(".login-form-error")).not.toContainText("senha inválidos");
  await page.getByRole("link", { name: "Reenviar confirmação de e-mail" }).click();
  await expect(page.getByLabel("E-mail do cadastro")).toBeVisible();
  expect(apiCalls).toBe(0);
});

for (const fragment of [true, false]) {
  test(`expired ${fragment ? "fragment" : "query"} callback cannot use another session, including after retry`, async ({
    page,
  }) => {
    await installSupabaseSession(page);
    const error =
      "error=access_denied&error_code=otp_expired&error_description=private-provider-detail&next=https://untrusted.example";
    await page.goto("/cadastro/barbearia" + (fragment ? "#" : "?") + error);
    const callbackError = page
      .getByRole("alert")
      .filter({ hasText: "Não foi possível validar o link" });
    await expect(callbackError).toContainText("expirou ou já foi usado");
    await expect(page.getByLabel("Nome da barbearia")).toHaveCount(0);
    await expect(page).toHaveURL("/cadastro/barbearia");
    await expect(page.locator("body")).not.toContainText("private-provider-detail");
    await page.getByRole("button", { name: "Verificar sessão novamente" }).click();
    await expect(callbackError).toContainText("expirou ou já foi usado");
    await expect(page.getByLabel("Nome da barbearia")).toHaveCount(0);
    await page.getByRole("link", { name: "Reenviar confirmação", exact: true }).click();
    await expect(page).toHaveURL("/confirmar-email");
  });
}

for (const failure of ["rate_limit", "lost_response", "unknown_account"] as const) {
  test(`resend ${failure} is explicit, bounded and never retried automatically`, async ({
    page,
  }) => {
    await page.clock.install();
    let calls = 0;
    let release: (() => void) | undefined;
    await page.route(resendURL, async (route) => {
      calls++;
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      if (failure === "lost_response") return route.abort("failed");
      if (failure === "rate_limit")
        return route.fulfill({
          status: 429,
          json: {
            code: 429,
            error_code: "over_email_send_rate_limit",
            msg: "private-provider-detail",
          },
        });
      return route.fulfill({
        status: 400,
        json: { code: 400, error_code: "user_not_found", msg: "private-provider-detail" },
      });
    });
    await page.goto("/confirmar-email");
    await page.getByLabel("E-mail do cadastro").fill(testEmail);
    await page.getByRole("form", { name: "Reenviar confirmação de e-mail" }).evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
      (form as HTMLFormElement).requestSubmit();
    });
    await expect(page.getByRole("button", { name: "Solicitando link…" })).toBeDisabled();
    await expect.poll(() => calls).toBe(1);
    release!();
    if (failure === "unknown_account")
      await expect(page.getByRole("status")).toContainText(
        "Se houver um cadastro aguardando confirmação",
      );
    else
      await expect(
        page.getByRole("form", { name: "Reenviar confirmação de e-mail" }).getByRole("alert"),
      ).toContainText(
        failure === "rate_limit" ? "limite de envio" : "Não foi possível confirmar o reenvio",
      );
    await expect(page.getByLabel("E-mail do cadastro")).toHaveValue(testEmail);
    await expect(page.locator("body")).not.toContainText("private-provider-detail");
    await page.clock.fastForward(61_000);
    await expect(
      page.getByRole("button", { name: "Reenviar confirmação", exact: true }),
    ).toBeEnabled();
    expect(calls).toBe(1);
  });
}

test("login explains an expired link without reflecting provider details or retaining callback parameters", async ({
  page,
}) => {
  await page.goto(
    "/login#error=access_denied&error_code=otp_expired&error_description=private-provider-detail",
  );
  await expect(page.locator(".login-form-error")).toContainText("expirou ou já foi usado");
  await expect(page.locator("body")).not.toContainText("private-provider-detail");
  await expect(page).toHaveURL("/login");
  await expect(page.getByRole("link", { name: "Reenviar confirmação de e-mail" })).toBeVisible();
});
