"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";

type InstallPrompt = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
};
type InstallResult = "accepted" | "dismissed" | "unavailable";
type PwaState = {
  ready: boolean;
  installed: boolean;
  ios: boolean;
  installing: boolean;
  install: () => Promise<InstallResult>;
};
const PwaContext = createContext<PwaState | null>(null);

export function PwaProvider({ children }: { children: React.ReactNode }) {
  const [ready, setReady] = useState(false);
  const [installed, setInstalled] = useState(false);
  const [ios, setIos] = useState(false);
  const [installing, setInstalling] = useState(false);
  const pending = useRef<InstallPrompt | null>(null);
  const busy = useRef(false);

  useEffect(() => {
    const standalone = window.matchMedia("(display-mode: standalone)");
    const updateDisplay = () =>
      setInstalled(
        standalone.matches ||
          (navigator as Navigator & { standalone?: boolean }).standalone === true,
      );
    const capturePrompt = (event: Event) => {
      event.preventDefault();
      pending.current = event as InstallPrompt;
    };
    const onInstalled = () => {
      pending.current = null;
      setInstalled(true);
    };
    updateDisplay();
    setIos(
      /iPad|iPhone|iPod/.test(navigator.userAgent) ||
        (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1),
    );
    setReady(true);
    window.addEventListener("beforeinstallprompt", capturePrompt);
    window.addEventListener("appinstalled", onInstalled);
    standalone.addEventListener("change", updateDisplay);

    const register = () => {
      if (
        process.env.NODE_ENV !== "production" ||
        !window.isSecureContext ||
        !("serviceWorker" in navigator)
      )
        return;
      void navigator.serviceWorker
        .register("/sw.js", { scope: "/", updateViaCache: "none" })
        .catch(() => console.warn("Não foi possível preparar a tela sem conexão do app."));
    };
    if (document.readyState === "complete") register();
    else window.addEventListener("load", register, { once: true });
    return () => {
      window.removeEventListener("load", register);
      window.removeEventListener("beforeinstallprompt", capturePrompt);
      window.removeEventListener("appinstalled", onInstalled);
      standalone.removeEventListener("change", updateDisplay);
    };
  }, []);

  const install = useCallback(async (): Promise<InstallResult> => {
    if (busy.current) return "dismissed";
    const prompt = pending.current;
    if (!prompt) return "unavailable";
    pending.current = null; // A browser prompt can only be used once.
    busy.current = true;
    setInstalling(true);
    try {
      await prompt.prompt(); // Called directly from the user's click.
      return (await prompt.userChoice).outcome;
    } catch {
      return "unavailable";
    } finally {
      busy.current = false;
      setInstalling(false);
    }
  }, []);

  return (
    <PwaContext.Provider value={{ ready, installed, ios, installing, install }}>
      {children}
    </PwaContext.Provider>
  );
}

export function usePwa() {
  const context = useContext(PwaContext);
  if (!context) throw new Error("PWA controls require PwaProvider.");
  return context;
}
