import { ArrowUpRight } from "lucide-react";
import type { AdminAppointment } from "@/lib/api/agenda";
import { agendaDate } from "@/lib/api/agenda";
import { bookingTime } from "@/lib/api/booking";
import { formatDate, money } from "@/lib/model";
import { AppointmentStatus } from "./appointment-status";
import styles from "./agenda.module.css";

export function AppointmentList({
  items,
  timezone,
  filtered,
  partial,
  showDate = true,
  onSelect,
}: {
  items: AdminAppointment[];
  timezone: string;
  filtered: boolean;
  partial: boolean;
  showDate?: boolean;
  onSelect: (item: AdminAppointment) => void;
}) {
  if (!items.length)
    return (
      <p className={styles.empty}>
        Nenhum agendamento {filtered ? "neste dia" : "no período exibido"}
        {partial ? " entre as reservas carregadas" : ""}.
      </p>
    );
  return (
    <div className={styles.appointmentList}>
      {items.map((item) => (
        <button
          type="button"
          key={item.id}
          className={styles.appointment + " agenda-list-item"}
          data-status={item.status}
          onClick={() => onSelect(item)}
        >
          <span className={styles.appointmentTime}>
            <strong>{bookingTime(item.starts_at, timezone)}</strong>
            <span>até {bookingTime(item.ends_at, timezone)}</span>
            {showDate && (
              <span>
                {formatDate(agendaDate(item.starts_at, timezone), {
                  day: "2-digit",
                  month: "2-digit",
                })}
              </span>
            )}
          </span>
          <span className={styles.appointmentContent}>
            <strong>{item.customer_name}</strong>
            <span>{item.service_name}</span>
            <span>
              {item.professional_name} · Serviço de {item.duration_minutes} min
            </span>
            <AppointmentStatus status={item.status} source={item.status_source} />
          </span>
          <span className={styles.appointmentPrice}>
            {money(item.price_cents)}
            <ArrowUpRight size={18} aria-hidden="true" />
          </span>
        </button>
      ))}
    </div>
  );
}
