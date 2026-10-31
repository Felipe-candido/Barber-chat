"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  getShopSettings,
  updateShopSettings,
  type SettingsDraft,
  type ShopSettings,
} from "@/lib/api/shop-settings";
import {
  canonicalSettings,
  mergeSettings,
  sameSettings,
  settingsDraft,
  validateSettingsDraft,
} from "@/lib/shop-settings";

type Mode =
  | "loading"
  | "load-error"
  | "ready"
  | "saving"
  | "conflict"
  | "uncertain"
  | "checking"
  | "review"
  | "blocked";
type Review = { current: ShopSettings; proposed: SettingsDraft };

export function useShopSettings(
  slug: string,
  shopID: string,
  userID: string,
  syncShopName: (shopID: string, name: string, userID: string) => void,
) {
  const [mode, setMode] = useState<Mode>("loading");
  const [settings, setSettings] = useState<ShopSettings | null>(null);
  const [draft, setDraft] = useState<SettingsDraft | null>(null);
  const [review, setReview] = useState<Review | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState("");
  const generation = useRef(0);
  const request = useRef<AbortController | null>(null);
  const inFlight = useRef(false);
  const pending = useRef<{ base: ShopSettings; sent: SettingsDraft } | null>(null);

  const acceptCurrent = useCallback(
    (current: ShopSettings) => {
      setSettings(current);
      setDraft(settingsDraft(current));
      setReview(null);
      setError(null);
      pending.current = null;
      setMode("ready");
      syncShopName(shopID, current.name, userID);
    },
    [shopID, userID, syncShopName],
  );

  const reload = useCallback(async () => {
    const attempt = ++generation.current;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    inFlight.current = true;
    setMode("loading");
    setError(null);
    setNotice("");
    setSettings(null);
    setDraft(null);
    setReview(null);
    pending.current = null;
    try {
      const current = await getShopSettings(slug, shopID, userID, controller.signal);
      if (generation.current === attempt) acceptCurrent(current);
    } catch (error) {
      if (generation.current === attempt) {
        const failure = apiError(error);
        setError(failure);
        setMode(accessFailure(failure) ? "blocked" : "load-error");
      }
    } finally {
      if (generation.current === attempt) inFlight.current = false;
    }
  }, [slug, shopID, userID, acceptCurrent]);

  useEffect(() => {
    void reload();
    return () => {
      generation.current++;
      request.current?.abort();
      inFlight.current = false;
    };
  }, [reload]);

  async function save() {
    if (
      inFlight.current ||
      mode !== "ready" ||
      !settings ||
      !draft ||
      Object.keys(validateSettingsDraft(draft)).length
    )
      return;
    const sent = canonicalSettings(draft);
    pending.current = { base: settings, sent };
    const attempt = ++generation.current;
    const controller = new AbortController();
    request.current = controller;
    inFlight.current = true;
    setMode("saving");
    setError(null);
    setNotice("");
    try {
      const current = await updateShopSettings(
        slug,
        shopID,
        userID,
        { ...sent, version: settings.version },
        controller.signal,
      );
      if (generation.current === attempt) {
        acceptCurrent(current);
        setNotice("Configurações salvas.");
      }
    } catch (error) {
      if (generation.current !== attempt) return;
      const failure = apiError(error);
      setError(failure);
      if (accessFailure(failure)) setMode("blocked");
      else if (failure.status === 409) setMode("conflict");
      else if ([400, 413, 415, 422].includes(failure.status)) {
        setMode("ready");
        pending.current = null;
      } else setMode("uncertain");
    } finally {
      if (generation.current === attempt) inFlight.current = false;
    }
  }

  async function checkCurrent() {
    const previous = pending.current;
    if (inFlight.current || !previous || (mode !== "uncertain" && mode !== "conflict")) return;
    const fallback = mode;
    const attempt = ++generation.current;
    const controller = new AbortController();
    request.current = controller;
    inFlight.current = true;
    setMode("checking");
    setError(null);
    try {
      const current = await getShopSettings(slug, shopID, userID, controller.signal);
      if (generation.current !== attempt) return;
      syncShopName(shopID, current.name, userID);
      if (
        current.version > previous.base.version &&
        sameSettings(settingsDraft(current), previous.sent)
      ) {
        acceptCurrent(current);
        setNotice(
          "Dados conferidos na API. O estado atual corresponde ao que você enviou; nada foi reenviado.",
        );
      } else {
        setReview({
          current,
          proposed: mergeSettings(
            settingsDraft(previous.base),
            settingsDraft(current),
            previous.sent,
          ),
        });
        setMode("review");
      }
    } catch (error) {
      if (generation.current === attempt) {
        const failure = apiError(error);
        setError(failure);
        setMode(accessFailure(failure) ? "blocked" : fallback);
      }
    } finally {
      if (generation.current === attempt) inFlight.current = false;
    }
  }

  function useReview(keepChanges: boolean) {
    if (!review || mode !== "review") return;
    const chosen = review;
    acceptCurrent(chosen.current);
    if (keepChanges) setDraft(chosen.proposed);
    setNotice(
      keepChanges
        ? "Rascunho preparado com a versão atual. Revise o formulário e salve quando estiver pronto. Nada foi reenviado."
        : "Dados atuais da API carregados. Seu rascunho foi descartado; nada foi reenviado.",
    );
  }

  function reset() {
    if (settings && mode === "ready") {
      setDraft(settingsDraft(settings));
      setError(null);
      setNotice("Alterações não salvas descartadas.");
    }
  }

  return {
    mode,
    settings,
    draft,
    review,
    error,
    notice,
    reload,
    save,
    checkCurrent,
    useReview,
    reset,
    setDraft: (value: SettingsDraft) => {
      if (mode === "ready") {
        setDraft(value);
        setNotice("");
      }
    },
    dirty: !!settings && !!draft && !sameSettings(settingsDraft(settings), draft),
  };
}

function apiError(error: unknown) {
  return error instanceof ApiError ? error : new ApiError("settings_failed", errorMessage(error));
}
function accessFailure(error: ApiError) {
  return [401, 403, 404].includes(error.status);
}
