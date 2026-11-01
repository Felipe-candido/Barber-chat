import { useId, useRef } from "react";
import { CalendarSearch, ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { addDays, dateKey, formatDate, parseDate, startOfWeek } from "@/lib/model";
import { agendaDate, type Agenda, type AdminAppointment } from "@/lib/api/agenda";
import { bookingTime } from "@/lib/api/booking";
import { appointmentStatusLabel } from "./appointment-status";
import { AppointmentList } from "./appointment-list";
import styles from "./agenda.module.css";

export type AgendaView = "day" | "week" | "month";

const views: { value: AgendaView; label: string }[] = [
  { value: "day", label: "Dia" },
  { value: "week", label: "Semana" },
  { value: "month", label: "Mês" },
];
const stepLabels: Record<AgendaView, [string, string]> = {
  day: ["Dia anterior", "Próximo dia"],
  week: ["Semana anterior", "Próxima semana"],
  month: ["Mês anterior", "Próximo mês"],
};

function shortMonth(date: Date) {
  return formatDate(date, { month: "short" }).replace(".", "");
}
function weekOf(key: string) {
  const start = startOfWeek(parseDate(key));
  return Array.from({ length: 7 }, (_, index) => addDays(start, index));
}
function periodTitle(view: AgendaView, focus: string, month: string, today: string) {
  if (view === "month") return formatDate(month + "-01", { month: "long", year: "numeric" });
  if (view === "day") return formatDate(focus, { weekday: "long", day: "numeric", month: "long" });
  const week = weekOf(focus);
  const [start, end] = [week[0], week[6]];
  const year = String(end.getFullYear()) === today.slice(0, 4) ? "" : ", " + end.getFullYear();
  return `${start.getDate()} ${shortMonth(start)} – ${end.getDate()} ${shortMonth(end)}${year}`;
}

export function AgendaCalendar({
  data,
  loading,
  view,
  focus,
  onViewChange,
  onNavigate,
  onOpenDay,
  onRefresh,
  onSelectAppointment,
}: {
  data: Agenda;
  loading: boolean;
  view: AgendaView;
  focus: string;
  onViewChange: (view: AgendaView) => void;
  onNavigate: (day: string) => void;
  onOpenDay: (day: string) => void;
  onRefresh: () => void;
  onSelectAppointment: (item: AdminAppointment) => void;
}) {
  const titleId = useId();
  const picker = useRef<HTMLInputElement>(null);
  const itemsByDay = new Map<string, AdminAppointment[]>();
  for (const item of [...data.appointments].sort(
    (a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at),
  )) {
    const key = agendaDate(item.starts_at, data.shop_timezone);
    itemsByDay.set(key, [...(itemsByDay.get(key) ?? []), item]);
  }
  const partial = !!data.next_cursor;

  function step(delta: number) {
    if (view === "month") {
      const anchor = parseDate(data.month + "-01");
      onNavigate(dateKey(new Date(anchor.getFullYear(), anchor.getMonth() + delta, 1)));
    } else onNavigate(dateKey(addDays(parseDate(focus), delta * (view === "week" ? 7 : 1))));
  }
  function openPicker() {
    const input = picker.current;
    if (!input) return;
    try {
      input.showPicker();
    } catch {
      input.focus();
      input.click();
    }
  }

  return (
    <section className={styles.calendar} aria-labelledby={titleId}>
      <div className={styles.toolbar}>
        <div className={styles.toolbarTop}>
          <h2 id={titleId}>{periodTitle(view, focus, data.month, data.today)}</h2>
          <div className={styles.toolbarTools}>
            <button
              type="button"
              className="icon-button"
              aria-label="Escolher data"
              disabled={loading}
              onClick={openPicker}
            >
              <CalendarSearch size={20} aria-hidden="true" />
            </button>
            <input
              ref={picker}
              type="date"
              tabIndex={-1}
              aria-hidden="true"
              className={styles.hiddenPicker}
              value={focus}
              onChange={(event) => event.target.value && onNavigate(event.target.value)}
            />
            <button
              type="button"
              className={styles.todayButton}
              disabled={loading}
              onClick={() => onNavigate(data.today)}
            >
              Hoje
            </button>
          </div>
        </div>
        <div className={styles.toolbarBottom}>
          <div className={styles.segmented} role="group" aria-label="Visualização da agenda">
            {views.map((item) => (
              <button
                key={item.value}
                type="button"
                aria-pressed={view === item.value}
                onClick={() => onViewChange(item.value)}
              >
                {item.label}
              </button>
            ))}
          </div>
          <div className={styles.stepper}>
            <button
              type="button"
              className="icon-button"
              aria-label={stepLabels[view][0]}
              disabled={loading}
              onClick={() => step(-1)}
            >
              <ChevronLeft size={20} aria-hidden="true" />
            </button>
            <button
              type="button"
              className="icon-button"
              aria-label={stepLabels[view][1]}
              disabled={loading}
              onClick={() => step(1)}
            >
              <ChevronRight size={20} aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>

      {view === "day" && (
        <DayView
          focus={focus}
          today={data.today}
          itemsByDay={itemsByDay}
          timezone={data.shop_timezone}
          partial={partial}
          onNavigate={onNavigate}
          onSelect={onSelectAppointment}
        />
      )}
      {view === "week" && (
        <WeekView
          focus={focus}
          today={data.today}
          itemsByDay={itemsByDay}
          timezone={data.shop_timezone}
          onOpenDay={onOpenDay}
          onSelect={onSelectAppointment}
        />
      )}
      {view === "month" && (
        <MonthView
          data={data}
          focus={focus}
          itemsByDay={itemsByDay}
          onOpenDay={onOpenDay}
          onSelect={onSelectAppointment}
        />
      )}

      <div className={styles.calendarFooter}>
        <p>
          <span>
            {data.appointments.length === 1
              ? "1 agendamento carregado, incluindo cancelados"
              : data.appointments.length + " agendamentos carregados, incluindo cancelados"}
          </span>
          <span>Horários em {data.shop_timezone}</span>
        </p>
        <button
          type="button"
          className={styles.refreshButton}
          disabled={loading}
          onClick={onRefresh}
        >
          <RefreshCw size={16} aria-hidden="true" />
          Atualizar agenda
        </button>
      </div>
    </section>
  );
}

function EventCard({
  item,
  timezone,
  onSelect,
}: {
  item: AdminAppointment;
  timezone: string;
  onSelect: (item: AdminAppointment) => void;
}) {
  return (
    <button
      type="button"
      className={styles.eventCard + " agenda-list-item"}
      data-status={item.status}
      onClick={() => onSelect(item)}
    >
      <span className={styles.eventTime}>
        {bookingTime(item.starts_at, timezone)}–{bookingTime(item.ends_at, timezone)}
      </span>
      <strong>{item.customer_name}</strong>
      <span className={styles.eventMeta}>
        {item.service_name} com {item.professional_name}
      </span>
      <span className={styles.eventStatus}>{appointmentStatusLabel(item)}</span>
    </button>
  );
}

function WeekView({
  focus,
  today,
  itemsByDay,
  timezone,
  onOpenDay,
  onSelect,
}: {
  focus: string;
  today: string;
  itemsByDay: Map<string, AdminAppointment[]>;
  timezone: string;
  onOpenDay: (day: string) => void;
  onSelect: (item: AdminAppointment) => void;
}) {
  return (
    <div className={styles.week}>
      {weekOf(focus).map((day) => {
        const key = dateKey(day);
        const items = itemsByDay.get(key) ?? [];
        return (
          <section
            key={key}
            data-date={key}
            className={styles.weekDay}
            data-empty={!items.length || undefined}
            aria-label={formatDate(day, { weekday: "long", day: "numeric", month: "long" })}
          >
            <button
              type="button"
              className={styles.weekDayHeader}
              aria-label={"Ver dia " + key}
              aria-current={key === today ? "date" : undefined}
              onClick={() => onOpenDay(key)}
            >
              <span>{formatDate(day, { weekday: "short" }).replace(".", "")}</span>
              <strong>{day.getDate()}</strong>
            </button>
            <div className={styles.weekEvents}>
              {items.length ? (
                items.map((item) => (
                  <EventCard key={item.id} item={item} timezone={timezone} onSelect={onSelect} />
                ))
              ) : (
                <p className={styles.free}>Sem agendamentos</p>
              )}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function DayView({
  focus,
  today,
  itemsByDay,
  timezone,
  partial,
  onNavigate,
  onSelect,
}: {
  focus: string;
  today: string;
  itemsByDay: Map<string, AdminAppointment[]>;
  timezone: string;
  partial: boolean;
  onNavigate: (day: string) => void;
  onSelect: (item: AdminAppointment) => void;
}) {
  const items = itemsByDay.get(focus) ?? [];
  return (
    <div className={styles.dayView}>
      <div className={styles.dayStrip} role="group" aria-label="Dias da semana">
        {weekOf(focus).map((day) => {
          const key = dateKey(day);
          const count = itemsByDay.get(key)?.length ?? 0;
          return (
            <button
              type="button"
              key={key}
              aria-label={"Ver dia " + key}
              aria-pressed={key === focus}
              aria-current={key === today ? "date" : undefined}
              onClick={() => onNavigate(key)}
            >
              <span>{formatDate(day, { weekday: "narrow" })}</span>
              <strong>{day.getDate()}</strong>
              <i data-filled={count > 0 || undefined} aria-hidden="true" />
            </button>
          );
        })}
      </div>
      <div className={styles.dayList}>
        <AppointmentList
          items={items}
          timezone={timezone}
          filtered
          partial={partial}
          showDate={false}
          onSelect={onSelect}
        />
      </div>
    </div>
  );
}

function MonthView({
  data,
  focus,
  itemsByDay,
  onOpenDay,
  onSelect,
}: {
  data: Agenda;
  focus: string;
  itemsByDay: Map<string, AdminAppointment[]>;
  onOpenDay: (day: string) => void;
  onSelect: (item: AdminAppointment) => void;
}) {
  const anchor = parseDate(data.month + "-01");
  const days = Array.from({ length: 42 }, (_, index) => addDays(parseDate(data.from_date), index));
  return (
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
              aria-pressed={focus === key}
              aria-current={key === data.today ? "date" : undefined}
              onClick={() => onOpenDay(key)}
            >
              {day.getDate()}
            </button>
            {appointments.length > 0 && (
              <span className={styles.dots} aria-hidden="true">
                {appointments.slice(0, 3).map((item) => (
                  <i key={item.id} data-status={item.status} />
                ))}
              </span>
            )}
            {appointments.slice(0, 3).map((item) => (
              <button
                type="button"
                key={item.id}
                className={styles.calendarEvent + " agenda-event"}
                data-status={item.status}
                onClick={() => onSelect(item)}
                title={
                  item.customer_name +
                  ", " +
                  item.service_name +
                  ", " +
                  appointmentStatusLabel(item)
                }
              >
                <strong>
                  {bookingTime(item.starts_at, data.shop_timezone)} {item.customer_name}
                </strong>
                <span>{item.service_name}</span>
                <small>{appointmentStatusLabel(item)}</small>
              </button>
            ))}
            {appointments.length > 3 && (
              <button type="button" className={styles.moreEvents} onClick={() => onOpenDay(key)}>
                Ver todos ({appointments.length})
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}
