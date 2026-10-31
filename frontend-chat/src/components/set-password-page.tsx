"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { LoaderCircle } from "lucide-react";
import { getSupabaseBrowserClient } from "@/lib/supabase/browser";
import { Alert, Button, buttonClasses } from "./ui";
import { AccessPage } from "./ui/access-page";
import { PasswordField } from "./ui/password-field";
import styles from "./ui/access-page.module.css";

export function SetPasswordPage() {
  const [ready, setReady] = useState(false);
  const [loading, setLoading] = useState(true);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState({ password: "", confirmation: "" });
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const confirmationInput = useRef<HTMLInputElement>(null);
  const passwordInput = useRef<HTMLInputElement>(null);
  const inFlight = useRef(false);
  useEffect(() => {
    let cancelled = false;
    setReady(false);
    setLoading(true);
    setError("");
    void (async () => {
      try {
        const { data, error } = await getSupabaseBrowserClient().auth.getUser();
        if (cancelled) return;
        if (error || !data.user) throw new Error("Invalid invitation");
        setReady(true);
      } catch {
        if (!cancelled)
          setError(
            "Não foi possível validar o convite. Confira o link recebido ou entre com uma conta válida.",
          );
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [revision]);
  async function submit() {
    if (inFlight.current || !ready) return;
    const passwordError =
      password.length < 8 || password.length > 128 ? "Use uma senha de 8 a 128 caracteres." : "";
    const confirmationError =
      password !== confirmation ? "Repita a mesma senha nos dois campos." : "";
    setFieldErrors({ password: passwordError, confirmation: confirmationError });
    if (passwordError || confirmationError) {
      (passwordError ? passwordInput : confirmationInput).current?.focus();
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const { error } = await getSupabaseBrowserClient().auth.updateUser({ password });
      if (error) throw new Error("Password rejected");
      setPassword("");
      setConfirmation("");
      setDone(true);
    } catch {
      setError(
        "Não foi possível definir a senha. Confira o convite e use uma senha forte com letras, números e símbolos.",
      );
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  return (
    <AccessPage
      title="Definir sua senha"
      description="Finalize seu acesso de barbeiro. Sua senha é enviada somente ao Supabase."
    >
      {loading && (
        <p className={styles.loading} role="status">
          <LoaderCircle size={22} aria-hidden="true" />
          Validando convite…
        </p>
      )}
      {error && <Alert tone="error">{error}</Alert>}
      {done ? (
        <>
          <Alert tone="success">
            Senha definida. Entre no painel para validar seu acesso à agenda.
          </Alert>
          <Link className={buttonClasses()} href="/admin">
            Entrar no painel
          </Link>
        </>
      ) : ready ? (
        <form
          className={styles.form}
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <fieldset className={styles.fields} disabled={busy}>
            <PasswordField
              ref={passwordInput}
              label="Nova senha"
              visibilityLabel="nova senha"
              autoComplete="new-password"
              minLength={8}
              maxLength={128}
              required
              value={password}
              hint="Use de 8 a 128 caracteres. Prefira uma senha longa e exclusiva."
              error={fieldErrors.password}
              onBlur={() =>
                setFieldErrors((current) => ({
                  ...current,
                  password:
                    password && password.length < 8 ? "Use uma senha de 8 a 128 caracteres." : "",
                }))
              }
              onChange={(event) => {
                setPassword(event.target.value);
                setFieldErrors({ password: "", confirmation: "" });
              }}
            />
            <PasswordField
              ref={confirmationInput}
              label="Confirmar senha"
              visibilityLabel="confirmação da senha"
              autoComplete="new-password"
              minLength={8}
              maxLength={128}
              required
              value={confirmation}
              error={fieldErrors.confirmation}
              onBlur={() =>
                setFieldErrors((current) => ({
                  ...current,
                  confirmation:
                    confirmation && confirmation !== password
                      ? "Repita a mesma senha nos dois campos."
                      : "",
                }))
              }
              onChange={(event) => {
                setConfirmation(event.target.value);
                setFieldErrors((current) => ({ ...current, confirmation: "" }));
              }}
            />
          </fieldset>
          <Button type="submit" loading={busy}>
            {busy ? "Salvando…" : "Salvar senha"}
          </Button>
        </form>
      ) : (
        !loading && (
          <Button variant="secondary" onClick={() => setRevision((value) => value + 1)}>
            Verificar convite novamente
          </Button>
        )
      )}
      {!done && (
        <Link
          href="/login"
          className={styles.textLink}
          aria-disabled={busy || undefined}
          onClick={(event) => {
            if (inFlight.current) event.preventDefault();
          }}
        >
          Voltar ao login
        </Link>
      )}
    </AccessPage>
  );
}
