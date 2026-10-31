"use client";

import { useEffect, useRef, useState } from "react";
import {
  Availability,
  BookingInput,
  BookingResult,
  Professionals,
  Slot,
  createAppointment,
  createStaffAppointment,
  listAvailability,
  listProfessionals,
  newBookingKey,
} from "@/lib/api/booking";
import { ApiError, errorMessage } from "@/lib/api/client";
import type { Service } from "@/lib/api/services";

export type StaffBookingContext = {
  professionalID: string | null;
  shopID: string;
  userID: string;
  role: "owner" | "barber";
  onCreated: () => void;
};

export function useBookingFlow({
  slug,
  service,
  onLockedChange,
  staff,
}: {
  slug: string;
  service: Service;
  onLockedChange: (locked: boolean) => void;
  staff?: StaffBookingContext;
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
  const [subscriptionRequired, setSubscriptionRequired] = useState(false);
  const [billingBusy, setBillingBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [result, setResult] = useState<BookingResult | null>(null);
  const attempt = useRef<{ key: string; input: BookingInput } | null>(null);
  const submitting = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const locked = busy || uncertain || billingBusy;

  useEffect(() => {
    const controller = new AbortController();
    setProfessionals(null);
    setProfessionalsError("");
    void listProfessionals(slug, service.id, controller.signal, staff?.userID)
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
  }, [slug, service.id, professionalsVersion, staff?.professionalID, staff?.userID]);

  useEffect(() => {
    setSlot(null);
    setAvailability(null);
    setAvailabilityError("");
    if (!professionalId || !date || !professionals) return;
    const controller = new AbortController();
    void listAvailability(slug, service.id, professionalId, date, controller.signal, staff?.userID)
      .then((data) => {
        if (!controller.signal.aborted) setAvailability(data);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setAvailabilityError(errorMessage(error));
      });
    return () => controller.abort();
  }, [slug, service.id, professionalId, date, professionals, availabilityVersion, staff?.userID]);

  useEffect(() => {
    onLockedChange(locked);
    return () => onLockedChange(false);
  }, [locked, onLockedChange]);
  useEffect(() => {
    if (
      !locked &&
      !(staff && (slot || customer.first_name || customer.last_name || customer.phone))
    )
      return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [locked, staff, slot, customer]);

  async function resumeAfterSubscription() {
    if (!staff || !subscriptionRequired || submitting.current || billingBusy) return;
    // An uncertain reservation must recover its original receipt/key first.
    // Its own saved slot may legitimately be absent from availability now.
    if (uncertain) {
      setSubscriptionRequired(false);
      setNotice(
        "Use Repetir confirmação para recuperar os mesmos dados e a mesma chave. A API vai revalidar seu acesso.",
      );
      return;
    }
    submitting.current = true;
    setBusy(true);
    setError("");
    try {
      const current = await listAvailability(
        slug,
        service.id,
        professionalId,
        date,
        undefined,
        staff.userID,
      );
      if (!alive.current) return;
      setAvailability(current);
      const selected = current.slots.find(
        (item) =>
          !!slot &&
          Date.parse(item.starts_at) === Date.parse(slot.starts_at) &&
          Date.parse(item.ends_at) === Date.parse(slot.ends_at),
      );
      setSlot(selected ?? null);
      setStep(selected ? "review" : "schedule");
      setSubscriptionRequired(false);
      setNotice(
        selected
          ? staff.role === "owner"
            ? "Assinatura confirmada e horário reconsultado. Revise os dados e confirme o agendamento."
            : "Horário reconsultado. Revise e confirme após o proprietário regularizar a assinatura; a API fará a validação final."
          : "Seus dados foram mantidos, mas o horário mudou ou foi ocupado. Escolha outro horário.",
      );
    } catch (failure) {
      if (alive.current) setError(errorMessage(failure));
    } finally {
      submitting.current = false;
      if (alive.current) setBusy(false);
    }
  }

  async function confirm() {
    if (submitting.current || billingBusy || subscriptionRequired || !slot || !professionalId)
      return;
    submitting.current = true;
    setBusy(true);
    setError("");
    setNotice("");
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
      const saved = await (staff
        ? createStaffAppointment(slug, attempt.current.input, attempt.current.key, staff.userID)
        : createAppointment(slug, attempt.current.input, attempt.current.key));
      if (!alive.current) return;
      setResult(saved);
      setUncertain(false);
      attempt.current = null;
      setCustomer({ first_name: "", last_name: "", phone: "", phone_country: "BR" });
      staff?.onCreated();
    } catch (error) {
      if (!alive.current) return;
      const billingDenied =
        !!staff &&
        error instanceof ApiError &&
        error.code === "shop_subscription_required" &&
        [402, 403].includes(error.status);
      setError(billingDenied ? "" : errorMessage(error));
      if (billingDenied) setSubscriptionRequired(true);
      const definitive =
        error instanceof ApiError &&
        error.status >= 400 &&
        error.status < 500 &&
        !["idempotency_conflict", "idempotency_expired"].includes(error.code);
      setUncertain(uncertain || !definitive);
      if (definitive && !uncertain) attempt.current = null;
      if (!uncertain && error instanceof ApiError && error.code === "slot_unavailable") {
        setStep("schedule");
        setAvailabilityVersion((value) => value + 1);
      }
    } finally {
      submitting.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return {
    professionals,
    professionalId,
    setProfessionalId,
    date,
    setDate,
    availability,
    slot,
    setSlot,
    customer,
    setCustomer,
    step,
    setStep,
    professionalsError,
    availabilityError,
    error,
    setError,
    setProfessionalsVersion,
    setAvailabilityVersion,
    busy,
    uncertain,
    result,
    locked,
    subscriptionRequired,
    billingBusy,
    setBillingBusy,
    notice,
    resumeAfterSubscription,
    confirm,
  };
}
