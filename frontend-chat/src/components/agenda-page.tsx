"use client";
import { useState } from "react";
import Link from "next/link";
import { CalendarDays, ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { addDays, dateKey, formatDate, startOfWeek } from "@/lib/model";
export function AgendaPage() {
  const [anchor, setAnchor] = useState(() => new Date());
  const first = new Date(anchor.getFullYear(), anchor.getMonth(), 1);
  const days = Array.from({ length: 42 }, (_, i) => addDays(startOfWeek(first), i));
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">SEU ESPAÇO DE GESTÃO</div>
          <h1>
            Sua agenda, em dia<span className="heading-dot">.</span>
          </h1>
          <p>A estrutura do calendário está pronta para receber os agendamentos.</p>
        </div>
        <button
          className="button button-primary"
          disabled
          title="A API ainda não oferece criação de agendamentos"
        >
          <Plus size={18} />
          Novo agendamento
        </button>
      </div>
      <div className="inline-hint integration-note">
        <strong>Agenda ainda não integrada.</strong> O backend não oferece agendamentos,
        profissionais ou disponibilidade. Nenhum horário exibido abaixo representa disponibilidade
        para reserva. <Link href="/admin/servicos">Acessar serviços →</Link>
      </div>
      <section className="calendar-card" aria-label="Calendário sem dados de agendamentos">
        <div className="calendar-toolbar">
          <div className="calendar-date-nav">
            <CalendarDays size={20} />
            <h2>{formatDate(anchor, { month: "long", year: "numeric" })}</h2>
            <button
              className="icon-button"
              aria-label="Mês anterior"
              onClick={() => setAnchor(new Date(anchor.getFullYear(), anchor.getMonth() - 1, 1))}
            >
              <ChevronLeft size={18} />
            </button>
            <button
              className="icon-button"
              aria-label="Próximo mês"
              onClick={() => setAnchor(new Date(anchor.getFullYear(), anchor.getMonth() + 1, 1))}
            >
              <ChevronRight size={18} />
            </button>
            <button className="today-button" onClick={() => setAnchor(new Date())}>
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
            {days.map((day) => (
              <div
                className={
                  "month-cell " + (day.getMonth() !== anchor.getMonth() ? "outside-month" : "")
                }
                key={dateKey(day)}
              >
                <span
                  className={
                    "month-day " + (dateKey(day) === dateKey(new Date()) ? "is-today" : "")
                  }
                >
                  {day.getDate()}
                </span>
              </div>
            ))}
          </div>
        </div>
        <div className="calendar-footer">Aguardando API de agendamentos · Sem dados simulados</div>
      </section>
    </>
  );
}
