"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import {
  ArrowUpRight,
  CalendarDays,
  Check,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Plus,
  Search,
  Wallet,
} from "lucide-react";
import {
  Appointment,
  addDays,
  dateKey,
  formatDate,
  money,
  minutes,
  parseDate,
  professionals,
  startOfWeek,
  timeString,
} from "@/lib/model";
import { useDemo } from "./demo-provider";
import { LoadingScreen } from "./ui";
import { AppointmentDetails, AppointmentForm } from "./appointment-modal";

type View = "Dia" | "Semana" | "Mês";
export function AgendaPage() {
  const { data, ready } = useDemo();
  const [anchor, setAnchor] = useState(() => new Date());
  const [view, setView] = useState<View>("Semana");
  const [professional, setProfessional] = useState("all");
  const [query, setQuery] = useState("");
  const [form, setForm] = useState<{
    date?: string;
    time?: string;
    appointment?: Appointment;
  } | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  useEffect(() => {
    const requested = new URLSearchParams(window.location.search).get("date");
    if (
      requested &&
      /^\d{4}-\d{2}-\d{2}$/.test(requested) &&
      !Number.isNaN(parseDate(requested).getTime())
    )
      setAnchor(parseDate(requested));
  }, []);
  if (!ready) return <LoadingScreen />;
  const today = dateKey(new Date());
  const todays = data.appointments.filter((a) => a.date === today && a.status !== "cancelled");
  const confirmed = todays.filter((a) => a.status === "confirmed" || a.status === "completed");
  const visible = data.appointments.filter(
    (a) =>
      a.status !== "cancelled" &&
      (professional === "all" || a.professional === professional) &&
      (!query || (a.customer + " " + a.serviceName).toLowerCase().includes(query.toLowerCase())),
  );
  const days =
    view === "Dia"
      ? [anchor]
      : Array.from({ length: 6 }, (_, i) => addDays(startOfWeek(anchor), i));
  const detail = data.appointments.find((a) => a.id === selected);
  function move(direction: number) {
    if (view === "Mês") setAnchor(new Date(anchor.getFullYear(), anchor.getMonth() + direction, 1));
    else setAnchor(addDays(anchor, direction * (view === "Semana" ? 7 : 1)));
  }
  const monthStart = new Date(anchor.getFullYear(), anchor.getMonth(), 1);
  const monthDays = Array.from({ length: 42 }, (_, i) => addDays(startOfWeek(monthStart), i));
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">UM BOM DIA COMEÇA AQUI</div>
          <h1>
            Sua agenda, em dia<span className="heading-dot">.</span>
          </h1>
          <p>Mais organização. Mais tempo para fazer o que você faz de melhor.</p>
        </div>
        <button className="button button-primary" onClick={() => setForm({})}>
          <Plus size={18} />
          Novo agendamento
        </button>
      </div>
      <section className="stats-grid" aria-label="Resumo de hoje">
        <div className="stat-card">
          <span className="stat-icon blue">
            <CalendarDays size={21} />
          </span>
          <div>
            <span>Agendamentos hoje</span>
            <strong>
              {todays.length.toString().padStart(2, "0")}
              <small>horários reservados</small>
            </strong>
          </div>
        </div>
        <div className="stat-card">
          <span className="stat-icon sage">
            <Check size={21} />
          </span>
          <div>
            <span>Confirmados hoje</span>
            <strong>
              {confirmed.length.toString().padStart(2, "0")}
              <small>tudo certo para receber</small>
            </strong>
          </div>
        </div>
        <div className="stat-card">
          <span className="stat-icon sand">
            <Wallet size={21} />
          </span>
          <div>
            <span>Valor previsto hoje</span>
            <strong>
              {money(todays.reduce((sum, a) => sum + a.price, 0))}
              <small>agendamentos do dia</small>
            </strong>
          </div>
        </div>
      </section>
      <section className="calendar-card" aria-label="Calendário de agendamentos">
        <div className="calendar-toolbar">
          <div className="calendar-date-nav">
            <h2>{formatDate(anchor, { month: "long", year: "numeric" })}</h2>
            <div className="flex gap-1">
              <button
                className="icon-button"
                aria-label="Período anterior"
                onClick={() => move(-1)}
              >
                <ChevronLeft size={18} />
              </button>
              <button className="icon-button" aria-label="Próximo período" onClick={() => move(1)}>
                <ChevronRight size={18} />
              </button>
            </div>
            <button className="today-button" onClick={() => setAnchor(new Date())}>
              Hoje
            </button>
          </div>
          <div className="view-switch" aria-label="Visualização">
            {(["Dia", "Semana", "Mês"] as View[]).map((v) => (
              <button key={v} aria-pressed={view === v} onClick={() => setView(v)}>
                {v}
              </button>
            ))}
          </div>
        </div>
        <div className="calendar-filters">
          <div className="flex items-center gap-2">
            <span className="muted">
              <CalendarDays size={16} />
            </span>
            <select
              aria-label="Filtrar por profissional"
              value={professional}
              onChange={(e) => setProfessional(e.target.value)}
            >
              <option value="all">Todos os profissionais</option>
              {professionals.map((p) => (
                <option value={p.id} key={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
          <label className="search-field">
            <Search size={16} />
            <input
              placeholder="Buscar na agenda"
              aria-label="Buscar na agenda"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
        </div>
        {view === "Mês" ? (
          <div className="month-scroll">
            <div className="month-grid">
              {["SEG", "TER", "QUA", "QUI", "SEX", "SÁB", "DOM"].map((d) => (
                <div className="month-weekday" key={d}>
                  {d}
                </div>
              ))}
              {monthDays.map((day) => {
                const key = dateKey(day);
                const appointments = visible
                  .filter((a) => a.date === key)
                  .sort((a, b) => a.time.localeCompare(b.time));
                return (
                  <div
                    key={key}
                    className={
                      "month-cell " + (day.getMonth() !== anchor.getMonth() ? "outside-month" : "")
                    }
                  >
                    <button
                      className={"month-day " + (key === today ? "is-today" : "")}
                      aria-label={"Ver dia " + formatDate(day, { day: "numeric", month: "long" })}
                      onClick={() => {
                        setAnchor(day);
                        setView("Dia");
                      }}
                    >
                      {day.getDate()}
                    </button>
                    {appointments.slice(0, 3).map((a) => (
                      <button
                        key={a.id}
                        className={"month-event event-" + a.professional}
                        onClick={() => setSelected(a.id)}
                      >
                        <b>{a.time}</b> {a.customer}
                      </button>
                    ))}
                    {appointments.length > 3 && (
                      <button
                        className="more-events"
                        onClick={() => {
                          setAnchor(day);
                          setView("Dia");
                        }}
                      >
                        +{appointments.length - 3} agendamentos
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
          <div className="week-scroll">
            <div
              className={"week-calendar " + (view === "Dia" ? "single-day" : "")}
              style={{
                gridTemplateColumns: "58px repeat(" + days.length + ", minmax(130px, 1fr))",
              }}
            >
              <div className="time-heading">
                <Clock3 size={15} />
              </div>
              {days.map((day) => (
                <div
                  key={dateKey(day)}
                  className={"day-heading " + (dateKey(day) === today ? "today-column" : "")}
                >
                  <span>{formatDate(day, { weekday: "short" }).replace(".", "")}</span>
                  <b className={dateKey(day) === today ? "is-today" : ""}>{day.getDate()}</b>
                </div>
              ))}
              <div className="time-axis">
                {Array.from({ length: 10 }, (_, i) => (
                  <span key={i} style={{ top: i * 76 }}>
                    {timeString(540 + i * 60)}
                  </span>
                ))}
                <span style={{ top: 760 }}>19:00</span>
              </div>
              {days.map((day) => {
                const key = dateKey(day);
                const events = visible
                  .filter((a) => a.date === key)
                  .sort((a, b) => a.time.localeCompare(b.time));
                return (
                  <div key={key} className={"day-column " + (key === today ? "today-column" : "")}>
                    {Array.from({ length: 20 }, (_, i) => (
                      <button
                        key={i}
                        className="calendar-slot"
                        style={{ top: i * 38 }}
                        disabled={key < today || day.getDay() === 0}
                        aria-label={
                          "Agendar " +
                          formatDate(day, { day: "numeric", month: "long" }) +
                          " às " +
                          timeString(540 + i * 30)
                        }
                        onClick={() => setForm({ date: key, time: timeString(540 + i * 30) })}
                      >
                        <Plus size={14} />
                      </button>
                    ))}
                    {events.map((a) => {
                      const collides = events.filter(
                        (b) =>
                          b.id !== a.id &&
                          minutes(a.time) < minutes(b.time) + b.duration &&
                          minutes(a.time) + a.duration > minutes(b.time),
                      );
                      const lane = collides.length
                        ? professionals.findIndex((p) => p.id === a.professional)
                        : 0;
                      return (
                        <button
                          key={a.id}
                          className={
                            "calendar-event event-" +
                            a.professional +
                            (a.duration < 30 ? " short-event" : "")
                          }
                          style={{
                            top: ((minutes(a.time) - 540) / 60) * 76 + 2,
                            height: Math.max((a.duration / 60) * 76 - 4, 24),
                            left: collides.length ? lane * 50 + 2 + "%" : "5px",
                            width: collides.length ? "46%" : "calc(100% - 10px)",
                          }}
                          onClick={() => setSelected(a.id)}
                          aria-label={a.customer + ", " + a.serviceName + ", " + a.time}
                        >
                          <span className="event-time">
                            {a.time}
                            <span className={"event-state " + a.status} />
                          </span>
                          <strong>{a.customer}</strong>
                          {a.duration >= 45 && (
                            <span className="event-service">{a.serviceName}</span>
                          )}
                        </button>
                      );
                    })}
                    {day.getDay() === 0 && <span className="closed-day">Fechado</span>}
                  </div>
                );
              })}
            </div>
          </div>
        )}
        <div className="calendar-footer">
          <div className="calendar-legend">
            {professionals.map((p) => (
              <span key={p.id}>
                <i className={"legend-" + p.id} />
                {p.name}
              </span>
            ))}
          </div>
          <span>Horário local · 9h às 19h</span>
        </div>
      </section>
      <div className="under-calendar">
        <p>
          <span className="mini-dot" />
          Uma agenda organizada abre espaço para um bom atendimento.
        </p>
        <Link href="/chat">
          Conhecer o chat de agendamento
          <ArrowUpRight size={15} />
        </Link>
      </div>
      {form && (
        <AppointmentForm
          {...form}
          onClose={() => setForm(null)}
          onSaved={(a) => {
            setAnchor(parseDate(a.date));
            setQuery("");
            setProfessional("all");
          }}
        />
      )}
      {detail && !form && (
        <AppointmentDetails
          appointment={detail}
          onClose={() => setSelected(null)}
          onEdit={() => {
            setForm({ appointment: detail });
            setSelected(null);
          }}
        />
      )}
    </>
  );
}
