"use client";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import { openBillingPortal, type BillingPortal } from "@/lib/api/billing-portal";

export function useBillingPortal(slug: string, shopID: string, userID: string) {
  const [mode, setMode] = useState<"closed" | "loading" | "ready" | "error">("closed");
  const [session, setSession] = useState<BillingPortal | null>(null);
  const [error, setError] = useState("");
  const busy = useRef(false);
  const generation = useRef(0);
  const abort = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      generation.current++;
      abort.current?.abort();
      busy.current = false;
    },
    [slug, shopID, userID],
  );
  useEffect(() => {
    if (mode !== "ready" || !session) return;
    const timer = setTimeout(
      () => {
        setSession(null);
        setMode("error");
        setError("O prazo para abrir este link terminou. Prepare um novo acesso ao portal.");
      },
      Math.max(0, Date.parse(session.open_until) - Date.now() - 30_000),
    );
    return () => clearTimeout(timer);
  }, [mode, session]);
  async function open() {
    if (busy.current) return;
    busy.current = true;
    const current = ++generation.current;
    const controller = new AbortController();
    abort.current = controller;
    setSession(null);
    setError("");
    setMode("loading");
    try {
      const result = await openBillingPortal(slug, shopID, userID, controller.signal);
      if (generation.current !== current) return;
      setSession(result);
      setMode("ready");
    } catch (failure) {
      if (generation.current !== current) return;
      setError(errorMessage(failure));
      setMode("error");
    } finally {
      if (generation.current === current) busy.current = false;
    }
  }
  function close() {
    generation.current++;
    abort.current?.abort();
    busy.current = false;
    setSession(null);
    setError("");
    setMode("closed");
  }
  return { mode, session, error, open, close, pending: mode === "loading" };
}
