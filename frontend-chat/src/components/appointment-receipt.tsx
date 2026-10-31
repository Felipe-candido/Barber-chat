"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Check, Copy, RefreshCw } from "lucide-react";
import {
  type Appointment,
  type AppointmentDetails,
  bookingDate,
  bookingTime,
  cancelAppointment,
  lookupAppointment,
} from "@/lib/api/booking";
import { errorMessage } from "@/lib/api/client";
import { money } from "@/lib/model";
import { Alert, Badge, Button } from "./ui";
import styles from "./booking/booking.module.css";

const statusNames: Record<Appointment["status"], string> = {
  scheduled: "Reservado",
  confirmed: "Confirmado",
  cancelled: "Cancelado",
  completed: "Concluído",
  no_show: "Não compareceu",
};

export function AppointmentReceipt({
  slug,
  code,
  initial,
  onShopName,
  onLockedChange,
}: {
  slug: string;
  code: string;
  initial?: Appointment;
  onShopName?: (name: string) => void;
  onLockedChange?: (locked: boolean) => void;
}) {
  const [details, setDetails] = useState<AppointmentDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const cancellingRef = useRef(false);
  const request = useRef<AbortController | null>(null);
  const codeInput = useRef<HTMLTextAreaElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const cancelHeading = useRef<HTMLHeadingElement>(null);
  const cancelTrigger = useRef<HTMLButtonElement>(null);
  const codeId = useId();
  const reload = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    setConfirmCancel(false);
    try {
      const result = await lookupAppointment(slug, code, controller.signal);
      if (!controller.signal.aborted) setDetails(result);
    } catch (cause) {
      if (!controller.signal.aborted) setError(errorMessage(cause));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [slug, code]);
  useEffect(() => {
    void reload();
    return () => request.current?.abort();
  }, [reload]);
  useEffect(() => {
    heading.current?.focus();
  }, []);
  useEffect(() => {
    if (confirmCancel) cancelHeading.current?.focus();
  }, [confirmCancel]);
  const appointment = details?.appointment ?? initial;
  useEffect(() => {
    if (appointment) onShopName?.(appointment.shop_name);
  }, [appointment, onShopName]);
  useEffect(() => {
    onLockedChange?.(cancelling);
    return () => onLockedChange?.(false);
  }, [cancelling, onLockedChange]);
  useEffect(() => {
    if (!cancelling) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [cancelling]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied("Código copiado. Guarde-o para consultar esta reserva.");
    } catch {
      codeInput.current?.focus();
      codeInput.current?.select();
      setCopied("Selecione o código e copie com Ctrl+C ou pelo menu do celular.");
    }
  }
  async function cancel() {
    if (cancellingRef.current || loading || error || !details?.can_cancel) return;
    cancellingRef.current = true;
    setCancelling(true);
    setError("");
    try {
      setDetails(await cancelAppointment(slug, code));
      setConfirmCancel(false);
      heading.current?.focus();
    } catch (cause) {
      setError(errorMessage(cause));
      setConfirmCancel(false);
    } finally {
      cancellingRef.current = false;
      setCancelling(false);
    }
  }
  const canCancel = details?.can_cancel && !loading && !error;
  return (
    <section className={styles.receipt} aria-label="Comprovante do agendamento">
      <h2 ref={heading} tabIndex={-1}>
        {initial && <Check size={22} aria-hidden="true" />}
        {initial ? "Agendamento registrado!" : "Informações do agendamento"}
      </h2>
      {loading && (
        <p className={styles.loading} role="status">
          Conferindo a situação atual da reserva…
        </p>
      )}
      {error && (
        <Alert tone="error">
          {error}
          <p>Atualize a situação antes de fazer outra operação.</p>
        </Alert>
      )}
      {appointment && (
        <>
          <div className={styles.selection}>
            <h3>{appointment.shop_name}</h3>
            <Badge
              tone={
                appointment.status === "cancelled" || appointment.status === "no_show"
                  ? "neutral"
                  : "success"
              }
            >
              {statusNames[appointment.status]}
            </Badge>
          </div>
          <div className={styles.codeBox}>
            <label htmlFor={codeId}>Código do agendamento</label>
            <textarea
              id={codeId}
              ref={codeInput}
              className={styles.code}
              rows={2}
              value={code}
              readOnly
              onFocus={(event) => event.currentTarget.select()}
              spellCheck={false}
              aria-describedby={`${codeId}-hint`}
            />
            <Button onClick={() => void copy()}>
              <Copy size={16} aria-hidden="true" />
              Copiar código
            </Button>
            {copied && (
              <p role="status" className={styles.hint}>
                {copied}
              </p>
            )}
            <p id={`${codeId}-hint`} className={styles.hint}>
              Guarde este código e o link da barbearia. Quem tiver o código poderá consultar e
              cancelar somente esta reserva. Não compartilhe publicamente.
            </p>
          </div>
          <dl className={styles.details}>
            <div>
              <dt>
                Situação{!details ? " no registro" : loading || error ? " na última consulta" : ""}
              </dt>
              <dd>{statusNames[appointment.status]}</dd>
            </div>
            <div>
              <dt>Serviço</dt>
              <dd>{appointment.service_name}</dd>
            </div>
            <div>
              <dt>Profissional</dt>
              <dd>{appointment.professional_name}</dd>
            </div>
            <div>
              <dt>Data</dt>
              <dd>{bookingDate(appointment.starts_at, appointment.shop_timezone)}</dd>
            </div>
            <div>
              <dt>Início</dt>
              <dd>{bookingTime(appointment.starts_at, appointment.shop_timezone)}</dd>
            </div>
            <div>
              <dt>Agenda reservada até</dt>
              <dd>{bookingTime(appointment.ends_at, appointment.shop_timezone)}</dd>
            </div>
            <div>
              <dt>Duração estimada do serviço</dt>
              <dd>{appointment.duration_minutes} minutos</dd>
            </div>
            <div>
              <dt>Valor</dt>
              <dd>{money(appointment.price_cents)}</dd>
            </div>
          </dl>
          <p className={styles.hint}>
            Horários da barbearia: {appointment.shop_timezone}. O intervalo reservado pode incluir
            margem entre atendimentos. Nenhuma mensagem será enviada pelo WhatsApp nesta etapa.
          </p>
        </>
      )}
      <div className={styles.actions}>
        <Button variant="secondary" disabled={loading || cancelling} onClick={() => void reload()}>
          <RefreshCw size={16} aria-hidden="true" />
          Atualizar situação
        </Button>
        {canCancel && !confirmCancel && (
          <Button
            ref={cancelTrigger}
            variant="quiet"
            disabled={cancelling}
            onClick={() => setConfirmCancel(true)}
          >
            Cancelar agendamento
          </Button>
        )}
      </div>
      {confirmCancel && canCancel && (
        <section className={styles.cancelConfirm} aria-label="Confirmar cancelamento">
          <h3 tabIndex={-1} ref={cancelHeading}>
            Cancelar este agendamento?
          </h3>
          <p className={styles.hint}>
            Seu horário será liberado para outra pessoa. Esta ação não pode ser desfeita.
          </p>
          <div className={styles.actions}>
            <Button variant="danger" loading={cancelling} onClick={() => void cancel()}>
              {cancelling ? "Cancelando…" : "Sim, cancelar agendamento"}
            </Button>
            <Button
              variant="secondary"
              disabled={cancelling}
              onClick={() => {
                setConfirmCancel(false);
                requestAnimationFrame(() => cancelTrigger.current?.focus());
              }}
            >
              Manter agendamento
            </Button>
          </div>
        </section>
      )}
      {details?.appointment.status === "cancelled" && (
        <Alert tone="success">Agendamento cancelado. O horário foi liberado.</Alert>
      )}
      {details && !details.can_cancel && details.appointment.status !== "cancelled" && (
        <p className={styles.hint}>
          O cancelamento não está disponível para esta reserva. Fale com a barbearia se precisar de
          ajuda.
        </p>
      )}
    </section>
  );
}
