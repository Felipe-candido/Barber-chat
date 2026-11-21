import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { isID } from "./identity";
import type { StaffProfessional } from "./professionals";

export type WorkingPeriod = { weekday: number; opens_at_minute: number; closes_at_minute: number };
export type ProfessionalDetails = StaffProfessional & {
  revision: string;
  schedule: { interval_minutes: number; buffer_minutes: number; working_hours: WorkingPeriod[] };
};
export type ProfessionalEditInput = Omit<ProfessionalDetails, "id">;
function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "Não conseguimos confirmar o cadastro do barbeiro. Atualize os dados antes de tentar novamente.",
  );
}
function details(value: unknown, id: string): ProfessionalDetails {
  const p = value as ProfessionalDetails;
  if (
    !p ||
    p.id !== id ||
    !isID(p.id) ||
    typeof p.name !== "string" ||
    !p.name.trim() ||
    typeof p.active !== "boolean" ||
    !Array.isArray(p.service_ids) ||
    p.service_ids.length > 100 ||
    p.service_ids.some((v) => !isID(v)) ||
    new Set(p.service_ids).size !== p.service_ids.length ||
    typeof p.revision !== "string" ||
    !/^[a-f0-9]{64}$/.test(p.revision) ||
    !p.schedule
  )
    invalid();
  const s = p.schedule;
  if (
    !Number.isInteger(s.interval_minutes) ||
    s.interval_minutes < 1 ||
    s.interval_minutes > 1440 ||
    !Number.isInteger(s.buffer_minutes) ||
    s.buffer_minutes < 0 ||
    s.buffer_minutes > 1440 ||
    !Array.isArray(s.working_hours) ||
    s.working_hours.length > 28
  )
    invalid();
  if (s.working_hours.some((w) => !w)) invalid();
  const hours = [...s.working_hours].sort(
    (a, b) => a.weekday - b.weekday || a.opens_at_minute - b.opens_at_minute,
  );
  for (let i = 0; i < hours.length; i++) {
    const w = hours[i];
    if (
      !w ||
      !Number.isInteger(w.weekday) ||
      w.weekday < 0 ||
      w.weekday > 6 ||
      !Number.isInteger(w.opens_at_minute) ||
      !Number.isInteger(w.closes_at_minute) ||
      w.opens_at_minute < 0 ||
      w.closes_at_minute > 1440 ||
      w.opens_at_minute >= w.closes_at_minute
    )
      invalid();
    if (
      i &&
      hours[i - 1].weekday === w.weekday &&
      hours[i - 1].closes_at_minute > w.opens_at_minute
    )
      invalid();
  }
  return p;
}
function path(slug: string, id: string) {
  if (!isID(id)) invalid();
  return `/api/v1/admin/shops/${shopPathSegment(slug)}/professionals/${id}`;
}
export async function getProfessionalDetails(slug: string, id: string, signal?: AbortSignal) {
  return details(await requestJSON(path(slug, id), { authenticated: true, signal }), id);
}
export async function updateProfessionalDetails(
  slug: string,
  id: string,
  input: ProfessionalEditInput,
  signal?: AbortSignal,
) {
  return details(
    await requestJSON(path(slug, id), {
      authenticated: true,
      method: "PUT",
      signal,
      body: {
        name: input.name,
        active: input.active,
        service_ids: input.service_ids,
        revision: input.revision,
        schedule: {
          interval_minutes: input.schedule.interval_minutes,
          buffer_minutes: input.schedule.buffer_minutes,
          working_hours: input.schedule.working_hours.map((w) => ({
            weekday: w.weekday,
            opens_at_minute: w.opens_at_minute,
            closes_at_minute: w.closes_at_minute,
          })),
        },
      },
    }),
    id,
  );
}
export function scheduleTime(minutes: number) {
  return `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
}
export function parseScheduleTime(value: string, closing = false) {
  if (closing && value === "24:00") return 1440;
  if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/.test(value))
    throw new ApiError(
      "invalid_professional_schedule",
      "Confira os horários no formato 09:00. O fechamento deve ser depois da abertura; use 24:00 para terminar à meia-noite.",
    );
  const [h, m] = value.split(":").map(Number);
  return h * 60 + m;
}
