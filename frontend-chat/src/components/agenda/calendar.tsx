import { useId, useState } from "react";
import { CalendarDays, ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import { addDays, dateKey, formatDate, parseDate } from "@/lib/model";
import { agendaDate, type Agenda, type AdminAppointment } from "@/lib/api/agenda";
import { bookingTime } from "@/lib/api/booking";
import { Button } from "../ui/button";
import { appointmentStatusLabel } from "./appointment-status";
import styles from "./agenda.module.css";

export function AgendaCalendar({
  data,
  loading,
  selectedDay,
  onSelectDay,
  onSelectAppointment,
  onMonthChange,
  onToday,
}: {
  data: Agenda;
  loading: boolean;
  selectedDay: string;
  onSelectDay: (day: string) => void;
  onSelectAppointment: (item: AdminAppointment) => void;
  onMonthChange: (month: string) => void;
  onToday: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const calendarId = useId();
  const titleId = useId();
  const anchor = parseDate(data.month + "-01");
  const days = Array.from({ length: 42 }, (_, index) => addDays(parseDate(data.from_date), index));
  const itemsByDay = new Map<string, AdminAppointment[]>();
  for (const item of [...data.appointments].sort(
    (a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at),
  )) {
    const key = agendaDate(item.starts_at, data.shop_timezone);
    itemsByDay.set(key, [...(itemsByDay.get(key) ?? []), item]);
  }
  function moveMonth(delta: number) {
    onMonthChange(
      dateKey(new Date(anchor.getFullYear(), anchor.getMonth() + delta, 1)).slice(0, 7),
    );
  }
  return (
    <section className={styles.calendar} aria-labelledby={titleId}>
      <div className={styles.calendarToolbar}>
        <h2 id={titleId}>
          <CalendarDays size={20} aria-hidden="true" />
          {formatDate(anchor, { month: "long", year: "numeric" })}
        </h2>
        <div className={styles.monthActions}>
          <button
            type="button"
            className="icon-button"
            aria-label="Mês anterior"
            disabled={loading}
            onClick={() => moveMonth(-1)}
          >
            <ChevronLeft size={20} aria-hidden="true" />
          </button>
          <Button variant="secondary" disabled={loading} onClick={onToday}>
            Hoje
          </Button>
          <button
            type="button"
            className="icon-button"
            aria-label="Próximo mês"
            disabled={loading}
            onClick={() => moveMonth(1)}
          >
            <ChevronRight size={20} aria-hidden="true" />
          </button>
        </div>
      </div>
      <button
        type="button"
        className={styles.calendarToggle}
        aria-controls={calendarId}
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? "Ocultar calendário mensal" : "Mostrar calendário mensal"}
        <ChevronDown size={18} aria-hidden="true" />
      </button>
      <div
        id={calendarId}
        className={styles.calendarBody + (expanded ? " " + styles.calendarExpanded : "")}
      >
        <div className={styles.monthGrid}>
          {["Seg", "Ter", "Qua", "Qui", "Sex", "Sáb", "Dom"].map((day) => (
            <div className={styles.weekday} key={day}>
              {day}
            </div>
          ))}
          {days.map((day) => {
            const key = dateKey(day);
            const appointments = itemsByDay.get(key) ?? [];
            return (
              <div
                key={key}
                data-date={key}
                className={
                  styles.monthCell +
                  (day.getMonth() !== anchor.getMonth() ? " " + styles.outsideMonth : "")
                }
              >
                <button
                  type="button"
                  className={styles.dayButton}
                  aria-label={"Ver dia " + key}
                  aria-pressed={selectedDay === key}
                  aria-current={key === data.today ? "date" : undefined}
                  onClick={() => onSelectDay(key)}
                >
                  {day.getDate()}
                </button>
                <span className={styles.mobileCount} aria-hidden="true">
                  {appointments.length || ""}
                </span>
                {appointments.slice(0, 4).map((item) => (
                  <button
                    type="button"
                    key={item.id}
                    className={styles.calendarEvent + " agenda-event"}
                    data-cancelled={item.status === "cancelled" || undefined}
                    onClick={() => onSelectAppointment(item)}
                    title={
                      item.customer_name +
                      " · " +
                      item.service_name +
                      " · " +
                      appointmentStatusLabel(item)
                    }
                  >
                    <strong>
                      {bookingTime(item.starts_at, data.shop_timezone)} · {item.customer_name}
                    </strong>
                    <span>
                      {item.service_name} · {item.professional_name}
                    </span>
                    <small>{appointmentStatusLabel(item)}</small>
                  </button>
                ))}
                {appointments.length > 4 && (
                  <button
                    type="button"
                    className={styles.moreEvents}
                    onClick={() => onSelectDay(key)}
                  >
                    Ver todos ({appointments.length})
                  </button>
                )}
              </div>
            );
          })}
        </div>
      </div>
      <p className={styles.calendarFooter}>
        {data.appointments.length} agendamento(s) carregado(s) · Inclui cancelamentos ·{" "}
        {data.next_cursor ? "Há mais reservas para carregar" : "Período completo"}
      </p>
    </section>
  );
}
