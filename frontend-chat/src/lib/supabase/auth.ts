import type { Session } from "@supabase/supabase-js";
import { getSupabaseBrowserClient } from "./browser";

export class AuthenticationError extends Error {
  constructor(
    message: string,
    public readonly code = "authentication_failed",
  ) {
    super(message);
    this.name = "AuthenticationError";
  }
}

function configurationError(error: unknown): AuthenticationError {
  return new AuthenticationError(
    error instanceof Error ? error.message : "Não foi possível configurar a autenticação.",
    "configuration",
  );
}

export async function signInWithPassword(email: string, password: string): Promise<Session> {
  let client;
  try {
    client = getSupabaseBrowserClient();
  } catch (error) {
    throw configurationError(error);
  }

  const result = await client.auth.signInWithPassword({ email: email.trim(), password });
  if (result.error) {
    const genericCredentialsError = result.error.status === 400 || result.error.status === 401;
    throw new AuthenticationError(
      genericCredentialsError
        ? "E-mail ou senha inválidos. Confira os dados e tente novamente."
        : "Não foi possível entrar agora. Tente novamente em instantes.",
      result.error.code ?? "authentication_failed",
    );
  }
  if (!result.data.session?.access_token) {
    throw new AuthenticationError("O Supabase não retornou uma sessão válida.", "missing_session");
  }
  return result.data.session;
}

// The SDK manages PKCE and exchanges the confirmation code on the fixed page.
// Metadata is a UI prefill only, never a source of permissions for the Go API.
export async function signUpOwner(email: string, password: string, displayName: string) {
  const { data, error } = await getSupabaseBrowserClient().auth.signUp({
    email: email.trim(),
    password,
    options: {
      emailRedirectTo: new URL("/cadastro/barbearia", window.location.origin).href,
      data: { display_name: displayName.trim() },
    },
  });
  if (error) {
    const message =
      error.code === "weak_password"
        ? "A senha não atende à política de segurança. Use uma senha longa e exclusiva."
        : error.status === 429
          ? "Muitas tentativas. Aguarde alguns minutos antes de tentar novamente."
          : "Não foi possível confirmar o cadastro. Confira seu e-mail antes de tentar novamente; se já possui conta, entre pelo login.";
    throw new AuthenticationError(message, error.code ?? "signup_failed");
  }
  return { hasSession: !!data.session?.access_token };
}

export async function currentAccessToken(expectedUserID?: string): Promise<string | null> {
  let client;
  try {
    client = getSupabaseBrowserClient();
  } catch (error) {
    throw configurationError(error);
  }
  const { data, error } = await client.auth.getSession();
  if (error) {
    throw new AuthenticationError(
      "Não foi possível restaurar a sessão. Entre novamente.",
      "session_unavailable",
    );
  }
  if (expectedUserID && data.session?.user.id !== expectedUserID) {
    throw new AuthenticationError(
      "A conta mudou. Entre com a conta que iniciou este cadastro.",
      "session_changed",
    );
  }
  return data.session?.access_token ?? null;
}

export async function signOut(): Promise<void> {
  let client;
  try {
    client = getSupabaseBrowserClient();
  } catch (error) {
    throw configurationError(error);
  }
  const { error } = await client.auth.signOut({ scope: "local" });
  if (error) {
    throw new AuthenticationError(
      "Não foi possível encerrar a sessão. Tente novamente.",
      "sign_out_failed",
    );
  }
}
