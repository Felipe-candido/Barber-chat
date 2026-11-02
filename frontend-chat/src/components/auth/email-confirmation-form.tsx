"use client";

import { useEffect, useRef, useState } from "react";
import { resendSignupConfirmation } from "@/lib/supabase/auth";
import { Alert, Button, TextField } from "../ui";
import styles from "../onboarding/onboarding.module.css";

const RESEND_INTERVAL_MS = 60_000;

export function EmailConfirmationForm({
  initialEmail = "",
  recentlySent = false,
}: {
  initialEmail?: string;
  recentlySent?: boolean;
}) {
  const [email, setEmail] = useState(initialEmail);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [emailError, setEmailError] = useState("");
  const [sent, setSent] = useState(false);
  const [nextAllowedAt, setNextAllowedAt] = useState(() =>
    recentlySent ? Date.now() + RESEND_INTERVAL_MS : 0,
  );
  const [remaining, setRemaining] = useState(recentlySent ? 60 : 0);
  const inFlight = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  useEffect(() => {
    const tick = () => setRemaining(Math.max(0, Math.ceil((nextAllowedAt - Date.now()) / 1000)));
    tick();
    if (!nextAllowedAt) return;
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [nextAllowedAt]);

  async function submit() {
    if (inFlight.current || Date.now() < nextAllowedAt) return;
    if (email.trim().length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
      setEmailError("Informe o e-mail usado no cadastro.");
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    setSent(false);
    // A lost response might still have sent the email; no automatic retry.
    setNextAllowedAt(Date.now() + RESEND_INTERVAL_MS);
    try {
      await resendSignupConfirmation(email);
      if (alive.current) setSent(true);
    } catch (cause) {
      if (alive.current)
        setError(
          cause instanceof Error
            ? cause.message
            : "Não foi possível confirmar o reenvio. Confira seu e-mail antes de tentar novamente.",
        );
    } finally {
      inFlight.current = false;
      if (alive.current) setBusy(false);
    }
  }

  return (
    <form
      className={styles.form}
      aria-label="Reenviar confirmação de e-mail"
      aria-busy={busy}
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <TextField
        label="E-mail do cadastro"
        name="confirmation-email"
        type="email"
        autoComplete="email"
        required
        maxLength={254}
        value={email}
        disabled={busy}
        error={emailError}
        onChange={(event) => {
          setEmail(event.target.value);
          setEmailError("");
          setError("");
          setSent(false);
        }}
        hint="Use o mesmo endereço informado ao criar sua conta."
      />
      {error && <Alert tone="error">{error}</Alert>}
      {sent && (
        <Alert>
          Se houver um cadastro aguardando confirmação, você receberá um novo link. Confira a caixa
          de entrada e o spam e use o e-mail mais recente. Se já confirmou, faça login normalmente.
        </Alert>
      )}
      <Button type="submit" loading={busy} disabled={remaining > 0}>
        {busy
          ? "Solicitando link…"
          : remaining > 0
            ? `Reenviar confirmação em ${remaining}s`
            : "Reenviar confirmação"}
      </Button>
      <p className={styles.description}>
        Abra o novo link neste navegador, usando o mesmo endereço do app. Solicitar outro link não
        exige criar a conta novamente.
      </p>
    </form>
  );
}
