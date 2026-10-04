import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import type { Appointment } from "./booking";

export type AdminAppointment = Appointment & { customer_name: string; customer_phone: string };
export type Agenda = {
  month: string;
  today: string;
  from_date: string;
  until_date: string;
  shop_timezone: string;
  appointments: AdminAppointment[];
  next_cursor: string | null;
};

function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou uma agenda incompatível. Atualize e tente novamente.",
  );
}
function text(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}
function instant(value: unknown): value is string {
  return text(value) && /(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value));
}
function appointment(value: unknown): AdminAppointment {
  const item = value as AdminAppointment;
  if (
    !item ||
    typeof item !== "object" ||
    !text(item.id) ||
    !text(item.customer_name) ||
    !text(item.customer_phone) ||
    !text(item.service_name) ||
    !text(item.professional_id) ||
    !text(item.professional_name) ||
    !text(item.shop_name) ||
    !text(item.shop_timezone) ||
    !instant(item.starts_at) ||
    !instant(item.ends_at) ||
    Date.parse(item.ends_at) <= Date.parse(item.starts_at) ||
    !["scheduled", "confirmed", "cancelled", "completed", "no_show"].includes(item.status) ||
    !Number.isInteger(item.duration_minutes) ||
    item.duration_minutes < 1 ||
    item.duration_minutes > 1440 ||
    !Number.isSafeInteger(item.price_cents) ||
    item.price_cents < 0 ||
    item.currency !== "BRL"
  )
    invalid();
  return item;
}

export async function listAgenda(
  slug: string,
  month?: string,
  after?: string,
  signal?: AbortSignal,
): Promise<Agenda> {
  const query = new URLSearchParams();
  if (month) query.set("month", month);
  if (after) query.set("after", after);
  const result = (await requestJSON(
    "/api/v1/admin/shops/" +
      shopPathSegment(slug) +
      "/appointments" +
      (query.size ? "?" + query : ""),
    { authenticated: true, signal },
  )) as Agenda;
  const date = /^\d{4}-\d{2}-\d{2}$/;
  if (
    !result ||
    typeof result !== "object" ||
    !/^\d{4}-\d{2}$/.test(result.month) ||
    (month && month !== result.month) ||
    !date.test(result.today) ||
    !date.test(result.from_date) ||
    !date.test(result.until_date) ||
    !text(result.shop_timezone) ||
    !Array.isArray(result.appointments) ||
    result.appointments.length > 100 ||
    (result.next_cursor !== null &&
      (!text(result.next_cursor) ||
        result.next_cursor.length > 160 ||
        result.appointments.length === 0))
  )
    invalid();
  try {
    new Intl.DateTimeFormat("pt-BR", { timeZone: result.shop_timezone }).format();
  } catch {
    invalid();
  }
  const items = result.appointments.map(appointment);
  if (new Set(items.map((item) => item.id)).size !== items.length) invalid();
  return { ...result, appointments: items };
}

// Group instants by the shop's civil day, not UTC or the browser's timezone.
export function agendaDate(value: string, zone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(value));
  const part = (name: string) => parts.find((item) => item.type === name)?.value;
  return `${part("year")}-${part("month")}-${part("day")}`;
}
