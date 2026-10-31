"use client";
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { X } from "lucide-react";
import { feedbackIcons, type FeedbackTone } from "./ui/alert";
import styles from "./feedback-provider.module.css";

type Notice = { text: string; tone: FeedbackTone };
type Notify = (text: string, options?: { tone?: FeedbackTone }) => void;
const FeedbackContext = createContext<Notify | null>(null);
export function FeedbackProvider({ children }: { children: React.ReactNode }) {
  const [notice, setNotice] = useState<Notice | null>(null);
  const [paused, setPaused] = useState(false);
  const notify = useCallback<Notify>((text, options) => {
    setNotice({ text, tone: options?.tone ?? "success" });
  }, []);
  useEffect(() => {
    if (!notice || paused || notice.tone === "error" || notice.tone === "warning") return;
    const timer = setTimeout(() => setNotice(null), 6000);
    return () => clearTimeout(timer);
  }, [notice, paused]);
  const Icon = feedbackIcons[notice?.tone ?? "info"];
  return (
    <FeedbackContext.Provider value={notify}>
      {children}
      <div
        role={notice ? (notice.tone === "error" ? "alert" : "status") : undefined}
        aria-live={notice?.tone === "error" ? "assertive" : "polite"}
        aria-atomic="true"
      >
        {notice && (
          <div
            className={`${styles.toast} ${styles[notice.tone]}`}
            onMouseEnter={() => setPaused(true)}
            onMouseLeave={() => setPaused(false)}
            onFocusCapture={() => setPaused(true)}
            onBlurCapture={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget as Node | null))
                setPaused(false);
            }}
          >
            <Icon size={20} aria-hidden="true" />
            <span>{notice.text}</span>
            <button
              type="button"
              className="icon-button"
              aria-label="Fechar aviso"
              onClick={() => {
                setNotice(null);
                setPaused(false);
              }}
            >
              <X size={18} aria-hidden="true" />
            </button>
          </div>
        )}
      </div>
    </FeedbackContext.Provider>
  );
}
export function useFeedback() {
  const notify = useContext(FeedbackContext);
  if (!notify) throw new Error("FeedbackProvider is required");
  return { notify };
}
