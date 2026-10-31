"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  getCommercialStatus,
  hasCurrentCoverage,
  setShopPublication,
  type CommercialStatus,
} from "@/lib/api/shop-commercial";

type Mode = "loading" | "ready" | "load-error" | "saving" | "uncertain" | "blocked";

export function useShopCommercial(
  slug: string,
  shopID: string,
  userID: string,
  settingsVersion?: number,
) {
  const [mode, setMode] = useState<Mode>("loading");
  const [status, setStatus] = useState<CommercialStatus | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const generation = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const busy = useRef(false);
  const uncertain = useRef(false);

  const accept = useCallback((value: CommercialStatus) => {
    setStatus(value);
    setError("");
    uncertain.current = false;
    setMode("ready");
  }, []);

  const refresh = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    const attempt = ++generation.current;
    const abort = new AbortController();
    controller.current?.abort();
    controller.current = abort;
    const checking = uncertain.current;
    setMode("loading");
    setStatus(null);
    setError("");
    setNotice("");
    try {
      const current = await getCommercialStatus(slug, shopID, userID, abort.signal);
      if (generation.current !== attempt) return;
      accept(current);
      if (checking) setNotice("Situação atual conferida na API. Nenhuma ação foi reenviada.");
    } catch (failure) {
      if (generation.current !== attempt) return;
      setError(errorMessage(failure));
      setMode(accessFailure(failure) ? "blocked" : checking ? "uncertain" : "load-error");
    } finally {
      if (generation.current === attempt) busy.current = false;
    }
  }, [slug, shopID, userID, accept]);

  useEffect(() => {
    void refresh();
    return () => {
      generation.current++;
      controller.current?.abort();
      busy.current = false;
    };
  }, [refresh]);

  // A successful settings save may resolve address readiness. Never reset its
  // draft or interrupt a publication write to update this independent projection.
  const previousVersion = useRef(settingsVersion);
  useEffect(() => {
    const changed =
      previousVersion.current !== undefined && settingsVersion !== previousVersion.current;
    previousVersion.current = settingsVersion;
    if (changed && !uncertain.current) void refresh();
  }, [settingsVersion, refresh]);

  useEffect(() => {
    if (
      mode !== "ready" ||
      !status?.publication_managed ||
      !status.subscription.coverage_until ||
      !status.subscription.operation_allowed
    )
      return;
    const remaining = Date.parse(status.subscription.coverage_until) - Date.now();
    const timer = window.setTimeout(
      () => {
        if (remaining <= 0) {
          setStatus(null);
          setMode("load-error");
          setError(
            "O período informado terminou. Consulte a situação atual antes de publicar ou compartilhar.",
          );
        } else void refresh();
      },
      Math.max(0, Math.min(remaining, 2_147_000_000)),
    );
    return () => clearTimeout(timer);
  }, [mode, status, refresh]);

  async function setRequested(requested: boolean) {
    if (
      busy.current ||
      mode !== "ready" ||
      !status?.publication_managed ||
      (requested && (!status.can_publish || !hasCurrentCoverage(status)))
    )
      return;
    busy.current = true;
    const attempt = ++generation.current;
    const abort = new AbortController();
    controller.current = abort;
    setMode("saving");
    setError("");
    setNotice("");
    // Any failure after dispatch requires a fresh observation, not an automatic
    // PUT retry. Even a confirmed 204 is followed by GET before showing a link.
    uncertain.current = true;
    try {
      await setShopPublication(slug, userID, requested, abort.signal);
      const current = await getCommercialStatus(slug, shopID, userID, abort.signal);
      if (generation.current !== attempt) return;
      accept(current);
      setNotice(
        current.publication_requested === requested
          ? requested
            ? "Publicação confirmada na API."
            : "Barbearia retirada do ar. A assinatura e os agendamentos foram mantidos."
          : "A situação atual é diferente da ação solicitada. Confira antes de continuar; nada foi reenviado.",
      );
    } catch (failure) {
      if (generation.current !== attempt) return;
      setStatus(null);
      setError(errorMessage(failure));
      setMode(accessFailure(failure) ? "blocked" : "uncertain");
    } finally {
      if (generation.current === attempt) busy.current = false;
    }
  }

  return {
    mode,
    status,
    error,
    notice,
    refresh,
    setRequested,
    pending: mode === "saving" || mode === "uncertain" || (mode === "loading" && uncertain.current),
    canShare: mode === "ready" && !!status?.public_booking_enabled && hasCurrentCoverage(status),
  };
}

function accessFailure(failure: unknown) {
  return failure instanceof ApiError && [401, 403, 404].includes(failure.status);
}
