import { agendaDate, type AdminAppointment } from "@/lib/api/agenda";
import { bookingTime } from "@/lib/api/booking";
import { formatDate, money } from "@/lib/model";
import { AppointmentStatus } from "./appointment-status";
import { Button } from "../ui";
import styles from "./agenda.module.css";

export function AppointmentDetails({
  item,
  timezone,
  onSell,
}: {
  item: AdminAppointment;
  timezone: string;
  onSell?: () => void;
}) {
  const details = [
    ["Cliente", item.customer_name],
    ["Telefone", item.customer_phone],
    ["Serviço", item.service_name],
    ["Barbeiro", item.professional_name],
    [
      "Data",
      formatDate(agendaDate(item.starts_at, timezone), {
        day: "2-digit",
        month: "long",
        year: "numeric",
      }),
    ],
    [
      "Horário reservado",
      bookingTime(item.starts_at, timezone) +
        "–" +
        bookingTime(item.ends_at, timezone) +
        " (" +
        timezone +
        ")",
    ],
    ["Duração do serviço", item.duration_minutes + " minutos"],
    ["Valor na reserva", money(item.price_cents)],
  ];
  return (
    <div className={styles.details}>
      <AppointmentStatus status={item.status} source={item.status_source} />
      <dl>
        {details.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <p>
        {item.version === undefined
          ? "Consulta dos dados da reserva. Atualize a API para gerenciar a situação pelo painel."
          : "Atendimentos são concluídos automaticamente após o horário reservado. Ausências e cancelamentos podem ser corrigidos com um motivo."}
      </p>
      {onSell && ["scheduled", "confirmed", "completed"].includes(item.status) && (
        <Button variant="secondary" onClick={onSell}>
          Registrar produtos neste atendimento
        </Button>
      )}
    </div>
  );
}
