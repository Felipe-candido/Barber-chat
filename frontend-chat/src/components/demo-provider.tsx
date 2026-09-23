"use client";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { Check, X, AlertCircle } from "lucide-react";
import {
  Appointment,
  AppointmentInput,
  DemoData,
  Service,
  dateKey,
  makeSeed,
  validData,
  isAvailable,
  professionals,
} from "@/lib/model";

const STORAGE_KEY = "palma-barbearia-demo-v1";
type Result = { error?: string; appointment?: Appointment };
type DemoContextValue = {
  data: DemoData;
  ready: boolean;
  notify: (text: string) => void;
  saveService: (service: Service) => void;
  deleteService: (id: string) => void;
  book: (input: AppointmentInput, id?: string) => Result;
  changeStatus: (id: string, status: Appointment["status"]) => void;
};
const DemoContext = createContext<DemoContextValue | null>(null);
const empty: DemoData = { version: 1, services: [], appointments: [] };
export function DemoProvider({ children }: { children: React.ReactNode }) {
  const [data, setData] = useState<DemoData>(empty);
  const [ready, setReady] = useState(false);
  const [toast, setToast] = useState("");
  const [storageWarning, setStorageWarning] = useState(false);
  const current = useRef(data);
  useEffect(() => {
    let initial = makeSeed();
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        const parsed: unknown = JSON.parse(saved);
        if (validData(parsed)) initial = parsed;
      }
    } catch {
      setStorageWarning(true);
    }
    current.current = initial;
    setData(initial);
    setReady(true);
    function synchronize(event: StorageEvent) {
      if (event.key !== STORAGE_KEY || !event.newValue) return;
      try {
        const parsed: unknown = JSON.parse(event.newValue);
        if (validData(parsed)) {
          current.current = parsed;
          setData(parsed);
        }
      } catch {
        /* Ignore malformed data from another tab. */
      }
    }
    window.addEventListener("storage", synchronize);
    return () => window.removeEventListener("storage", synchronize);
  }, []);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(""), 4200);
    return () => clearTimeout(timer);
  }, [toast]);
  const notify = useCallback((message: string) => setToast(message), []);
  function persist(next: DemoData) {
    current.current = next;
    setData(next);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    } catch {
      setStorageWarning(true);
    }
  }
  function saveService(service: Service) {
    const state = current.current;
    persist({
      ...state,
      services: state.services.some((s) => s.id === service.id)
        ? state.services.map((s) => (s.id === service.id ? service : s))
        : [...state.services, service],
    });
  }
  function deleteService(id: string) {
    persist({ ...current.current, services: current.current.services.filter((s) => s.id !== id) });
    notify("Serviço excluído. O histórico de agendamentos foi preservado.");
  }
  function book(input: AppointmentInput, id?: string): Result {
    const state = current.current;
    const existing = state.appointments.find((a) => a.id === id);
    const service = state.services.find((s) => s.id === input.serviceId && s.active);
    if (!service) return { error: "Esse serviço não está mais disponível. Escolha outro serviço." };
    if (input.customer.trim().length < 3) return { error: "Informe o nome completo do cliente." };
    if (![10, 11].includes(input.phone.replace(/\D/g, "").length))
      return { error: "Informe um telefone válido com DDD." };
    if (!professionals.some((p) => p.id === input.professional))
      return { error: "Escolha um profissional." };
    if (input.date < dateKey(new Date())) return { error: "Escolha uma data a partir de hoje." };
    if (
      !isAvailable(
        state.appointments,
        input.date,
        input.time,
        service.duration,
        input.professional,
        id,
      )
    )
      return { error: "Esse horário não está disponível. Escolha outro horário." };
    const now = new Date();
    if (
      input.date === dateKey(now) &&
      input.time <=
        String(now.getHours()).padStart(2, "0") + ":" + String(now.getMinutes()).padStart(2, "0")
    )
      return { error: "Esse horário já passou. Escolha um horário futuro." };
    const appointment: Appointment = {
      ...input,
      id: id ?? crypto.randomUUID(),
      customer: input.customer.trim(),
      serviceName: service.name,
      duration: service.duration,
      price: service.price,
      status: existing?.status === "completed" ? "completed" : "confirmed",
    };
    persist({
      ...state,
      appointments: id
        ? state.appointments.map((a) => (a.id === id ? appointment : a))
        : [...state.appointments, appointment],
    });
    return { appointment };
  }
  function changeStatus(id: string, status: Appointment["status"]) {
    persist({
      ...current.current,
      appointments: current.current.appointments.map((a) => (a.id === id ? { ...a, status } : a)),
    });
    notify(status === "cancelled" ? "Agendamento cancelado." : "Agendamento atualizado.");
  }
  return (
    <DemoContext.Provider
      value={{ data, ready, notify, saveService, deleteService, book, changeStatus }}
    >
      {children}
      {storageWarning && (
        <div className="storage-notice" role="status">
          <AlertCircle size={16} /> As alterações ficarão apenas nesta sessão.
        </div>
      )}
      {toast && (
        <div className="toast" role="status">
          <span className="toast-icon">
            <Check size={17} />
          </span>
          {toast}
          <button aria-label="Fechar aviso" onClick={() => setToast("")}>
            <X size={16} />
          </button>
        </div>
      )}
    </DemoContext.Provider>
  );
}
export function useDemo() {
  const context = useContext(DemoContext);
  if (!context) throw new Error("DemoProvider is required");
  return context;
}
