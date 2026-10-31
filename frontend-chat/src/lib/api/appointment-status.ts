import { ApiError, requestJSON } from "./client";
import {
  hasAppointmentMetadata,
  parseAdminAppointment,
  type AdminAppointment,
  type AppointmentMetadata,
  type AppointmentStatusSource,
} from "./agenda";
import { shopPathSegment } from "./config";
import { isID } from "./identity";
import { instant, integer, type ShopRequest } from "./products";

export type AppointmentStatusInput = {
  status: "completed" | "no_show" | "cancelled";
  expected_version: number;
  reason: string;
};
export type AppointmentStatusEvent = {
  version: number;
  previous_status: AdminAppointment["status"];
  status: AdminAppointment["status"];
  source: AppointmentStatusSource;
  actor_id: string | null;
  reason: string;
  occurred_at: string;
};
export type AppointmentStatusDetail = {
  appointment: AdminAppointment & AppointmentMetadata;
  history: AppointmentStatusEvent[];
  history_truncated: boolean;
  replayed: boolean;
  result_version: number;
};
export function isAppointmentStatusInput(value: unknown): value is AppointmentStatusInput {
  const v = value as AppointmentStatusInput;
  return (
    !!v &&
    ["completed", "no_show", "cancelled"].includes(v.status) &&
    integer(v.expected_version, 1) &&
    typeof v.reason === "string" &&
    !!v.reason.trim() &&
    [...v.reason].length <= 500 &&
    !/[\u0000-\u001f\u007f]/.test(v.reason)
  );
}
function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou dados incompatíveis do atendimento. Consulte o estado atual antes de iniciar outra alteração.",
  );
}
function path(scope: ShopRequest, id: string) {
  if (!isID(scope.userID) || !isID(scope.shopID) || !isID(id))
    throw new ApiError(
      "authentication_required",
      "Confirme sua conta e sua barbearia antes de continuar.",
      401,
    );
  return (
    "/api/v1/admin/shops/" + shopPathSegment(scope.slug) + "/appointments/" + encodeURIComponent(id)
  );
}
function parse(value: unknown, id: string): AppointmentStatusDetail {
  const v = value as AppointmentStatusDetail;
  if (
    !v ||
    !v.appointment ||
    !Array.isArray(v.history) ||
    v.history.length > 100 ||
    typeof v.history_truncated !== "boolean" ||
    typeof v.replayed !== "boolean"
  )
    invalid();
  const appointment = parseAdminAppointment(v.appointment);
  if (
    appointment.id !== id ||
    !hasAppointmentMetadata(appointment) ||
    !integer(v.result_version, 1, appointment.version)
  )
    invalid();
  let previous = appointment.version + 1;
  const statuses = ["scheduled", "confirmed", "completed", "no_show", "cancelled"];
  for (const e of v.history) {
    if (
      !e ||
      !integer(e.version, 2, appointment.version) ||
      e.version >= previous ||
      !statuses.includes(e.previous_status) ||
      !statuses.includes(e.status) ||
      !["public", "staff", "automatic"].includes(e.source) ||
      (e.source === "staff" ? !isID(e.actor_id) : e.actor_id !== null) ||
      typeof e.reason !== "string" ||
      !e.reason.trim() ||
      !instant(e.occurred_at)
    )
      invalid();
    previous = e.version;
  }
  return { ...v, appointment };
}
export async function getAppointmentStatus(scope: ShopRequest, id: string, signal?: AbortSignal) {
  return parse(
    await requestJSON(path(scope, id), {
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    id,
  );
}
export async function changeAppointmentStatus(
  scope: ShopRequest,
  id: string,
  body: AppointmentStatusInput,
  key: string,
  signal?: AbortSignal,
) {
  const out = parse(
    await requestJSON(path(scope, id) + "/status", {
      method: "POST",
      body,
      idempotencyKey: key,
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    id,
  );
  if (out.result_version !== body.expected_version + 1) invalid();
  return out;
}
