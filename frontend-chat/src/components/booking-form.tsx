"use client";

import { useEffect, useRef } from "react";
import type { Service } from "@/lib/api/services";
import { money } from "@/lib/model";
import { AppointmentReceipt } from "./appointment-receipt";
import { Alert } from "./ui";
import { useBookingFlow, type StaffBookingContext } from "./booking/use-booking-flow";
import { StaffSubscriptionRecovery } from "./booking/staff-subscription-recovery";
import { BookingProgress, type BookingStage } from "./booking/progress";
import { ScheduleStep } from "./booking/schedule-step";
import { ContactStep } from "./booking/contact-step";
import { ReviewStep } from "./booking/review-step";
import styles from "./booking/booking.module.css";

export function BookingForm({
  slug,
  service,
  onBack,
  onLockedChange,
  staff,
  onStageChange,
  onShopName,
}: {
  slug: string;
  service: Service;
  onBack: () => void;
  onLockedChange: (locked: boolean) => void;
  staff?: StaffBookingContext;
  onStageChange?: (stage: BookingStage) => void;
  onShopName?: (name: string) => void;
}) {
  const flow = useBookingFlow({ slug, service, onLockedChange, staff });
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    onStageChange?.(flow.result ? "complete" : flow.step);
  }, [flow.step, flow.result, onStageChange]);
  useEffect(() => {
    if (flow.professionals) onShopName?.(flow.professionals.shop_name);
  }, [flow.professionals, onShopName]);
  useEffect(() => {
    heading.current?.focus();
  }, [flow.step]);
  if (flow.result)
    return (
      <AppointmentReceipt
        slug={slug}
        code={flow.result.code}
        initial={flow.result.appointment}
        onShopName={onShopName}
        onLockedChange={onLockedChange}
      />
    );
  const zone = flow.availability?.shop_timezone ?? flow.professionals?.shop_timezone;
  return (
    <section className={styles.flow} aria-label="Novo agendamento">
      {staff && <BookingProgress stage={flow.step} />}
      <div className={styles.selection}>
        <div>
          <strong>{service.name}</strong>
          <span className={styles.muted}>{service.duration_minutes} min de serviço</span>
        </div>
        <strong>{money(service.price_cents)}</strong>
      </div>
      <h2 tabIndex={-1} ref={heading}>
        {flow.step === "schedule"
          ? "Escolha profissional e horário"
          : flow.step === "contact"
            ? "Seus dados para o agendamento"
            : "Confira antes de confirmar"}
      </h2>
      {flow.error && <Alert tone="error">{flow.error}</Alert>}
      {flow.notice && <Alert>{flow.notice}</Alert>}
      {flow.step === "schedule" && (
        <ScheduleStep flow={flow} fixedProfessional={!!staff?.professionalID} onBack={onBack} />
      )}
      {flow.slot && zone && flow.step === "contact" && <ContactStep flow={flow} zone={zone} />}
      {flow.slot && zone && flow.step === "review" && <ReviewStep flow={flow} zone={zone} />}
      {staff && flow.subscriptionRequired && (
        <StaffSubscriptionRecovery slug={slug} staff={staff} flow={flow} />
      )}
    </section>
  );
}
