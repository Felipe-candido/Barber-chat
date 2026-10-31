import type { AdminAppointment } from "@/lib/api/agenda";
import { Badge } from "../ui/badge";

export const statusLabels: Record<AdminAppointment["status"], string> = {
  scheduled: "Agendado",
  confirmed: "Confirmado",
  cancelled: "Cancelado",
  completed: "Concluído",
  no_show: "Não compareceu",
};
export function appointmentStatusLabel(item: Pick<AdminAppointment, "status" | "status_source">) {
  return item.status === "completed" && item.status_source === "automatic"
    ? "Concluído automaticamente"
    : statusLabels[item.status];
}
export function AppointmentStatus({
  status,
  source,
}: {
  status: AdminAppointment["status"];
  source?: AdminAppointment["status_source"];
}) {
  const tone =
    status === "confirmed" || status === "completed"
      ? "success"
      : status === "scheduled"
        ? "info"
        : status === "no_show"
          ? "warning"
          : "neutral";
  return <Badge tone={tone}>{appointmentStatusLabel({ status, status_source: source })}</Badge>;
}
