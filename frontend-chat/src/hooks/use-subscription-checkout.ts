"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  getSubscriptionOffer,
  startSubscriptionCheckout,
  type SubscriptionOffer,
  type SubscriptionCheckout,
} from "@/lib/api/subscription-checkout";

type Mode = "closed" | "loading" | "review" | "opening" | "ready" | "error";
export function useSubscriptionCheckout(
  slug: string,
  shopID: string,
  userID: string,
  intent: SubscriptionCheckout["intent"] = "publish",
) {
  const [mode, setMode] = useState<Mode>("closed");
  const [offer, setOffer] = useState<SubscriptionOffer | null>(null);
  const [checkout, setCheckout] = useState<SubscriptionCheckout | null>(null);
  const [error, setError] = useState("");
  const [uncertain, setUncertain] = useState(false);
  const busy = useRef(false);
  const generation = useRef(0);
  const abort = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      generation.current++;
      abort.current?.abort();
      busy.current = false;
    },
    [slug, shopID, userID, intent],
  );
  useEffect(() => {
    if (mode !== "ready" || !checkout) return;
    const remaining = Date.parse(checkout.expires_at) - Date.now() - 30_000;
    const timer = setTimeout(
      () => {
        setCheckout(null);
        setError(
          "O endereço de pagamento expirou. Consulte as condições novamente para conferir a tentativa na API.",
        );
        setMode("error");
      },
      Math.max(0, remaining),
    );
    return () => clearTimeout(timer);
  }, [mode, checkout]);
  async function load() {
    if (busy.current) return;
    busy.current = true;
    const attempt = ++generation.current;
    const controller = new AbortController();
    abort.current = controller;
    setMode("loading");
    setOffer(null);
    setCheckout(null);
    setError("");
    try {
      const current = await getSubscriptionOffer(slug, shopID, userID, controller.signal);
      if (generation.current !== attempt) return;
      setOffer(current);
      setMode("review");
    } catch (failure) {
      if (generation.current !== attempt) return;
      setError(errorMessage(failure));
      setMode("error");
    } finally {
      if (generation.current === attempt) busy.current = false;
    }
  }
  async function confirm() {
    if (busy.current || mode !== "review" || !offer) return;
    busy.current = true;
    const attempt = ++generation.current;
    const controller = new AbortController();
    abort.current = controller;
    setMode("opening");
    setError("");
    try {
      const result = await startSubscriptionCheckout(
        slug,
        userID,
        offer.offer_id,
        controller.signal,
        intent,
      );
      if (generation.current !== attempt) return;
      setCheckout(result);
      setUncertain(false);
      setMode("ready");
    } catch (failure) {
      if (generation.current !== attempt) return;
      const definite =
        failure instanceof ApiError && [400, 401, 403, 409, 413, 415, 422].includes(failure.status);
      setUncertain(!definite);
      setCheckout(null);
      setError(errorMessage(failure));
      setMode("error");
    } finally {
      if (generation.current === attempt) busy.current = false;
    }
  }
  function close() {
    if (busy.current) return;
    setMode("closed");
    setOffer(null);
    setCheckout(null);
    setError("");
  }
  return {
    mode,
    offer,
    checkout,
    error,
    uncertain,
    load,
    confirm,
    close,
    pending: mode === "opening",
  };
}
