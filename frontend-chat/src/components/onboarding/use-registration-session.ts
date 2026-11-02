"use client";

import { useEffect, useRef, useState } from "react";
import type { User } from "@supabase/supabase-js";
import { getSupabaseBrowserClient } from "@/lib/supabase/browser";
import { clearRegistrationAttempt } from "@/lib/onboarding";
import { confirmationLinkError, readConfirmationCallback } from "@/lib/supabase/confirmation";

export function useRegistrationSession() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const callback = useRef<ReturnType<typeof readConfirmationCallback> | null>(null);
  useEffect(() => {
    let cancelled = false;
    let unsubscribe: (() => void) | undefined;
    setLoading(true);
    setError("");
    setUser(null);
    void (async () => {
      try {
        callback.current ??= readConfirmationCallback();
        const initialCallback = callback.current;
        const client = getSupabaseBrowserClient();
        // detectSessionInUrl performs a single PKCE exchange during SDK init.
        const initialized = await client.auth.initialize();
        const params = new URLSearchParams(window.location.search);
        // The SDK may already have removed `code`. This page has no supported
        // query/hash options, including arbitrary `next` redirect targets.
        if (window.location.search || window.location.hash)
          window.history.replaceState(window.history.state, "", "/cadastro/barbearia");
        const session = await client.auth.getSession();
        // A remaining code was not consumed (for example, a missing verifier).
        // A cached initialization error must not reject a later explicit login.
        if (
          initialCallback.hasError ||
          params.has("code") ||
          params.has("error") ||
          (initialized.error && (initialCallback.present || !session.data.session))
        )
          throw new Error(
            confirmationLinkError(initialCallback.errorCode || initialized.error?.code),
          );
        if (session.error)
          throw new Error("Não foi possível restaurar sua sessão. Entre novamente.");
        if (!session.data.session) return;
        const result = await client.auth.getUser();
        if (result.error || !result.data.user || result.data.user.is_anonymous)
          throw new Error("Sua sessão não foi aceita. Entre novamente para continuar.");
        if (cancelled) return;
        const currentUser = result.data.user;
        setUser(currentUser);
        const listener = client.auth.onAuthStateChange((event, next) => {
          if (event === "SIGNED_OUT" || (next && next.user.id !== currentUser.id)) {
            // No auth calls inside the SDK callback (the auth lock is held).
            try {
              clearRegistrationAttempt();
            } catch {
              /* Storage may be unavailable. */
            }
            setUser(null);
            setError("A conta desta aba mudou. Entre novamente antes de continuar.");
          }
        });
        unsubscribe = () => listener.data.subscription.unsubscribe();
      } catch (error) {
        if (!cancelled)
          setError(error instanceof Error ? error.message : "Não foi possível verificar a sessão.");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
      unsubscribe?.();
    };
  }, [revision]);
  return { user, loading, error, retry: () => setRevision((value) => value + 1) };
}
