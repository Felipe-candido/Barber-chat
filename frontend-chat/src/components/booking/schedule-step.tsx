import { RefreshCw } from "lucide-react";
import { useId } from "react";
import { bookingTime } from "@/lib/api/booking";
import { Alert, Button, SelectField, TextField } from "../ui";
import type { useBookingFlow } from "./use-booking-flow";
import styles from "./booking.module.css";

export function ScheduleStep({
  flow,
  fixedProfessional,
  onBack,
}: {
  flow: ReturnType<typeof useBookingFlow>;
  fixedProfessional: boolean;
  onBack: () => void;
}) {
  const hintId = useId();
  const {
    professionals,
    professionalsError,
    setProfessionalsVersion,
    professionalId,
    setProfessionalId,
    date,
    setDate,
    availability,
    availabilityError,
    setAvailabilityVersion,
    setSlot,
    setStep,
    setError,
  } = flow;
  return (
    <>
      {professionalsError ? (
        <>
          <Alert tone="error">{professionalsError}</Alert>
          <Button variant="secondary" onClick={() => setProfessionalsVersion((value) => value + 1)}>
            Tentar carregar profissionais
          </Button>
        </>
      ) : !professionals ? (
        <p className={styles.loading} role="status">
          Carregando profissionais…
        </p>
      ) : !professionals.professionals.length ? (
        <p className={styles.hint}>
          Nenhum profissional habilitado para este serviço no momento. Escolha outro serviço ou
          consulte a barbearia.
        </p>
      ) : (
        <>
          <SelectField
            label="Profissional"
            value={professionalId}
            disabled={fixedProfessional}
            onChange={(event) => setProfessionalId(event.target.value)}
          >
            <option value="">Selecione um profissional</option>
            {professionals.professionals.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </SelectField>
          <TextField
            label="Data do agendamento"
            type="date"
            min={professionals.today}
            max={professionals.last_booking_date}
            value={date}
            onChange={(event) => setDate(event.target.value)}
          />
          <p className={styles.hint}>
            Horários de {professionals.shop_name} · {professionals.shop_timezone}
          </p>
          {professionalId && date && (
            <>
              {availabilityError ? (
                <Alert tone="error">{availabilityError}</Alert>
              ) : !availability ? (
                <p className={styles.loading} role="status">
                  Buscando horários…
                </p>
              ) : !availability.slots.length ? (
                <p className={styles.hint}>
                  Nenhum horário livre nesta data. Escolha outro dia ou profissional.
                </p>
              ) : (
                <div className={styles.stack}>
                  <p className={styles.hint} id={hintId}>
                    Escolha um horário para continuar. A reserva será feita só após a confirmação.
                  </p>
                  <div
                    className={styles.slots}
                    role="group"
                    aria-label="Horários disponíveis"
                    aria-describedby={hintId}
                  >
                    {availability.slots.map((item) => (
                      <button
                        className={styles.slot}
                        key={item.starts_at}
                        type="button"
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
                </div>
              )}
              <Button
                variant="secondary"
                onClick={() => setAvailabilityVersion((value) => value + 1)}
              >
                <RefreshCw size={16} aria-hidden="true" />
                Atualizar horários
              </Button>
            </>
          )}
        </>
      )}
      <Button variant="quiet" onClick={onBack}>
        Escolher outro serviço
      </Button>
    </>
  );
}
