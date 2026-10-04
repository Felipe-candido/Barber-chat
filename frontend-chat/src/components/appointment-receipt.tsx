"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import {
  Appointment,
  AppointmentDetails,
  bookingDate,
  bookingTime,
  cancelAppointment,
  lookupAppointment,
} from "@/lib/api/booking";
import { errorMessage } from "@/lib/api/client";
import { money } from "@/lib/model";

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
}: {
  slug: string;
  code: string;
  initial?: Appointment;
}) {
  const [details, setDetails] = useState<AppointmentDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const request = useRef<AbortController | null>(null);
  const codeInput = useRef<HTMLInputElement>(null);
  const reload = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    try {
      const result = await lookupAppointment(slug, code, controller.signal);
      if (!controller.signal.aborted) setDetails(result);
    } catch (error) {
      if (!controller.signal.aborted) setError(errorMessage(error));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [slug, code]);
  useEffect(() => {
    void reload();
    return () => request.current?.abort();
  }, [reload]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied("Código copiado. Guarde-o para consultar esta reserva.");
    } catch {
      codeInput.current?.select();
      setCopied("Selecione o código e copie com Ctrl+C ou pelo menu do celular.");
    }
  }
  async function cancel() {
    if (cancelling) return;
    setCancelling(true);
    setError("");
    try {
      setDetails(await cancelAppointment(slug, code));
      setConfirmCancel(false);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setCancelling(false);
    }
  }
  const appointment = details?.appointment ?? initial;
  return (
    <section className="booking-receipt" aria-label="Comprovante do agendamento">
      {initial && (
        <h3>
          <Check size={22} /> Agendamento registrado!
        </h3>
      )}
      {loading && <p role="status">Conferindo a situação atual da reserva…</p>}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {appointment && (
        <>
          <h3>{appointment.shop_name}</h3>
          <dl className="receipt-details">
            <div>
              <dt>Situação{!details ? " no registro" : ""}</dt>
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
          <p className="booking-note">
            Horários da barbearia: {appointment.shop_timezone}. O intervalo reservado pode incluir
            margem entre atendimentos.
          </p>
          <label className="booking-field">
            Código do agendamento
            <input
              ref={codeInput}
              className="appointment-code"
              value={code}
              readOnly
              onFocus={(event) => event.currentTarget.select()}
              spellCheck={false}
            />
          </label>
          <button className="button button-primary" onClick={() => void copy()}>
            <Copy size={16} /> Copiar código
          </button>
          {copied && <p role="status">{copied}</p>}
          <p className="booking-note">
            Guarde este código e o link da barbearia. Quem tiver o código poderá consultar e
            cancelar somente esta reserva. Nenhuma mensagem será enviada pelo WhatsApp nesta etapa.
          </p>
        </>
      )}
      <div className="booking-actions">
        <button
          className="button button-secondary"
          disabled={loading || cancelling}
          onClick={() => void reload()}
        >
          Atualizar situação
        </button>
        {details?.can_cancel && !confirmCancel && (
          <button
            className="button button-secondary"
            disabled={loading || cancelling}
            onClick={() => setConfirmCancel(true)}
          >
            Cancelar agendamento
          </button>
        )}
      </div>
      {confirmCancel && details?.can_cancel && (
        <section className="cancellation-confirm" aria-label="Confirmar cancelamento">
          <p>Cancelar este agendamento? Seu horário será liberado para outra pessoa.</p>
          <div className="booking-actions">
            <button
              className="button button-primary"
              disabled={cancelling}
              onClick={() => void cancel()}
            >
              {cancelling ? "Cancelando…" : "Sim, cancelar agendamento"}
            </button>
            <button
              className="button button-secondary"
              disabled={cancelling}
              onClick={() => setConfirmCancel(false)}
            >
              Manter agendamento
            </button>
          </div>
        </section>
      )}
      {details?.appointment.status === "cancelled" && (
        <p role="status">Agendamento cancelado. O horário foi liberado.</p>
      )}
      {details && !details.can_cancel && details.appointment.status !== "cancelled" && (
        <p className="booking-note">
          O cancelamento não está disponível para esta reserva. Fale com a barbearia se precisar de
          ajuda.
        </p>
      )}
    </section>
  );
}

export function AppointmentLookup({ slug }: { slug: string }) {
  const [code, setCode] = useState("");
  const [submitted, setSubmitted] = useState<{ code: string; attempt: number } | null>(null);
  return (
    <section className="booking-lookup">
      <h3>Consultar meu agendamento</h3>
      <p>Use o código recebido ao concluir sua reserva nesta barbearia.</p>
      <form
        className="booking-form"
        onSubmit={(event) => {
          event.preventDefault();
          setSubmitted({ code: code.trim().toUpperCase(), attempt: (submitted?.attempt ?? 0) + 1 });
        }}
      >
        <label className="booking-field">
          Código recebido
          <input
            required
            maxLength={80}
            value={code}
            onChange={(event) => setCode(event.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </label>
        <button className="button button-primary">Consultar agendamento</button>
      </form>
      {submitted && (
        <AppointmentReceipt key={submitted.attempt} slug={slug} code={submitted.code} />
      )}
    </section>
  );
}
