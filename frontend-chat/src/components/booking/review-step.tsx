import { bookingDate, bookingTime } from "@/lib/api/booking";
import { Alert, Button } from "../ui";
import type { useBookingFlow } from "./use-booking-flow";
import styles from "./booking.module.css";

export function ReviewStep({
  flow,
  zone,
}: {
  flow: ReturnType<typeof useBookingFlow>;
  zone: string;
}) {
  const {
    slot,
    professionals,
    professionalId,
    customer,
    uncertain,
    busy,
    locked,
    setStep,
    setError,
    confirm,
  } = flow;
  if (!slot) return null;
  return (
    <>
      <dl className={styles.details}>
        <div>
          <dt>Profissional</dt>
          <dd>{professionals?.professionals.find((item) => item.id === professionalId)?.name}</dd>
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
      <p className={styles.hint}>
        O horário só fica reservado após a confirmação. O código será exibido na tela para você
        consultar e cancelar antes do início.
      </p>
      {uncertain && (
        <Alert tone="warning">
          Mantenha esta página aberta e use “Repetir confirmação”. Os mesmos dados e a mesma chave
          serão usados para recuperar esta tentativa.
        </Alert>
      )}
      <Button
        loading={busy}
        disabled={flow.subscriptionRequired || flow.billingBusy}
        onClick={() => void confirm()}
      >
        {busy ? "Confirmando…" : uncertain ? "Repetir confirmação" : "Confirmar agendamento"}
      </Button>
      <Button
        variant="quiet"
        disabled={locked || flow.subscriptionRequired}
        onClick={() => {
          setStep("contact");
          setError("");
        }}
      >
        Corrigir meus dados
      </Button>
    </>
  );
}
