"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { MessageCircle, RefreshCw } from "lucide-react";
import { ApiError, errorMessage } from "@/lib/api/client";
import {
  connectWhatsApp,
  disconnectWhatsApp,
  getNotificationSettings,
  getWhatsAppConnection,
  saveNotificationSettings,
  type NotificationSettings,
  type PairingQR,
  type WhatsAppConnection,
} from "@/lib/api/notifications";
import { useUnsavedSettings } from "@/hooks/use-unsaved-settings";
import { Alert, Button, SelectField, TextField } from "../ui";
import styles from "./shop-settings.module.css";
import local from "./whatsapp-settings.module.css";

const labels: Record<WhatsAppConnection["state"], string> = {
  not_configured: "Ainda não conectado",
  connecting: "Conectando",
  qr_required: "Aguardando leitura do QR",
  connected: "Conectado",
  disconnected: "Desconectado",
  unknown: "Conexão não confirmada",
};
export function WhatsAppSettings({
  slug,
  shopID,
  userID,
}: {
  slug: string;
  shopID: string;
  userID: string;
}) {
  const scope = useMemo(() => ({ slug, shopID, userID }), [slug, shopID, userID]);
  const [connection, setConnection] = useState<WhatsAppConnection | null>(null),
    [settings, setSettings] = useState<NotificationSettings | null>(null),
    [draft, setDraft] = useState<NotificationSettings | null>(null),
    [qr, setQR] = useState<PairingQR | null>(null);
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [uncertain, setUncertain] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const controller = useRef<AbortController | null>(null),
    inFlight = useRef(false),
    alive = useRef(false);
  const dirty =
    !!draft &&
    !!settings &&
    (draft.enabled !== settings.enabled ||
      draft.delivery_mode !== settings.delivery_mode ||
      draft.reminder_lead_minutes !== settings.reminder_lead_minutes);
  useUnsavedSettings(dirty, busy || uncertain);
  const refresh = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    const c = new AbortController();
    controller.current?.abort();
    controller.current = c;
    setLoading(true);
    setError("");
    setQR(null);
    try {
      const [status, policy] = await Promise.all([
        getWhatsAppConnection(scope, c.signal),
        getNotificationSettings(scope, c.signal),
      ]);
      if (c.signal.aborted) return;
      setConnection(status);
      setSettings(policy);
      setDraft(policy);
      setUncertain(false);
    } catch (cause) {
      if (!c.signal.aborted) {
        setError(errorMessage(cause));
        setConnection(null);
        setSettings(null);
        setDraft(null);
      }
    } finally {
      if (!c.signal.aborted) {
        setLoading(false);
        inFlight.current = false;
      }
    }
  }, [scope]);
  useEffect(() => {
    alive.current = true;
    inFlight.current = false;
    void refresh();
    return () => {
      alive.current = false;
      controller.current?.abort();
    };
  }, [refresh]);
  useEffect(() => {
    if (!qr) return;
    const timeout = window.setTimeout(
      () => {
        setQR(null);
        setNotice("O QR expirou. Gere outro para continuar o pareamento.");
      },
      Math.max(0, Math.min(20000, Date.parse(qr.discard_after) - Date.now())),
    );
    const hide = () => {
      if (document.hidden) setQR(null);
    };
    document.addEventListener("visibilitychange", hide);
    return () => {
      window.clearTimeout(timeout);
      document.removeEventListener("visibilitychange", hide);
    };
  }, [qr]);
  useEffect(() => {
    if (!connection || busy || loading || uncertain) return;
    const c = new AbortController();
    let polling = false;
    const timer = window.setInterval(
      () => {
        if (document.hidden || polling) return;
        polling = true;
        void getWhatsAppConnection(scope, c.signal)
          .then((next) => {
            if (!c.signal.aborted) {
              setConnection(next);
              if (next.state === "connected") setQR(null);
            }
          })
          .catch((cause) => {
            if (!c.signal.aborted) {
              setQR(null);
              setConnection(null);
              setError(errorMessage(cause));
            }
          })
          .finally(() => {
            polling = false;
          });
      },
      qr || connection.state === "connecting" || connection.state === "qr_required" ? 5000 : 30000,
    );
    return () => {
      c.abort();
      window.clearInterval(timer);
    };
  }, [scope, connection, qr, busy, loading, uncertain]);
  async function command(action: "connect" | "disconnect" | "save") {
    if (inFlight.current || uncertain || !settings || !draft) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    setQR(null);
    const c = new AbortController();
    controller.current?.abort();
    controller.current = c;
    try {
      if (action === "connect") {
        const next = await connectWhatsApp(scope, c.signal);
        if (c.signal.aborted) return;
        setConnection(next.connection);
        setQR(next.qr);
        setNotice(
          next.qr
            ? "Abra o WhatsApp no celular e escolha Aparelhos conectados → Conectar aparelho."
            : next.connection.state === "connected"
              ? "Conexão confirmada."
              : "Solicitação iniciada. Consulte o estado e gere o QR novamente se necessário.",
        );
      } else if (action === "disconnect") {
        const next = await disconnectWhatsApp(scope, c.signal);
        if (c.signal.aborted) return;
        setConnection(next);
        setNotice("Solicitação de desconexão registrada.");
      } else {
        const next = await saveNotificationSettings(scope, draft, c.signal);
        if (c.signal.aborted) return;
        setSettings(next);
        setDraft(next);
        setNotice("Preferências de WhatsApp salvas. A política vale para novos agendamentos.");
      }
    } catch (cause) {
      if (alive.current && !c.signal.aborted) {
        setError(errorMessage(cause));
        setQR(null);
        setUncertain(
          !(
            cause instanceof ApiError &&
            cause.status >= 400 &&
            cause.status < 500 &&
            cause.code !== "connection_busy"
          ),
        );
      }
    } finally {
      if (alive.current && !c.signal.aborted) {
        setBusy(false);
        inFlight.current = false;
      }
    }
  }
  return (
    <section className={styles.section} aria-labelledby="whatsapp-title">
      <div className={local.heading}>
        <MessageCircle size={22} aria-hidden="true" />
        <h2 id="whatsapp-title">WhatsApp da barbearia</h2>
      </div>
      <p>Um número para confirmações e lembretes de todos os profissionais da unidade.</p>
      {loading && <p role="status">Consultando conexão e preferências…</p>}
      {error && <Alert tone="error">{error}</Alert>}
      {notice && <p role="status">{notice}</p>}
      {uncertain && (
        <Alert tone="warning">
          A ação pode ter sido concluída. Confira o estado antes de solicitar outra operação.
        </Alert>
      )}
      <div className={local.stack}>
        {connection && (
          <>
            <div
              className={local.status}
              data-connected={connection.state === "connected" && !connection.stale}
            >
              <span aria-hidden="true" />
              {labels[connection.state]}
              {connection.stale && connection.state !== "not_configured"
                ? " · verificação desatualizada"
                : ""}
            </div>
            <p className={local.muted}>
              {connection.verified_at
                ? "Última verificação: " + new Date(connection.verified_at).toLocaleString("pt-BR")
                : "Ainda sem verificação de conexão."}
            </p>
            {!connection.provider_configured && (
              <Alert tone="warning">
                A integração de WhatsApp ainda precisa ser habilitada pelo responsável pela
                plataforma.
              </Alert>
            )}
          </>
        )}
        <div className={styles.actions}>
          <Button
            variant="secondary"
            disabled={busy || loading || uncertain || !connection?.provider_configured}
            onClick={() => void command("connect")}
          >
            {qr
              ? "Gerar outro QR"
              : connection?.state === "connected"
                ? "Atualizar conexão"
                : "Conectar WhatsApp"}
          </Button>
          <Button
            variant="quiet"
            disabled={busy || loading || uncertain || !connection?.desired_connected}
            onClick={() => {
              if (
                window.confirm(
                  "Desconectar o WhatsApp desta barbearia? Os próximos envios ficarão indisponíveis até reconectar.",
                )
              )
                void command("disconnect");
            }}
          >
            Desconectar
          </Button>
          <Button
            variant="quiet"
            disabled={busy || loading}
            onClick={() => {
              if (
                !dirty ||
                window.confirm("Consultar novamente e descartar preferências não salvas?")
              )
                void refresh();
            }}
          >
            <RefreshCw size={16} aria-hidden="true" />
            Conferir estado
          </Button>
        </div>
        {qr && (
          <div className={local.qr}>
            <img
              src={qr.png}
              width={256}
              height={256}
              alt="QR code para conectar o WhatsApp da barbearia"
            />
            <p>Escaneie com o WhatsApp. O QR é temporário e aparece somente para o proprietário.</p>
            <Button variant="quiet" onClick={() => setQR(null)}>
              Ocultar QR
            </Button>
          </div>
        )}
        {draft && settings && (
          <form
            className={local.stack}
            onSubmit={(event) => {
              event.preventDefault();
              void command("save");
            }}
          >
            <label className={local.check}>
              <input
                type="checkbox"
                checked={draft.enabled}
                disabled={busy || uncertain || !draft.delivery_available}
                onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
              />
              <span>
                Enviar confirmação e disponibilizar lembrete aos clientes que autorizarem.
              </span>
            </label>
            {!draft.delivery_available && (
              <p className={local.muted}>Os envios ainda não estão habilitados neste ambiente.</p>
            )}
            <SelectField
              label="Forma de enviar o lembrete"
              value={draft.delivery_mode}
              disabled={busy || uncertain}
              onChange={(e) =>
                setDraft({ ...draft, delivery_mode: e.target.value as "request" | "automatic" })
              }
            >
              <option value="request">Manual, pelo botão na agenda</option>
              <option value="automatic" disabled={!draft.automatic_available}>
                Automático, antes do atendimento
              </option>
            </SelectField>
            {draft.delivery_mode === "automatic" && (
              <TextField
                label="Antecedência do lembrete (minutos)"
                type="number"
                min={5}
                max={10080}
                required
                value={draft.reminder_lead_minutes}
                disabled={busy || uncertain}
                onChange={(e) =>
                  setDraft({ ...draft, reminder_lead_minutes: Number(e.target.value) })
                }
                hint="1440 minutos = 24 horas. De 5 minutos a 7 dias."
              />
            )}
            <p className={local.muted}>
              O modo e a antecedência valem para novos agendamentos. Desativar os envios bloqueia
              também as mensagens pendentes. Agendamentos sem autorização continuam funcionando e
              exibindo o código na tela.
            </p>
            <Button type="submit" disabled={!dirty || busy || uncertain} loading={busy}>
              Salvar preferências de WhatsApp
            </Button>
          </form>
        )}
      </div>
    </section>
  );
}
