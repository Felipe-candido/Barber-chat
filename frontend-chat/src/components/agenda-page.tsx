"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { CalendarDays, ChevronLeft, ChevronRight, Plus, RefreshCw } from "lucide-react";
import { addDays, dateKey, formatDate, money, parseDate } from "@/lib/model";
import { useSelectedShop } from "./shop-panel";
import { publicBookingPath } from "@/lib/api/config";
import { agendaDate, listAgenda, type Agenda, type AdminAppointment } from "@/lib/api/agenda";
import { bookingTime } from "@/lib/api/booking";
import { ApiError, errorMessage } from "@/lib/api/client";
import { Modal } from "./ui";

const statuses: Record<AdminAppointment["status"], string> = {
  scheduled: "Agendado",
  confirmed: "Confirmado",
  cancelled: "Cancelado",
  completed: "Concluído",
  no_show: "Não compareceu",
};

export function AgendaPage() {
  const shop = useSelectedShop();
  // The initial month comes from the backend in the shop's timezone.
  const [month, setMonth] = useState<string>();
  const [after, setAfter] = useState<string>();
  const [revision, setRevision] = useState(0);
  const [data, setData] = useState<Agenda | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [dayFilter, setDayFilter] = useState("");
  const [selected, setSelected] = useState<AdminAppointment | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setSelected(null);
    if (!after) setData(null);
    listAgenda(shop.slug, month, after, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setData((previous) => {
          if (!after || !previous || previous.month !== result.month) return result;
          const items = new Map(previous.appointments.map((item) => [item.id, item]));
          result.appointments.forEach((item) => items.set(item.id, item));
          return { ...result, appointments: [...items.values()] };
        });
      })
      .catch((cause) => {
        if (controller.signal.aborted) return;
        if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403))
          setData(null);
        setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [shop.slug, month, after, revision]);

  function refresh() {
    setAfter(undefined);
    setRevision((value) => value + 1);
  }
  useEffect(() => {
    // Returning from a public booking tab refreshes committed appointments.
    const focus = () => {
      setAfter(undefined);
      setRevision((value) => value + 1);
    };
    window.addEventListener("focus", focus);
    return () => window.removeEventListener("focus", focus);
  }, []);

  function showMonth(value: string) {
    setMonth(value);
    setAfter(undefined);
    setDayFilter("");
    setData(null);
    setSelected(null);
    setRevision((current) => current + 1);
  }
  const anchor = data ? parseDate(data.month + "-01") : null;
  const days = data
    ? Array.from({ length: 42 }, (_, i) => addDays(parseDate(data.from_date), i))
    : [];
  const itemsByDay = new Map<string, AdminAppointment[]>();
  for (const item of data?.appointments ?? []) {
    const day = agendaDate(item.starts_at, data!.shop_timezone);
    itemsByDay.set(day, [...(itemsByDay.get(day) ?? []), item]);
  }
  const visible = (data?.appointments ?? []).filter(
    (item) => !dayFilter || agendaDate(item.starts_at, data!.shop_timezone) === dayFilter,
  );
  const time = (value: string) => bookingTime(value, data!.shop_timezone);

  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">SEU ESPAÇO DE GESTÃO</div>
          <h1>
            Sua agenda, em dia<span className="heading-dot">.</span>
          </h1>
          <p>Agendamentos de {shop.name}, consultados no banco.</p>
        </div>
        <Link
          className="button button-primary"
          href={publicBookingPath(shop.slug)}
          target="_blank"
          rel="noopener noreferrer"
        >
          <Plus size={18} /> Novo agendamento
        </Link>
      </div>
      <div className="agenda-actions">
        <button className="button button-secondary" onClick={refresh} disabled={loading}>
          <RefreshCw size={16} /> Atualizar agenda
        </button>
        {data && <span>Horários em {data.shop_timezone}</span>}
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error} Nenhum agendamento foi alterado. Use Atualizar agenda para tentar novamente.
        </p>
      )}
      {loading && <p role="status">Carregando agendamentos…</p>}
      {data && anchor && (
        <>
          <section className="calendar-card" aria-label="Calendário de agendamentos">
            <div className="calendar-toolbar">
              <div className="calendar-date-nav">
                <CalendarDays size={20} />
                <h2>{formatDate(anchor, { month: "long", year: "numeric" })}</h2>
                <button
                  className="icon-button"
                  aria-label="Mês anterior"
                  disabled={loading}
                  onClick={() =>
                    showMonth(
                      dateKey(new Date(anchor.getFullYear(), anchor.getMonth() - 1, 1)).slice(0, 7),
                    )
                  }
                >
                  <ChevronLeft size={18} />
                </button>
                <button
                  className="icon-button"
                  aria-label="Próximo mês"
                  disabled={loading}
                  onClick={() =>
                    showMonth(
                      dateKey(new Date(anchor.getFullYear(), anchor.getMonth() + 1, 1)).slice(0, 7),
                    )
                  }
                >
                  <ChevronRight size={18} />
                </button>
                <button
                  className="today-button"
                  disabled={loading}
                  onClick={() => showMonth(data.today.slice(0, 7))}
                >
                  Hoje
                </button>
              </div>
            </div>
            <div className="month-scroll">
              <div className="month-grid">
                {["SEG", "TER", "QUA", "QUI", "SEX", "SÁB", "DOM"].map((day) => (
                  <div className="month-weekday" key={day}>
                    {day}
                  </div>
                ))}
                {days.map((day) => {
                  const key = dateKey(day),
                    appointments = itemsByDay.get(key) ?? [];
                  return (
                    <div
                      className={
                        "month-cell " +
                        (day.getMonth() !== anchor.getMonth() ? "outside-month" : "")
                      }
                      key={key}
                      data-date={key}
                    >
                      <button
                        className={"month-day " + (key === data.today ? "is-today" : "")}
                        aria-label={"Ver dia " + key}
                        onClick={() => setDayFilter(key)}
                      >
                        {day.getDate()}
                      </button>
                      {appointments.slice(0, 4).map((item) => (
                        <button
                          key={item.id}
                          className={
                            "agenda-event " +
                            (item.status === "cancelled" ? "agenda-cancelled" : "")
                          }
                          onClick={() => setSelected(item)}
                          title={
                            item.customer_name +
                            " · " +
                            item.service_name +
                            " · " +
                            statuses[item.status]
                          }
                        >
                          <strong>
                            {time(item.starts_at)} · {item.customer_name}
                          </strong>
                          <span>
                            {item.service_name} · {item.professional_name}
                          </span>
                          <small>{statuses[item.status]}</small>
                        </button>
                      ))}
                      {appointments.length > 4 && (
                        <button className="more-events" onClick={() => setDayFilter(key)}>
                          Ver todos ({appointments.length})
                        </button>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
            <div className="calendar-footer">
              {data.appointments.length} agendamento(s) carregado(s) · Inclui cancelamentos ·{" "}
              {data.next_cursor ? "Há mais reservas para carregar" : "Período completo"}
            </div>
          </section>
          <section className="agenda-list" aria-label="Lista de agendamentos">
            <div className="agenda-actions">
              <h2>Agendamentos do período</h2>
              <label>
                Filtrar por dia{" "}
                <input
                  type="date"
                  value={dayFilter}
                  min={data.from_date}
                  max={dateKey(addDays(parseDate(data.until_date), -1))}
                  onChange={(event) => setDayFilter(event.target.value)}
                />
              </label>
              {dayFilter && (
                <button className="button button-secondary" onClick={() => setDayFilter("")}>
                  Ver todo o período
                </button>
              )}
            </div>
            {!visible.length && (
              <p>
                Nenhum agendamento {dayFilter ? "neste dia" : "no período exibido"}
                {data.next_cursor ? " entre as reservas carregadas" : ""}.
              </p>
            )}
            <div className="agenda-list-grid">
              {visible.map((item) => (
                <button
                  className="agenda-list-item"
                  key={item.id}
                  onClick={() => setSelected(item)}
                >
                  <strong>{item.customer_name}</strong>
                  <span>
                    {formatDate(agendaDate(item.starts_at, data.shop_timezone), {
                      day: "2-digit",
                      month: "2-digit",
                    })}{" "}
                    · {time(item.starts_at)}–{time(item.ends_at)}
                  </span>
                  <span>
                    {item.service_name} · {item.professional_name}
                  </span>
                  <small>
                    {statuses[item.status]} · {money(item.price_cents)}
                  </small>
                </button>
              ))}
            </div>
            {data.next_cursor && (
              <button
                className="button button-secondary"
                disabled={loading}
                onClick={() => {
                  setMonth(data.month);
                  setAfter(data.next_cursor!);
                  setRevision((value) => value + 1);
                }}
              >
                Carregar mais agendamentos
              </button>
            )}
          </section>
        </>
      )}
      <Modal
        open={!!selected}
        onClose={() => setSelected(null)}
        title="Detalhes do agendamento"
        subtitle={shop.name}
      >
        {selected && data && (
          <div className="booking-receipt">
            <dl className="receipt-details">
              <div>
                <dt>Cliente</dt>
                <dd>{selected.customer_name}</dd>
              </div>
              <div>
                <dt>Telefone</dt>
                <dd>{selected.customer_phone}</dd>
              </div>
              <div>
                <dt>Serviço</dt>
                <dd>{selected.service_name}</dd>
              </div>
              <div>
                <dt>Barbeiro</dt>
                <dd>{selected.professional_name}</dd>
              </div>
              <div>
                <dt>Data</dt>
                <dd>
                  {formatDate(agendaDate(selected.starts_at, data.shop_timezone), {
                    day: "2-digit",
                    month: "long",
                    year: "numeric",
                  })}
                </dd>
              </div>
              <div>
                <dt>Horário reservado</dt>
                <dd>
                  {time(selected.starts_at)}–{time(selected.ends_at)} ({data.shop_timezone})
                </dd>
              </div>
              <div>
                <dt>Duração do serviço</dt>
                <dd>{selected.duration_minutes} minutos</dd>
              </div>
              <div>
                <dt>Valor na reserva</dt>
                <dd>{money(selected.price_cents)}</dd>
              </div>
              <div>
                <dt>Situação</dt>
                <dd>{statuses[selected.status]}</dd>
              </div>
            </dl>
            <p>
              Visualização administrativa. Alterações de status pelo painel ainda não estão
              disponíveis.
            </p>
          </div>
        )}
      </Modal>
    </>
  );
}
