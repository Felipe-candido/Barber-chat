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

export async function currentAccessToken(): Promise<string | null> {
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
  return data.session?.access_token ?? null;
}

export async function signOut(): Promise<void> {
  let client;
  try {
    client = getSupabaseBrowserClient();
  } catch (error) {
    throw configurationError(error);
  }
  const { error } = await client.auth.signOut();
  if (error) {
    throw new AuthenticationError(
      "Não foi possível encerrar a sessão. Tente novamente.",
      "sign_out_failed",
    );
  }
}
