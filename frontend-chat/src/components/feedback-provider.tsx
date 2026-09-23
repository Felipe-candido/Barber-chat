"use client";
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { Check, X } from "lucide-react";

const FeedbackContext = createContext<((text: string) => void) | null>(null);
export function FeedbackProvider({ children }: { children: React.ReactNode }) {
  const [message, setMessage] = useState("");
  const notify = useCallback((text: string) => setMessage(text), []);
  useEffect(() => {
    if (!message) return;
    const timer = setTimeout(() => setMessage(""), 6000);
    return () => clearTimeout(timer);
  }, [message]);
  return (
    <FeedbackContext.Provider value={notify}>
      {children}
      {message && (
        <div className="toast" role="status">
          <span className="toast-icon">
            <Check size={17} />
          </span>
          {message}
          <button aria-label="Fechar aviso" onClick={() => setMessage("")}>
            <X size={16} />
          </button>
        </div>
      )}
    </FeedbackContext.Provider>
  );
}
export function useFeedback() {
  const notify = useContext(FeedbackContext);
  if (!notify) throw new Error("FeedbackProvider is required");
  return { notify };
}
