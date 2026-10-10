"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase/browser";

export function SetPasswordPage() {
  const [ready, setReady] = useState(false);
  const [loading, setLoading] = useState(true);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const client = getSupabaseBrowserClient();
        const { data, error } = await client.auth.getUser();
        if (cancelled) return;
        if (error || !data.user)
          throw new Error(
            "Convite inválido ou expirado. Peça ao proprietário para conferir seu acesso.",
          );
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
  }, []);
  async function submit() {
    if (inFlight.current) return;
    if (password.length < 12 || password.length > 128 || password !== confirmation) {
      setError("Use uma senha de 12 a 128 caracteres e repita a mesma senha nos dois campos.");
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const { error } = await getSupabaseBrowserClient().auth.updateUser({ password });
      if (error)
        throw new Error("A senha não foi aceita. Confira as regras do Supabase e tente novamente.");
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
    <main className="access-screen">
      <section className="professional-card" style={{ maxWidth: 480, width: "100%" }}>
        <h1>Definir sua senha</h1>
        <p>Finalize seu acesso de barbeiro. Sua senha é enviada somente ao Supabase.</p>
        {loading && <p role="status">Validando convite…</p>}
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        {done ? (
          <>
            <p role="status">Senha definida. Entre no painel para validar seu acesso à agenda.</p>
            <Link className="button button-primary" href="/admin">
              Entrar no painel
            </Link>
          </>
        ) : (
          ready && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void submit();
              }}
            >
              <label className="booking-field">
                Nova senha
                <input
                  type="password"
                  autoComplete="new-password"
                  minLength={12}
                  maxLength={128}
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={busy}
                />
              </label>
              <label className="booking-field">
                Confirmar senha
                <input
                  type="password"
                  autoComplete="new-password"
                  minLength={12}
                  maxLength={128}
                  required
                  value={confirmation}
                  onChange={(e) => setConfirmation(e.target.value)}
                  disabled={busy}
                />
              </label>
              <button className="button button-primary" disabled={busy}>
                {busy ? "Salvando…" : "Salvar senha"}
              </button>
            </form>
          )
        )}
        {!done && <Link href="/login">Voltar ao login</Link>}
      </section>
    </main>
  );
}
