"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  getCompletionSettings,
  saveCompletionSettings,
  type CompletionMode,
  type CompletionSettings,
} from "@/lib/api/appointment-completion";
import { errorMessage } from "@/lib/api/client";
import { useUnsavedSettings } from "@/hooks/use-unsaved-settings";
import { Alert, Button, SelectField } from "../ui";
import styles from "./shop-settings.module.css";

export function AppointmentCompletionSettings({
  slug,
  shopID,
  userID,
}: {
  slug: string;
  shopID: string;
  userID: string;
}) {
  const scope = useMemo(() => ({ slug, shopID, userID }), [slug, shopID, userID]);
  const [settings, setSettings] = useState<CompletionSettings | null>(null);
  const [mode, setMode] = useState<CompletionMode>("automatic");
  const [busy, setBusy] = useState(false),
    [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const controller = useRef<AbortController | null>(null),
    inFlight = useRef(false);
  const dirty = !!settings && mode !== settings.mode;
  useUnsavedSettings(dirty, busy || uncertain);
  const load = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    const c = new AbortController();
    controller.current = c;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const current = await getCompletionSettings(scope, c.signal);
      if (!c.signal.aborted) {
        setSettings(current);
        setMode(current.mode);
        setUncertain(false);
      }
    } catch (cause) {
      if (!c.signal.aborted) {
        setError(errorMessage(cause));
        setSettings(null);
      }
    } finally {
      if (!c.signal.aborted) {
        setBusy(false);
        inFlight.current = false;
      }
    }
  }, [scope]);
  useEffect(() => {
    inFlight.current = false;
    setSettings(null);
    setUncertain(false);
    void load();
    return () => {
      controller.current?.abort();
    };
  }, [load]);
  async function save() {
    if (!settings || !dirty || inFlight.current || uncertain) return;
    inFlight.current = true;
    const c = new AbortController();
    controller.current = c;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const current = await saveCompletionSettings(scope, mode, settings.version, c.signal);
      if (!c.signal.aborted) {
        setSettings(current);
        setMode(current.mode);
        setNotice("Modo de fechamento salvo.");
      }
    } catch (cause) {
      if (!c.signal.aborted) {
        setError(errorMessage(cause));
        setUncertain(true);
      }
    } finally {
      if (!c.signal.aborted) {
        setBusy(false);
        inFlight.current = false;
      }
    }
  }
  return (
    <section className={styles.section} aria-labelledby="appointment-completion-title">
      <h2 id="appointment-completion-title">Fechamento dos atendimentos</h2>
      <p>
        Escolha como os atendimentos desta barbearia são finalizados. A alteração vale para toda a
        equipe.
      </p>
      <form
        className={styles.fields}
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
      >
        {error && <Alert tone="error">{error}</Alert>}
        {notice && (
          <div role="status">
            <Alert tone="success">{notice}</Alert>
          </div>
        )}
        {settings && (
          <>
            <SelectField
              label="Modo de fechamento"
              value={mode}
              disabled={busy || uncertain}
              onChange={(event) => {
                setMode(event.target.value as CompletionMode);
                setNotice("");
              }}
            >
              <option value="automatic">Automático, após o horário reservado</option>
              <option value="manual">Manual, pela equipe</option>
            </SelectField>
            <p>
              {mode === "manual"
                ? "Os atendimentos continuam em aberto após o horário. A equipe marca realizado, ausência ou cancelamento na agenda. O registro de produtos continua disponível."
                : "Os atendimentos são concluídos após o fim do horário reservado. Pendências anteriores à reativação do automático exigem fechamento manual. Ausências e cancelamentos podem ser corrigidos pela equipe."}
            </p>
            {mode === "automatic" && settings.mode === "manual" && (
              <Alert tone="warning">
                Ao ativar o automático, pendências de horários já encerrados continuam em aberto
                para revisão manual.
              </Alert>
            )}
          </>
        )}
        {uncertain && (
          <Alert tone="warning">
            O resultado pode ter sido salvo ou alterado por outra pessoa. Consulte o modo atual
            antes de fazer outra alteração.
          </Alert>
        )}
        <div className={styles.actions}>
          <Button type="submit" disabled={!dirty || busy || uncertain}>
            Salvar modo de fechamento
          </Button>
          <Button
            variant="secondary"
            disabled={busy}
            onClick={() => {
              if (
                !dirty ||
                uncertain ||
                window.confirm("Descartar a alteração não salva e consultar o modo atual?")
              )
                void load();
            }}
          >
            {busy ? "Consultando…" : "Consultar modo atual"}
          </Button>
        </div>
      </form>
    </section>
  );
}
