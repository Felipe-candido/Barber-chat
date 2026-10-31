"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { signUpOwner } from "@/lib/supabase/auth";
import { getSupabaseBrowserClient } from "@/lib/supabase/browser";
import { errorMessage } from "@/lib/api/client";
import { Alert, Button, TextField, buttonClasses } from "../ui";
import { PasswordField } from "../ui/password-field";
import { OnboardingShell } from "./onboarding-shell";
import { ErrorSummary } from "./error-summary";
import styles from "./onboarding.module.css";

export function SignupPage() {
  const router = useRouter();
  const [values, setValues] = useState({ display_name: "", email: "", password: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [activeSession, setActiveSession] = useState(false);
  const [checking, setChecking] = useState(true);
  const inFlight = useRef(false);
  const summaryRef = useRef<HTMLDivElement>(null);
  const labels = { display_name: "Seu nome", email: "E-mail", password: "Senha" };
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const { data, error } = await getSupabaseBrowserClient().auth.getSession();
        if (error) throw error;
        if (!cancelled) setActiveSession(!!data.session);
      } catch {
        if (!cancelled)
          setError(
            "Não foi possível verificar sua sessão. Confira a configuração do Supabase e recarregue a página.",
          );
      } finally {
        if (!cancelled) setChecking(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);
  function validate(field: keyof typeof values) {
    const value = values[field];
    if (field === "display_name")
      return !value.trim() ||
        [...value.trim()].length > 100 ||
        /[\u0000-\u001f\u007f-\u009f]/.test(value)
        ? "Informe seu nome, com até 100 caracteres."
        : "";
    if (field === "email")
      return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim()) && value.trim().length <= 254
        ? ""
        : "Informe um e-mail válido que você consegue acessar.";
    return value.length >= 8 && value.length <= 128 ? "" : "Use uma senha de 8 a 128 caracteres.";
  }
  async function submit() {
    if (inFlight.current || activeSession || checking) return;
    const next = Object.fromEntries(
      (Object.keys(values) as (keyof typeof values)[]).map((field) => [field, validate(field)]),
    );
    setErrors(next);
    if (Object.values(next).some(Boolean)) {
      requestAnimationFrame(() => summaryRef.current?.focus());
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await signUpOwner(values.email, values.password, values.display_name);
      setValues((current) => ({ ...current, password: "" }));
      if (result.hasSession) router.replace("/cadastro/barbearia");
      else setSent(true);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  return (
    <OnboardingShell step={sent ? 2 : 1}>
      <div>
        <h1>{sent ? "Confira seu e-mail" : "Comece pela sua conta"}</h1>
        <p className={styles.description}>
          {sent
            ? "Confirme o endereço de e-mail para continuar com a sua barbearia."
            : "Crie seu acesso de proprietário. Você não precisa informar cartão nesta etapa."}
        </p>
      </div>
      {error && <Alert tone="error">{error}</Alert>}
      {checking ? (
        <p role="status">Verificando sessão…</p>
      ) : sent ? (
        <>
          <Alert>
            Se o cadastro puder ser concluído, você receberá um link em {values.email.trim()}.
            Confira também o spam. Se já possui conta, entre pelo login.
          </Alert>
          <p className={styles.description}>
            Abra o link no mesmo navegador e endereço usados aqui (localhost e 127.0.0.1 são origens
            diferentes). Se abrir em outro dispositivo, confirme o e-mail e depois faça login.
          </p>
          <Link href="/cadastro/barbearia" className={buttonClasses()}>
            Já confirmei, continuar
          </Link>
          <Link href="/login" className={buttonClasses("secondary")}>
            Entrar com minha conta
          </Link>
          <Button
            variant="quiet"
            onClick={() => {
              setSent(false);
              setError("");
            }}
          >
            Corrigir e-mail
          </Button>
        </>
      ) : activeSession ? (
        <>
          <Alert>
            Você já está com uma sessão ativa. Continue com essa conta ou saia pelo painel antes de
            criar outra.
          </Alert>
          <Link href="/cadastro/barbearia" className={buttonClasses()}>
            Continuar cadastro da barbearia
          </Link>
          <Link href="/admin" className={buttonClasses("secondary")}>
            Minhas barbearias
          </Link>
        </>
      ) : (
        <form
          className={styles.form}
          noValidate
          aria-busy={busy}
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <ErrorSummary errors={errors} labels={labels} summaryRef={summaryRef} />
          <fieldset className={styles.fields} disabled={busy}>
            <TextField
              id="registration-display_name"
              label="Seu nome"
              autoComplete="name"
              required
              maxLength={100}
              value={values.display_name}
              error={errors.display_name}
              onChange={(e) => {
                setValues({ ...values, display_name: e.target.value });
                setErrors({ ...errors, display_name: "" });
              }}
              onBlur={() => setErrors({ ...errors, display_name: validate("display_name") })}
            />
            <TextField
              id="registration-email"
              label="E-mail"
              type="email"
              autoComplete="email"
              required
              maxLength={254}
              value={values.email}
              error={errors.email}
              hint="Use seu próprio e-mail. É por ele que você confirma sua conta."
              onChange={(e) => {
                setValues({ ...values, email: e.target.value });
                setErrors({ ...errors, email: "" });
              }}
              onBlur={() => setErrors({ ...errors, email: validate("email") })}
            />
            <PasswordField
              id="registration-password"
              label="Senha"
              visibilityLabel="senha"
              autoComplete="new-password"
              required
              minLength={8}
              maxLength={128}
              value={values.password}
              error={errors.password}
              hint="De 8 a 128 caracteres. Prefira uma senha longa e exclusiva."
              onChange={(e) => {
                setValues({ ...values, password: e.target.value });
                setErrors({ ...errors, password: "" });
              }}
              onBlur={() => setErrors({ ...errors, password: validate("password") })}
            />
          </fieldset>
          <Button type="submit" loading={busy} className={styles.fullWidth}>
            {busy ? "Criando conta…" : "Criar conta"}
          </Button>
        </form>
      )}
    </OnboardingShell>
  );
}
