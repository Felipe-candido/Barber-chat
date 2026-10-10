"use client";

import { useEffect, useRef, useState } from "react";
import { AppointmentReceipt } from "./appointment-receipt";
import {
  Availability,
  BookingInput,
  BookingResult,
  Professionals,
  Slot,
  bookingDate,
  bookingTime,
  createAppointment,
  createStaffAppointment,
  listAvailability,
  listProfessionals,
  newBookingKey,
} from "@/lib/api/booking";
import { ApiError, errorMessage } from "@/lib/api/client";
import type { Service } from "@/lib/api/services";
import { money } from "@/lib/model";

export function BookingForm({
  slug,
  service,
  onBack,
  onLockedChange,
  staff,
}: {
  slug: string;
  service: Service;
  onBack: () => void;
  onLockedChange: (locked: boolean) => void;
  staff?: { professionalID: string | null; onCreated: () => void };
}) {
  const [professionals, setProfessionals] = useState<Professionals | null>(null);
  const [professionalId, setProfessionalId] = useState("");
  const [date, setDate] = useState("");
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [slot, setSlot] = useState<Slot | null>(null);
  const [customer, setCustomer] = useState({
    first_name: "",
    last_name: "",
    phone: "",
    phone_country: "BR",
  });
  const [step, setStep] = useState<"schedule" | "contact" | "review">("schedule");
  const [professionalsError, setProfessionalsError] = useState("");
  const [availabilityError, setAvailabilityError] = useState("");
  const [error, setError] = useState("");
  const [professionalsVersion, setProfessionalsVersion] = useState(0);
  const [availabilityVersion, setAvailabilityVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [result, setResult] = useState<BookingResult | null>(null);
  const attempt = useRef<{ key: string; input: BookingInput } | null>(null);
  const submitting = useRef(false);
  const locked = busy || uncertain;

  useEffect(() => {
    const controller = new AbortController();
    setProfessionals(null);
    setProfessionalsError("");
    void listProfessionals(slug, service.id, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) {
          const allowed = staff?.professionalID;
          const eligible = allowed
            ? data.professionals.filter((p) => p.id === allowed)
            : data.professionals;
          setProfessionals({ ...data, professionals: eligible });
          if (allowed) setProfessionalId(eligible.length ? allowed : "");
          setDate(data.today);
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setProfessionalsError(errorMessage(error));
      });
    return () => controller.abort();
  }, [slug, service.id, professionalsVersion, staff?.professionalID]);

  useEffect(() => {
    setSlot(null);
    setAvailability(null);
    setAvailabilityError("");
    if (!professionalId || !date || !professionals) return;
    const controller = new AbortController();
    void listAvailability(slug, service.id, professionalId, date, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setAvailability(data);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setAvailabilityError(errorMessage(error));
      });
    return () => controller.abort();
  }, [slug, service.id, professionalId, date, professionals, availabilityVersion]);

  useEffect(() => {
    onLockedChange(locked);
    return () => onLockedChange(false);
  }, [locked, onLockedChange]);
  useEffect(() => {
    if (!locked) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [locked]);

  async function confirm() {
    if (submitting.current || !slot || !professionalId) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    try {
      if (!attempt.current)
        attempt.current = {
          key: newBookingKey(),
          input: {
            service_id: service.id,
            professional_id: professionalId,
            starts_at: slot.starts_at,
            customer: {
              ...customer,
              first_name: customer.first_name.trim(),
              last_name: customer.last_name.trim(),
              phone: customer.phone.trim(),
            },
          },
        };
      const saved = await (staff ? createStaffAppointment : createAppointment)(
        slug,
        attempt.current.input,
        attempt.current.key,
      );
      setResult(saved);
      setUncertain(false);
      attempt.current = null;
      setCustomer({ first_name: "", last_name: "", phone: "", phone_country: "BR" });
      staff?.onCreated();
    } catch (error) {
      setError(errorMessage(error));
      const definitive =
        error instanceof ApiError &&
        error.status >= 400 &&
        error.status < 500 &&
        !["idempotency_conflict", "idempotency_expired"].includes(error.code);
      setUncertain(!definitive);
      if (definitive) attempt.current = null;
      if (error instanceof ApiError && error.code === "slot_unavailable") {
        setStep("schedule");
        setAvailabilityVersion((value) => value + 1);
      }
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  if (result)
    return <AppointmentReceipt slug={slug} code={result.code} initial={result.appointment} />;
  const professional = professionals?.professionals.find((item) => item.id === professionalId);
  const zone = availability?.shop_timezone ?? professionals?.shop_timezone;

  return (
    <section className="booking-form-flow" aria-label="Novo agendamento">
      <div className="booking-selection">
        <strong>{service.name}</strong>
        <span>
          {service.duration_minutes} min de serviço · {money(service.price_cents)}
        </span>
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {step === "schedule" && (
        <>
          <h3>Escolha profissional e horário</h3>
          {professionalsError ? (
            <>
              <p className="form-error" role="alert">
                {professionalsError}
              </p>
              <button
                className="button button-secondary"
                onClick={() => setProfessionalsVersion((value) => value + 1)}
              >
                Tentar carregar profissionais
              </button>
            </>
          ) : !professionals ? (
            <p role="status">Carregando profissionais…</p>
          ) : professionals.professionals.length === 0 ? (
            <p>
              Nenhum profissional habilitado para este serviço. A barbearia precisa configurar os
              profissionais e seus serviços no painel.
            </p>
          ) : (
            <>
              <label className="booking-field">
                Profissional
                <select
                  value={professionalId}
                  disabled={!!staff?.professionalID}
                  onChange={(event) => setProfessionalId(event.target.value)}
                >
                  <option value="">Selecione um profissional</option>
                  {professionals.professionals.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className="booking-field">
                Data do agendamento
                <input
                  type="date"
                  min={professionals.today}
                  max={professionals.last_booking_date}
                  value={date}
                  onChange={(event) => setDate(event.target.value)}
                />
              </label>
              <p className="booking-note">
                Horários de {professionals.shop_name} · {professionals.shop_timezone}
              </p>
              {professionalId && date && (
                <>
                  {availabilityError ? (
                    <p className="form-error" role="alert">
                      {availabilityError}
                    </p>
                  ) : !availability ? (
                    <p role="status">Buscando horários…</p>
                  ) : availability.slots.length === 0 ? (
                    <p>Nenhum horário livre nesta data. Escolha outro dia ou profissional.</p>
                  ) : (
                    <div className="time-options" aria-label="Horários disponíveis">
                      {availability.slots.map((item) => (
                        <button
                          key={item.starts_at}
                          onClick={() => {
                            setSlot(item);
                            setStep("contact");
                            setError("");
                          }}
                        >
                          {bookingTime(item.starts_at, availability.shop_timezone)}
                        </button>
                      ))}
                    </div>
                  )}
                  <button
                    className="button button-secondary"
                    onClick={() => setAvailabilityVersion((value) => value + 1)}
                  >
                    Atualizar horários
                  </button>
                </>
              )}
            </>
          )}
          <button className="chat-back" onClick={onBack}>
            Escolher outro serviço
          </button>
        </>
      )}
      {slot && zone && step === "contact" && (
        <>
          <h3>Seus dados para o agendamento</h3>
          <p>
            {professional?.name} · {bookingDate(slot.starts_at, zone)} às{" "}
            {bookingTime(slot.starts_at, zone)}
          </p>
          <form
            className="booking-form"
            onSubmit={(event) => {
              event.preventDefault();
              setStep("review");
            }}
          >
            <label className="booking-field">
              Nome
              <input
                required
                maxLength={100}
                autoComplete="given-name"
                value={customer.first_name}
                onChange={(event) => setCustomer({ ...customer, first_name: event.target.value })}
              />
            </label>
            <label className="booking-field">
              Sobrenome
              <input
                required
                maxLength={100}
                autoComplete="family-name"
                value={customer.last_name}
                onChange={(event) => setCustomer({ ...customer, last_name: event.target.value })}
              />
            </label>
            <label className="booking-field">
              País do telefone
              <select
                value={customer.phone_country}
                onChange={(event) =>
                  setCustomer({ ...customer, phone_country: event.target.value })
                }
              >
                <option value="BR">Brasil (+55)</option>
                <option value="PT">Portugal (+351)</option>
                <option value="US">Estados Unidos (+1)</option>
              </select>
            </label>
            <label className="booking-field">
              Telefone com DDD
              <input
                required
                type="tel"
                maxLength={40}
                autoComplete="tel"
                placeholder="(11) 91234-5678"
                value={customer.phone}
                onChange={(event) => setCustomer({ ...customer, phone: event.target.value })}
              />
            </label>
            <p className="booking-note">
              Se o país não estiver na lista, informe o telefone completo com + e o código do país.
              Seus dados serão enviados à barbearia somente ao confirmar. Não haverá envio de
              WhatsApp ou cadastro de conta.
            </p>
            <button className="button button-primary">Revisar agendamento</button>
          </form>
          <button
            className="chat-back"
            onClick={() => {
              setStep("schedule");
              setAvailabilityVersion((value) => value + 1);
            }}
          >
            Escolher outro horário
          </button>
        </>
      )}
      {slot && zone && step === "review" && (
        <>
          <h3>Confira antes de confirmar</h3>
          <dl className="receipt-details">
            <div>
              <dt>Profissional</dt>
              <dd>{professional?.name}</dd>
            </div>
            <div>
              <dt>Data</dt>
              <dd>{bookingDate(slot.starts_at, zone)}</dd>
            </div>
            <div>
              <dt>Horário reservado</dt>
              <dd>
                {bookingTime(slot.starts_at, zone)}–{bookingTime(slot.ends_at, zone)}
              </dd>
            </div>
            <div>
              <dt>Cliente</dt>
              <dd>
                {customer.first_name} {customer.last_name}
              </dd>
            </div>
            <div>
              <dt>Contato</dt>
              <dd>{customer.phone}</dd>
            </div>
          </dl>
          <p className="booking-note">
            O horário só fica reservado após a confirmação do servidor. Você receberá um código na
            tela para consultar e cancelar antes do início.
          </p>
          {uncertain && (
            <p className="booking-note">
              Mantenha esta página aberta e repita a confirmação. Os mesmos dados e a mesma chave
              serão usados, sem criar uma nova tentativa.
            </p>
          )}
          <button className="button button-primary" disabled={busy} onClick={() => void confirm()}>
            {busy ? "Confirmando…" : uncertain ? "Repetir confirmação" : "Confirmar agendamento"}
          </button>
          <button
            className="chat-back"
            disabled={locked}
            onClick={() => {
              setStep("contact");
              setError("");
            }}
          >
            Corrigir meus dados
          </button>
        </>
      )}
    </section>
  );
}
