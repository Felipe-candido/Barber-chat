import { bookingDate, bookingTime } from "@/lib/api/booking";
import { Button, SelectField, TextField } from "../ui";
import type { useBookingFlow } from "./use-booking-flow";
import styles from "./booking.module.css";

export function ContactStep({
  flow,
  zone,
}: {
  flow: ReturnType<typeof useBookingFlow>;
  zone: string;
}) {
  const {
    customer,
    setCustomer,
    slot,
    professionals,
    professionalId,
    setStep,
    setAvailabilityVersion,
  } = flow;
  if (!slot) return null;
  return (
    <>
      <p className={styles.hint}>
        {professionals?.professionals.find((item) => item.id === professionalId)?.name} ·{" "}
        {bookingDate(slot.starts_at, zone)} às {bookingTime(slot.starts_at, zone)}
      </p>
      <form
        className={styles.form}
        onSubmit={(event) => {
          event.preventDefault();
          setStep("review");
        }}
      >
        <TextField
          label="Nome"
          required
          maxLength={100}
          autoComplete="given-name"
          value={customer.first_name}
          onChange={(event) => setCustomer({ ...customer, first_name: event.target.value })}
        />
        <TextField
          label="Sobrenome"
          required
          maxLength={100}
          autoComplete="family-name"
          value={customer.last_name}
          onChange={(event) => setCustomer({ ...customer, last_name: event.target.value })}
        />
        <SelectField
          label="País do telefone"
          value={customer.phone_country}
          onChange={(event) => setCustomer({ ...customer, phone_country: event.target.value })}
        >
          <option value="BR">Brasil (+55)</option>
          <option value="PT">Portugal (+351)</option>
          <option value="US">Estados Unidos (+1)</option>
        </SelectField>
        <TextField
          label="Telefone com DDD"
          required
          type="tel"
          maxLength={40}
          autoComplete="tel"
          placeholder="(11) 91234-5678"
          value={customer.phone}
          onChange={(event) => setCustomer({ ...customer, phone: event.target.value })}
          hint="Se o país não estiver na lista, informe o número completo com + e o código do país."
        />
        <p className={styles.hint}>
          Seus dados serão enviados à barbearia somente ao confirmar. Não será criada uma conta nem
          enviada uma mensagem pelo WhatsApp nesta etapa.
        </p>
        <Button type="submit">Revisar agendamento</Button>
      </form>
      <Button
        variant="quiet"
        onClick={() => {
          setStep("schedule");
          setAvailabilityVersion((value) => value + 1);
        }}
      >
        Escolher outro horário
      </Button>
    </>
  );
}
