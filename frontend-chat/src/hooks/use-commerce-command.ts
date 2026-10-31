"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import type { ShopRequest } from "@/lib/api/products";

type Attempt<T> = { version: 1; key: string; body: T };
export function useCommerceCommand<T, R>(
  scope: ShopRequest,
  operation: string,
  valid: (v: unknown) => v is T,
  execute: (body: T, key: string, signal: AbortSignal) => Promise<R>,
) {
  const storageKey = "barber:command:v1:" + scope.shopID + ":" + scope.userID + ":" + operation;
  const [attempt, setAttempt] = useState<Attempt<T> | null>(null);
  const [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [corrupt, setCorrupt] = useState(false);
  const [error, setError] = useState(""),
    [errorCode, setErrorCode] = useState("");
  const active = useRef(true),
    running = useRef(false),
    controller = useRef<AbortController | null>(null);
  const stored = useRef<Attempt<T> | null>(null);
  useEffect(() => {
    active.current = true;
    try {
      const raw = sessionStorage.getItem(storageKey);
      if (raw) {
        if (raw.length > 32768) throw new Error("invalid stored command");
        const saved = JSON.parse(raw) as Attempt<unknown>;
        if (saved.version !== 1 || !/^[a-f0-9]{64}$/.test(saved.key) || !valid(saved.body))
          throw new Error("invalid stored command");
        stored.current = saved as Attempt<T>;
        setAttempt(stored.current);
      }
    } catch {
      setCorrupt(true);
      setError(
        "Não foi possível recuperar a tentativa desta aba. Confira o histórico antes de encerrar a tentativa.",
      );
    }
    setReady(true);
    return () => {
      active.current = false;
      controller.current?.abort();
    };
  }, [storageKey, valid]);
  async function submit(body?: T): Promise<R | undefined> {
    if (running.current || !ready || corrupt) return;
    running.current = true;
    setBusy(true);
    setError("");
    setErrorCode("");
    const hadAttempt = !!stored.current;
    try {
      if (!stored.current) {
        if (body === undefined || !valid(body))
          throw new Error("Revise os campos antes de confirmar.");
        const key = Array.from(crypto.getRandomValues(new Uint8Array(32)), (b) =>
          b.toString(16).padStart(2, "0"),
        ).join("");
        const next: Attempt<T> = { version: 1, key, body: JSON.parse(JSON.stringify(body)) as T };
        // Persist BEFORE sending. An unmount, lost response or reload cannot
        // silently turn an additive write into another command.
        sessionStorage.setItem(storageKey, JSON.stringify(next));
        stored.current = next;
        setAttempt(next);
      }
      const c = new AbortController();
      controller.current = c;
      const result = await execute(stored.current.body, stored.current.key, c.signal);
      sessionStorage.removeItem(storageKey);
      stored.current = null;
      if (active.current) {
        setAttempt(null);
        return result;
      }
    } catch (cause) {
      if (!active.current) return;
      setError(errorMessage(cause));
      setErrorCode(cause instanceof ApiError ? cause.code : "");
      const definitive =
        cause instanceof ApiError &&
        cause.status >= 400 &&
        cause.status < 500 &&
        ![
          "invalid_response",
          "idempotency_conflict",
          "sale_command_pending",
          "product_command_pending",
          "appointment_command_pending",
        ].includes(cause.code);
      if (definitive && !hadAttempt) {
        try {
          sessionStorage.removeItem(storageKey);
          stored.current = null;
          setAttempt(null);
        } catch {
          setCorrupt(true);
        }
      }
    } finally {
      running.current = false;
      if (active.current) setBusy(false);
    }
  }
  function discard() {
    if (running.current) return;
    try {
      sessionStorage.removeItem(storageKey);
      stored.current = null;
      setAttempt(null);
      setCorrupt(false);
      setError("");
      setErrorCode("");
    } catch {
      setError(
        "O navegador não permitiu encerrar a tentativa. Confira as permissões de armazenamento.",
      );
    }
  }
  return {
    submit,
    discard,
    attempt: attempt?.body ?? null,
    ready,
    busy,
    corrupt,
    error,
    errorCode,
    locked: !ready || busy || !!attempt || corrupt,
  };
}
