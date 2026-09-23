export type Category = "Cabelo" | "Barba" | "Combos" | "Cuidados";
export type Service = {
  id: string;
  name: string;
  description: string;
  duration: number;
  price: number;
  category: Category;
  active: boolean;
};
export type AppointmentStatus = "confirmed" | "pending" | "completed" | "cancelled";
export type Appointment = {
  id: string;
  customer: string;
  phone: string;
  serviceId: string;
  serviceName: string;
  duration: number;
  price: number;
  professional: string;
  date: string;
  time: string;
  status: AppointmentStatus;
  source: "chat" | "manual";
  notes: string;
};
export type AppointmentInput = Pick<
  Appointment,
  "customer" | "phone" | "serviceId" | "professional" | "date" | "time" | "notes" | "source"
>;
export type DemoData = { version: 1; services: Service[]; appointments: Appointment[] };
export const professionals = [
  {
    id: "felipe",
    name: "Felipe Palma",
    initials: "FP",
    role: "Barbeiro especialista",
    color: "blue",
  },
  {
    id: "rafael",
    name: "Rafael Costa",
    initials: "RC",
    role: "Barbeiro especialista",
    color: "sage",
  },
];
export const statuses: Record<AppointmentStatus, { label: string; className: string }> = {
  confirmed: { label: "Confirmado", className: "status-confirmed" },
  pending: { label: "Pendente", className: "status-pending" },
  completed: { label: "Concluído", className: "status-completed" },
  cancelled: { label: "Cancelado", className: "status-cancelled" },
};
export function dateKey(date: Date) {
  return [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, "0"),
    String(date.getDate()).padStart(2, "0"),
  ].join("-");
}
export function parseDate(value: string) {
  return new Date(value + "T12:00:00");
}
export function addDays(date: Date, count: number) {
  const next = new Date(date);
  next.setDate(next.getDate() + count);
  return next;
}
export function startOfWeek(date: Date) {
  return addDays(date, -((date.getDay() + 6) % 7));
}
export function formatDate(date: Date | string, options: Intl.DateTimeFormatOptions) {
  const value = typeof date === "string" ? parseDate(date) : date;
  return Number.isNaN(value.getTime())
    ? "Selecione uma data"
    : new Intl.DateTimeFormat("pt-BR", options).format(value);
}
export function money(cents: number) {
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
}
export function minutes(time: string) {
  const [h, m] = time.split(":").map(Number);
  return h * 60 + m;
}
export function timeString(value: number) {
  return (
    String(Math.floor(value / 60)).padStart(2, "0") + ":" + String(value % 60).padStart(2, "0")
  );
}
export function initials(value: string) {
  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}
