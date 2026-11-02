"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";
import {
  bookingMessageLabel,
  getBookingMessages,
  sendBookingMessage,
  type BookingMessages,
  type BookingMessageStatus,
} from "@/lib/api/notifications";
import { useCommerceScope } from "../commerce/shared";
import { Alert, Button } from "../ui";
import styles from "./appointment-notifications.module.css";

export function AppointmentNotifications({ id, revision }: { id: string; revision: string }) {
  const scope = useCommerceScope();
  const [data, setData] = useState<BookingMessages | null>(null),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [uncertain, setUncertain] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const controller = useRef<AbortController | null>(null),
    inFlight = useRef(false);
  const refresh = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    const c = new AbortController();
    controller.current?.abort();
    controller.current = c;
    setLoading(true);
    setError("");
    try {
      const next = await getBookingMessages(scope, id, c.signal);
      if (!c.signal.aborted) {
        setData(next);
        setUncertain(false);
      }
    } catch (cause) {
      if (!c.signal.aborted) {
        setError(errorMessage(cause));
        setData(null);
      }
    } finally {
      if (!c.signal.aborted) {
        setLoading(false);
        inFlight.current = false;
      }
    }
  }, [scope, id]);
  useEffect(() => {
    inFlight.current = false;
    setData(null);
    setNotice("");
    void refresh();
    return () => controller.current?.abort();
  }, [refresh, revision]);
  async function send(kind: BookingMessageStatus["kind"]) {
    if (inFlight.current || uncertain) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    const c = new AbortController();
    controller.current?.abort();
    controller.current = c;
    try {
      const state = await sendBookingMessage(scope, id, kind, c.signal);
      if (c.signal.aborted) return;
      setNotice(bookingMessageLabel(state));
      const next = await getBookingMessages(scope, id, c.signal);
      if (!c.signal.aborted) setData(next);
    } catch (cause) {
      if (!c.signal.aborted) {
        setError(errorMessage(cause));
        setData(null);
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
    <section className={styles.panel} aria-label="Mensagens de WhatsApp do atendimento">
      <div className={styles.heading}>
        <h3>WhatsApp do agendamento</h3>
        <Button variant="quiet" disabled={busy || loading} onClick={() => void refresh()}>
          Conferir envios
        </Button>
      </div>
      {loading && <p role="status">Consultando mensagens…</p>}
      {error && <Alert tone="error">{error}</Alert>}
      {notice && <p role="status">{notice}</p>}
      {uncertain && (
        <Alert tone="warning">
          O envio pode ter sido aceito. Confira os envios antes de solicitar outra ação.
        </Alert>
      )}
      {data && !data.delivery_available && (
        <p>Os envios de WhatsApp estão indisponíveis neste ambiente.</p>
      )}
      {data && data.messages.length === 0 && (
        <p>
          Este agendamento não tem mensagens planejadas. O envio depende da autorização registrada
          na reserva.
        </p>
      )}
      {data?.messages.map((m) => (
        <div className={styles.row} key={m.kind}>
          <div>
            <strong>{m.kind === "booking_confirmation" ? "Confirmação" : "Lembrete"}</strong>
            <p>{bookingMessageLabel(m.state)}</p>
            {m.receipt === "delivered" && <p>Entrega confirmada pelo WhatsApp.</p>}
            {m.receipt === "read" && <p>Leitura confirmada pelo WhatsApp.</p>}
            {m.state === "unknown" && (
              <p>
                A resposta não comprova se a mensagem chegou. A repetição automática está bloqueada.
              </p>
            )}
            {m.state === "ready" && !m.can_send && (
              <p>
                {m.mode === "automatic"
                  ? "Aguardando a janela programada ou uma conexão confirmada."
                  : "Confira a conexão e as preferências de WhatsApp da barbearia."}
              </p>
            )}
          </div>
          {m.can_send && (
            <Button
              variant="secondary"
              disabled={busy || loading || uncertain}
              loading={busy}
              onClick={() => void send(m.kind)}
            >
              {m.kind === "booking_confirmation"
                ? "Enviar confirmação pendente"
                : "Enviar lembrete"}
            </Button>
          )}
        </div>
      ))}
      <p className={styles.note}>
        Envio aceito é diferente de confirmação de entrega ou leitura pelo cliente.
      </p>
    </section>
  );
}
