import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { instant, integer, type ShopRequest } from "./products";

export type WhatsAppConnection = {
  state: "not_configured" | "connecting" | "qr_required" | "connected" | "disconnected" | "unknown";
  desired_connected: boolean;
  stale: boolean;
  provider_configured: boolean;
  verified_at: string | null;
  last_connected_at: string | null;
  error_code: string;
};
export type PairingQR = { png: string; discard_after: string };
export type NotificationSettings = {
  enabled: boolean;
  delivery_mode: "request" | "automatic";
  reminder_lead_minutes: number;
  version: number;
  delivery_available: boolean;
  automatic_available: boolean;
};
export type BookingMessageStatus = {
  receipt?: "" | "accepted" | "delivered" | "read";
  kind: "booking_confirmation" | "booking_reminder";
  state: string;
  reason: string;
  mode: "request" | "automatic";
  due_at: string;
  expires_at: string;
  can_send: boolean;
};
export type BookingMessages = { messages: BookingMessageStatus[]; delivery_available: boolean };
const connectionStates = [
  "not_configured",
  "connecting",
  "qr_required",
  "connected",
  "disconnected",
  "unknown",
];
const messageStates = [
  "ready",
  "leased",
  "sending",
  "accepted",
  "failed",
  "unknown",
  "expired",
  "invalidated",
];
function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "Não foi possível validar o estado do WhatsApp. Consulte novamente antes de executar outra ação.",
  );
}
function record(v: unknown): Record<string, unknown> {
  if (!v || typeof v !== "object" || Array.isArray(v)) invalid();
  return v as Record<string, unknown>;
}
function base(s: ShopRequest) {
  return "/api/v1/admin/shops/" + shopPathSegment(s.slug);
}
function auth(s: ShopRequest, signal?: AbortSignal) {
  return { authenticated: true, expectedUserID: s.userID, signal };
}
function connection(v: unknown): WhatsAppConnection {
  const r = record(v);
  if (
    !connectionStates.includes(String(r.state)) ||
    typeof r.desired_connected !== "boolean" ||
    typeof r.stale !== "boolean" ||
    typeof r.provider_configured !== "boolean" ||
    (r.verified_at !== null && !instant(r.verified_at)) ||
    (r.last_connected_at !== null && !instant(r.last_connected_at)) ||
    typeof r.error_code !== "string" ||
    r.error_code.length > 40
  )
    invalid();
  return r as WhatsAppConnection;
}
function settings(v: unknown): NotificationSettings {
  const r = record(v);
  if (
    typeof r.enabled !== "boolean" ||
    !["request", "automatic"].includes(String(r.delivery_mode)) ||
    !integer(r.reminder_lead_minutes, 5, 10080) ||
    !integer(r.version) ||
    typeof r.delivery_available !== "boolean" ||
    typeof r.automatic_available !== "boolean"
  )
    invalid();
  return r as NotificationSettings;
}
export async function getWhatsAppConnection(s: ShopRequest, signal?: AbortSignal) {
  return connection(await requestJSON(base(s) + "/whatsapp/connection", auth(s, signal)));
}
export async function connectWhatsApp(
  s: ShopRequest,
  signal?: AbortSignal,
): Promise<{ connection: WhatsAppConnection; qr: PairingQR | null }> {
  const r = record(
    await requestJSON(base(s) + "/whatsapp/connect", {
      ...auth(s, signal),
      method: "POST",
      body: {},
    }),
  );
  let qr: PairingQR | null = null;
  if (r.qr !== null) {
    const q = record(r.qr);
    if (
      typeof q.png !== "string" ||
      q.png.length > 200000 ||
      !/^data:image\/png;base64,iVBORw0KGgo[A-Za-z0-9+/]*={0,2}$/.test(q.png) ||
      !instant(q.discard_after)
    )
      invalid();
    qr = { png: q.png, discard_after: q.discard_after };
  }
  return { connection: connection(r.connection), qr };
}
export async function disconnectWhatsApp(s: ShopRequest, signal?: AbortSignal) {
  return connection(
    await requestJSON(base(s) + "/whatsapp/disconnect", {
      ...auth(s, signal),
      method: "POST",
      body: {},
    }),
  );
}
export async function getNotificationSettings(s: ShopRequest, signal?: AbortSignal) {
  return settings(await requestJSON(base(s) + "/notifications/settings", auth(s, signal)));
}
export async function saveNotificationSettings(
  s: ShopRequest,
  input: NotificationSettings,
  signal?: AbortSignal,
) {
  return settings(
    await requestJSON(base(s) + "/notifications/settings", {
      ...auth(s, signal),
      method: "PUT",
      body: {
        enabled: input.enabled,
        delivery_mode: input.delivery_mode,
        reminder_lead_minutes: input.reminder_lead_minutes,
        version: input.version,
      },
    }),
  );
}
export async function getNotificationOptions(
  slug: string,
  signal?: AbortSignal,
  userID?: string,
): Promise<boolean> {
  const path = userID
    ? "/api/v1/admin/shops/" + shopPathSegment(slug) + "/notifications/options"
    : "/api/v1/public/shops/" + shopPathSegment(slug) + "/notification-options";
  const r = record(
    await requestJSON(path, { signal, authenticated: !!userID, expectedUserID: userID }),
  );
  if (typeof r.whatsapp_enabled !== "boolean") invalid();
  return r.whatsapp_enabled;
}
export async function getBookingMessages(
  s: ShopRequest,
  id: string,
  signal?: AbortSignal,
): Promise<BookingMessages> {
  const r = record(
    await requestJSON(
      base(s) + "/appointments/" + encodeURIComponent(id) + "/notifications",
      auth(s, signal),
    ),
  );
  if (
    typeof r.delivery_available !== "boolean" ||
    !Array.isArray(r.messages) ||
    r.messages.length > 2
  )
    invalid();
  const messages = r.messages.map((v) => {
    const m = record(v);
    if (
      (m.receipt !== undefined &&
        !["", "accepted", "delivered", "read"].includes(String(m.receipt))) ||
      !["booking_confirmation", "booking_reminder"].includes(String(m.kind)) ||
      !messageStates.includes(String(m.state)) ||
      !["request", "automatic"].includes(String(m.mode)) ||
      !instant(m.due_at) ||
      !instant(m.expires_at) ||
      typeof m.can_send !== "boolean" ||
      typeof m.reason !== "string" ||
      m.reason.length > 40
    )
      invalid();
    return m as BookingMessageStatus;
  });
  if (new Set(messages.map((m) => m.kind)).size !== messages.length) invalid();
  return { messages, delivery_available: r.delivery_available };
}
export async function sendBookingMessage(
  s: ShopRequest,
  id: string,
  kind: BookingMessageStatus["kind"],
  signal?: AbortSignal,
) {
  const r = record(
    await requestJSON(
      base(s) +
        "/appointments/" +
        encodeURIComponent(id) +
        "/notifications/" +
        (kind === "booking_confirmation" ? "confirmation" : "reminder"),
      { ...auth(s, signal), method: "POST", body: {} },
    ),
  );
  if (typeof r.state !== "string" || r.state.length > 40) invalid();
  return r.state;
}
export function bookingMessageLabel(state: string): string {
  return (
    (
      {
        ready: "Pronta para envio",
        leased: "Envio em andamento",
        sending: "Envio em andamento",
        accepted: "Envio aceito",
        failed: "Não enviada",
        unknown: "Resultado incerto — confira antes de agir",
        expired: "Prazo de envio encerrado",
        invalidated: "Envio desativado",
        connection_unavailable: "WhatsApp sem conexão confirmada",
        not_due: "Aguardando horário programado",
        not_requested: "Sem autorização ou planejamento de WhatsApp",
        unavailable: "Envio indisponível",
        busy: "Uma operação já está em andamento",
        sender_busy:
          "Outro envio está em andamento nesta barbearia. Confira novamente em instantes.",
      } as Record<string, string>
    )[state] ?? "Consulte o estado do envio"
  );
}