export function isAvailable(
  appointments: Appointment[],
  date: string,
  time: string,
  duration: number,
  professional: string,
  exceptId?: string,
) {
  if (
    !/^\d{4}-\d{2}-\d{2}$/.test(date) ||
    Number.isNaN(parseDate(date).getTime()) ||
    dateKey(parseDate(date)) !== date ||
    !/^([01]\d|2[0-3]):[0-5]\d$/.test(time)
  )
    return false;
  const begin = minutes(time);
  return (
    begin >= 9 * 60 &&
    begin + duration <= 19 * 60 &&
    parseDate(date).getDay() !== 0 &&
    !appointments.some(
      (a) =>
        a.id !== exceptId &&
        a.date === date &&
        a.professional === professional &&
        a.status !== "cancelled" &&
        begin < minutes(a.time) + a.duration &&
        begin + duration > minutes(a.time),
    )
  );
}
export function availableTimes(
  appointments: Appointment[],
  date: string,
  duration: number,
  professional: string,
  exceptId?: string,
) {
  if (!date || date < dateKey(new Date())) return [];
  return Array.from({ length: 20 }, (_, i) => timeString(540 + i * 30)).filter((time) => {
    const past =
      date === dateKey(new Date()) &&
      minutes(time) <= new Date().getHours() * 60 + new Date().getMinutes();
    return !past && isAvailable(appointments, date, time, duration, professional, exceptId);
  });
}
export function makeSeed(): DemoData {
  const services: Service[] = [
    {
      id: "corte",
      name: "Corte clássico",
      description: "Seu estilo, com acabamento na medida.",
      duration: 30,
      price: 4500,
      category: "Cabelo",
      active: true,
    },
    {
      id: "degrade",
      name: "Corte degradê",
      description: "Transição precisa e finalização com estilo.",
      duration: 45,
      price: 5500,
      category: "Cabelo",
      active: true,
    },
    {
      id: "barba",
      name: "Barba completa",
      description: "Desenho, toalha quente e cuidado especial.",
      duration: 30,
      price: 3500,
      category: "Barba",
      active: true,
    },
    {
      id: "combo",
      name: "Cabelo + barba",
      description: "O cuidado completo para renovar o visual.",
      duration: 60,
      price: 7500,
      category: "Combos",
      active: true,
    },
    {
      id: "sobrancelha",
      name: "Design de sobrancelha",
      description: "Os pequenos detalhes fazem a diferença.",
      duration: 15,
      price: 1500,
      category: "Cuidados",
      active: true,
    },
    {
      id: "hidratacao",
      name: "Hidratação capilar",
      description: "Maciez, saúde e uma dose extra de cuidado.",
      duration: 30,
      price: 3000,
      category: "Cuidados",
      active: false,
    },
  ];
  const monday = startOfWeek(new Date());
  const customers = [
    "Lucas Mendes",
    "Pedro Oliveira",
    "Gabriel Santos",
    "André Lima",
    "João Almeida",
    "Bruno Ferreira",
    "Rafael Souza",
    "Gustavo Costa",
    "Matheus Silva",
    "Thiago Ribeiro",
    "Diego Martins",
    "Henrique Rocha",
    "Caio Nunes",
    "Daniel Alves",
    "Vinícius Dias",
    "Eduardo Reis",
    "Felipe Gomes",
    "Leonardo Castro",
  ];
  const slots = [
    [0, "09:00"],
    [0, "11:00"],
    [0, "14:00"],
    [1, "09:30"],
    [1, "11:00"],
    [1, "14:30"],
    [1, "16:00"],
    [2, "09:00"],
    [2, "10:30"],
    [2, "13:30"],
    [2, "16:00"],
    [3, "10:00"],
    [3, "14:00"],
    [3, "16:30"],
    [4, "09:30"],
    [4, "13:00"],
    [5, "10:00"],
    [5, "14:00"],
  ] as const;
  const today = dateKey(new Date());
  const appointments: Appointment[] = slots.map(([day, time], i) => {
    const s = services[i % 4];
    const date = dateKey(addDays(monday, day));
    return {
      id: "demo-" + i,
      customer: customers[i],
      phone: "(11) 99999-0000",
      serviceId: s.id,
      serviceName: s.name,
      duration: s.duration,
      price: s.price,
      professional: professionals[i % 2].id,
      date,
      time,
      status: date < today ? "completed" : i % 4 === 1 ? "pending" : "confirmed",
      source: i % 3 === 0 ? "manual" : "chat",
      notes: "",
    };
  });
  return { version: 1, services, appointments };
}
export function validData(value: unknown): value is DemoData {
  if (!value || typeof value !== "object") return false;
  const data = value as DemoData;
  return (
    data.version === 1 &&
    Array.isArray(data.services) &&
    Array.isArray(data.appointments) &&
    data.services.every(
      (s) =>
        s &&
        typeof s.id === "string" &&
        typeof s.name === "string" &&
        typeof s.description === "string" &&
        Number.isInteger(s.duration) &&
        s.duration > 0 &&
        Number.isInteger(s.price) &&
        s.price >= 0 &&
        ["Cabelo", "Barba", "Combos", "Cuidados"].includes(s.category) &&
        typeof s.active === "boolean",
    ) &&
    data.appointments.every(
      (a) =>
        a &&
        typeof a.id === "string" &&
        typeof a.customer === "string" &&
        typeof a.phone === "string" &&
        typeof a.serviceName === "string" &&
        typeof a.serviceId === "string" &&
        /^\d{4}-\d{2}-\d{2}$/.test(a.date) &&
        /^\d{2}:\d{2}$/.test(a.time) &&
        Number.isInteger(a.duration) &&
        a.duration > 0 &&
        Number.isInteger(a.price) &&
        a.price >= 0 &&
        professionals.some((p) => p.id === a.professional) &&
        Object.hasOwn(statuses, a.status) &&
        typeof a.notes === "string",
    )
  );
}
