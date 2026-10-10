import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

export type Professional = { id: string; name: string };
export type Professionals = {
  shop_name: string;
  shop_timezone: string;
  today: string;
  last_booking_date: string;
  professionals: Professional[];
};
export type Slot = { starts_at: string; ends_at: string };
export type Availability = { shop_timezone: string; slots: Slot[] };
export type BookingInput = {
  service_id: string;
  professional_id: string;
  starts_at: string;
  customer: { first_name: string; last_name: string; phone: string; phone_country: string };
};
export type Appointment = {
  id: string;
  shop_name: string;
  status: "scheduled" | "confirmed" | "cancelled" | "completed" | "no_show";
  service_name: string;
  duration_minutes: number;
  price_cents: number;
  currency: "BRL";
  professional_id: string;
  professional_name: string;
  starts_at: string;
  ends_at: string;
  shop_timezone: string;
};
export type BookingResult = { appointment: Appointment; code: string; replayed: boolean };
export type AppointmentDetails = { appointment: Appointment; can_cancel: boolean };

function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "A resposta de agendamento da API não é compatível. Não faça outra reserva antes de conferir esta tentativa.",
  );
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid();
  return value as Record<string, unknown>;
}
function text(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}
function instant(value: unknown): value is string {
  return text(value) && /(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value));
}
function timezone(value: unknown): value is string {
  if (!text(value)) return false;
  try {
    new Intl.DateTimeFormat("pt-BR", { timeZone: value }).format();
    return true;
  } catch {
    return false;
  }
}
function interval(value: unknown): Slot {
  const item = record(value);
  if (
    !instant(item.starts_at) ||
    !instant(item.ends_at) ||
    Date.parse(item.ends_at) <= Date.parse(item.starts_at)
  )
    invalid();
  return { starts_at: item.starts_at, ends_at: item.ends_at };
}
function appointment(value: unknown): Appointment {
  const item = record(value);
  interval(item);
  if (
    !text(item.id) ||
    !text(item.shop_name) ||
    !text(item.service_name) ||
    !text(item.professional_id) ||
    !text(item.professional_name) ||
    !timezone(item.shop_timezone) ||
    !["scheduled", "confirmed", "cancelled", "completed", "no_show"].includes(
      String(item.status),
    ) ||
    !Number.isInteger(item.duration_minutes) ||
    Number(item.duration_minutes) < 1 ||
    Number(item.duration_minutes) > 1440 ||
    !Number.isSafeInteger(item.price_cents) ||
    Number(item.price_cents) < 0 ||
    item.currency !== "BRL"
  )
    invalid();
  return item as Appointment;
}
function details(value: unknown): AppointmentDetails {
  const item = record(value);
  if (typeof item.can_cancel !== "boolean") invalid();
  return { appointment: appointment(item.appointment), can_cancel: item.can_cancel };
}
function base(slug: string) {
  return "/api/v1/public/shops/" + shopPathSegment(slug);
}

// This key is both an idempotency identifier and a secret retry credential.
// Never replace it while an earlier submission's outcome is uncertain.
export function newBookingKey(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(32)), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}
export async function listProfessionals(
  slug: string,
  serviceId: string,
  signal?: AbortSignal,
): Promise<Professionals> {
  const result = record(
    await requestJSON(
      base(slug) + "/professionals?" + new URLSearchParams({ service_id: serviceId }),
      { signal },
    ),
  );
  if (
    !text(result.shop_name) ||
    !timezone(result.shop_timezone) ||
    !text(result.today) ||
    !/^\d{4}-\d{2}-\d{2}$/.test(result.today) ||
    !text(result.last_booking_date) ||
    !/^\d{4}-\d{2}-\d{2}$/.test(result.last_booking_date) ||
    !Array.isArray(result.professionals)
  )
    invalid();
  const professionals = result.professionals.map((value) => {
    const item = record(value);
    if (!text(item.id) || !text(item.name)) invalid();
    return { id: item.id, name: item.name };
  });
  return {
    shop_name: result.shop_name,
    shop_timezone: result.shop_timezone,
    today: result.today,
    last_booking_date: result.last_booking_date,
    professionals,
  };
}
export async function listAvailability(
  slug: string,
  serviceId: string,
  professionalId: string,
  date: string,
  signal?: AbortSignal,
): Promise<Availability> {
  const query = new URLSearchParams({
    service_id: serviceId,
    professional_id: professionalId,
    date,
  });
  const result = record(await requestJSON(base(slug) + "/availability?" + query, { signal }));
  if (!timezone(result.shop_timezone) || !Array.isArray(result.slots)) invalid();
  return { shop_timezone: result.shop_timezone, slots: result.slots.map(interval) };
}
export async function createAppointment(
  slug: string,
  input: BookingInput,
  key: string,
): Promise<BookingResult> {
  return createBooking(slug, input, key, false);
}
export async function createStaffAppointment(
  slug: string,
  input: BookingInput,
  key: string,
): Promise<BookingResult> {
  return createBooking(slug, input, key, true);
}
async function createBooking(
  slug: string,
  input: BookingInput,
  key: string,
  staff: boolean,
): Promise<BookingResult> {
  const result = record(
    await requestJSON(
      (staff ? "/api/v1/admin/shops/" + shopPathSegment(slug) : base(slug)) + "/appointments",
      {
        method: "POST",
        body: input,
        idempotencyKey: key,
        authenticated: staff,
      },
    ),
  );
  if (
    typeof result.code !== "string" ||
    !/^[A-Z2-7]{8}(?:-[A-Z2-7]{8}){3}$/.test(result.code) ||
    typeof result.replayed !== "boolean"
  )
    invalid();
  return {
    appointment: appointment(result.appointment),
    code: result.code,
    replayed: result.replayed,
  };
}
export async function lookupAppointment(
  slug: string,
  code: string,
  signal?: AbortSignal,
): Promise<AppointmentDetails> {
  return details(
    await requestJSON(base(slug) + "/appointments/lookup", {
      method: "POST",
      body: { code },
      signal,
    }),
  );
}
export async function cancelAppointment(slug: string, code: string): Promise<AppointmentDetails> {
  return details(
    await requestJSON(base(slug) + "/appointments/cancel", { method: "POST", body: { code } }),
  );
}
export function bookingDate(value: string, zone: string): string {
  return new Intl.DateTimeFormat("pt-BR", { timeZone: zone, dateStyle: "full" }).format(
    new Date(value),
  );
}
export function bookingTime(value: string, zone: string): string {
  return new Intl.DateTimeFormat("pt-BR", {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}
