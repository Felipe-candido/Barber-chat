"use client";
import { useState } from "react";
import {
  AlertCircle,
  CalendarDays,
  Clock3,
  Phone,
  Scissors,
  UserRound,
  Pencil,
  Check,
  X,
} from "lucide-react";
import {
  Appointment,
  availableTimes,
  dateKey,
  formatDate,
  money,
  professionals,
  statuses,
} from "@/lib/model";
import { useDemo } from "./demo-provider";
import { Modal } from "./ui";

export function AppointmentForm({
  appointment,
  date,
  time,
  onClose,
  onSaved,
}: {
  appointment?: Appointment;
  date?: string;
  time?: string;
  onClose: () => void;
  onSaved?: (appointment: Appointment) => void;
}) {
  const { data, book, notify } = useDemo();
  const active = data.services.filter((s) => s.active);
  const [serviceId, setService] = useState(appointment?.serviceId ?? active[0]?.id ?? "");
  const [professional, setProfessional] = useState(appointment?.professional ?? "felipe");
  const [day, setDay] = useState(appointment?.date ?? date ?? dateKey(new Date()));
  const [selectedTime, setTime] = useState(appointment?.time ?? time ?? "");
  const [error, setError] = useState("");
  const service = active.find((s) => s.id === serviceId);
  const times = availableTimes(
    data.appointments,
    day,
    service?.duration ?? 30,
    professional,
    appointment?.id,
  );
  return (
    <Modal
      open
      onClose={onClose}
      title={appointment ? "Editar agendamento" : "Novo agendamento"}
      subtitle="Um horário reservado para cuidar de alguém."
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          const form = new FormData(event.currentTarget);
          const result = book(
            {
              customer: String(form.get("customer")),
              phone: String(form.get("phone")),
              serviceId,
              professional,
              date: day,
              time: selectedTime,
              notes: String(form.get("notes")),
              source: appointment?.source ?? "manual",
            },
            appointment?.id,
          );
          if (result.error) setError(result.error);
          else {
            if (result.appointment) onSaved?.(result.appointment);
            notify(appointment ? "Agendamento atualizado!" : "Agendamento criado com sucesso!");
            onClose();
          }
        }}
        className="form-stack"
      >
        <div className="field-row">
          <label>
            Nome do cliente
            <input
              name="customer"
              required
              minLength={3}
              maxLength={100}
              defaultValue={appointment?.customer}
              placeholder="Nome e sobrenome"
              autoComplete="name"
            />
          </label>
          <label>
            Telefone com DDD
            <input
              name="phone"
              required
              type="tel"
              defaultValue={appointment?.phone}
              placeholder="(11) 99999-9999"
              autoComplete="tel"
            />
          </label>
        </div>
        <label>
          Serviço
          <select
            value={serviceId}
            onChange={(e) => {
              setService(e.target.value);
              setTime("");
            }}
            required
          >
            <option value="" disabled>
              Selecione um serviço
            </option>
            {active.map((s) => (
              <option value={s.id} key={s.id}>
                {s.name} · {s.duration} min · {money(s.price)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Profissional
          <select
            value={professional}
            onChange={(e) => {
              setProfessional(e.target.value);
              setTime("");
            }}
          >
            {professionals.map((p) => (
              <option value={p.id} key={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <div className="field-row">
          <label>
            Data
            <input
              type="date"
              required
              min={dateKey(new Date())}
              value={day}
              onChange={(e) => {
                setDay(e.target.value);
                setTime("");
              }}
            />
          </label>
          <label>
            Horário
            <select
              required
              value={times.includes(selectedTime) ? selectedTime : ""}
              onChange={(e) => setTime(e.target.value)}
            >
              <option value="">Selecione</option>
              {times.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
        </div>
        {!times.length && (
          <p className="inline-hint">
            Nenhum horário livre nessa data. Experimente outro dia ou profissional.
          </p>
        )}
        <label>
          Observações <span className="optional">opcional</span>
          <textarea
            name="notes"
            rows={2}
            maxLength={500}
            defaultValue={appointment?.notes}
            placeholder="Algum cuidado especial?"
          />
        </label>
        {service && (
          <div className="form-summary">
            <span>
              <Clock3 size={16} />
              {service.duration} minutos
            </span>
            <strong>{money(service.price)}</strong>
          </div>
        )}
        {error && (
          <div className="form-error" role="alert">
            <AlertCircle size={16} />
            {error}
          </div>
        )}
        {!active.length && (
          <p className="form-error">Cadastre um serviço ativo antes de agendar.</p>
        )}
        <div className="modal-actions">
          <button type="button" className="button button-secondary" onClick={onClose}>
            Voltar
          </button>
          <button
            className="button button-primary"
            disabled={!active.length || !times.includes(selectedTime)}
          >
            {appointment ? "Salvar alterações" : "Confirmar agendamento"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
export function AppointmentDetails({
  appointment: a,
  onClose,
  onEdit,
}: {
  appointment: Appointment;
  onClose: () => void;
  onEdit: () => void;
}) {
  const { changeStatus } = useDemo();
  const [cancel, setCancel] = useState(false);
  const status = statuses[a.status];
  const editable = a.date >= dateKey(new Date()) && !["cancelled", "completed"].includes(a.status);
  function update(status: Appointment["status"]) {
    changeStatus(a.id, status);
    onClose();
  }
  return (
    <Modal
      open
      onClose={onClose}
      title={cancel ? "Cancelar agendamento?" : a.customer}
      subtitle={cancel ? "O horário ficará disponível novamente." : "Tudo pronto para receber bem."}
    >
      {cancel ? (
        <>
          <p className="dialog-copy">
            Deseja cancelar o horário de <strong>{a.customer}</strong> em{" "}
            {formatDate(a.date, { day: "numeric", month: "long" })}, às {a.time}?
          </p>
          <div className="modal-actions">
            <button className="button button-secondary" onClick={() => setCancel(false)}>
              Manter agendamento
            </button>
            <button className="button button-danger" onClick={() => update("cancelled")}>
              Sim, cancelar
            </button>
          </div>
        </>
      ) : (
        <>
          <span className={"status-badge " + status.className}>{status.label}</span>
          <div className="appointment-detail-grid">
            <div>
              <Scissors />
              <span>
                Serviço<strong>{a.serviceName}</strong>
              </span>
            </div>
            <div>
              <CalendarDays />
              <span>
                Data<strong>{formatDate(a.date, { day: "numeric", month: "long" })}</strong>
              </span>
            </div>
            <div>
              <Clock3 />
              <span>
                Horário
                <strong>
                  {a.time} · {a.duration} min
                </strong>
              </span>
            </div>
            <div>
              <UserRound />
              <span>
                Profissional
                <strong>{professionals.find((p) => p.id === a.professional)?.name}</strong>
              </span>
            </div>
            <div>
              <Phone />
              <span>
                Contato<strong>{a.phone}</strong>
              </span>
            </div>
            <div>
              <span>
                Valor<strong>{money(a.price)}</strong>
              </span>
            </div>
          </div>
          {a.notes && <p className="detail-note">{a.notes}</p>}
          <p className="muted text-xs">
            Agendado {a.source === "chat" ? "pelo chat" : "pelo painel"} · Dados de demonstração
          </p>
          <div className="detail-actions">
            {editable && (
              <button className="button button-secondary" onClick={onEdit}>
                <Pencil size={15} />
                Editar
              </button>
            )}
            {a.status === "pending" && (
              <button className="button button-primary" onClick={() => update("confirmed")}>
                <Check size={15} />
                Confirmar
              </button>
            )}
            {a.status === "confirmed" && a.date <= dateKey(new Date()) && (
              <button className="button button-primary" onClick={() => update("completed")}>
                <Check size={15} />
                Concluir
              </button>
            )}
            {!["cancelled", "completed"].includes(a.status) && (
              <button className="button button-quiet danger-text" onClick={() => setCancel(true)}>
                <X size={15} />
                Cancelar
              </button>
            )}
          </div>
        </>
      )}
    </Modal>
  );
}
