import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { isID } from "./identity";
import { instant, integer, maxQuantity, type ShopRequest } from "./products";

export type ReportSource = "all" | "services" | "products";
export type AppointmentOutcome = "scheduled" | "confirmed" | "completed" | "cancelled" | "no_show";
export type SaleOutcome = "recorded" | "cancelled";
export type ReportFilters = {
  from: string;
  through: string;
  source: ReportSource;
  customer_name?: string;
  phone?: string;
  phone_country?: string;
  professional_id?: string;
  service_id?: string;
  product_id?: string;
};
export type HistoryStatuses = {
  appointment_status?: AppointmentOutcome | "";
  sale_status?: SaleOutcome | "";
};
export type ReportScope = ShopRequest & { professionalID: string | null };
export type ReportPeriod = {
  from_date: string;
  through_date: string;
  from_at: string;
  until_at: string;
  timezone: string;
};
export type ReportCounts = {
  scheduled: number;
  confirmed: number;
  completed: number;
  completed_automatically: number;
  cancelled: number;
  no_show: number;
};
export type ReportValues = {
  currency: string;
  scheduled_service_value_cents: number;
  completed_service_value_cents: number;
  product_sale_value_cents: number;
  combined_value_cents: number;
  completed_appointment_count: number;
  product_sale_count: number;
  product_unit_count: number;
};
export type ReportDay = { local_date: string; counts: ReportCounts; values: ReportValues[] };
export type ReportGroup = {
  id: string | null;
  label: string;
  counts: ReportCounts;
  values: ReportValues[];
};
export type ReportOverview = {
  period: ReportPeriod;
  source: ReportSource;
  observed_at: string;
  counts: ReportCounts;
  values: ReportValues[];
  by_day: ReportDay[];
  by_professional: ReportGroup[];
  by_service: ReportGroup[];
  by_product: ReportGroup[];
};
export type ReportHistoryItem = {
  id: string; // Local composite key: one appointment or one item, never just sale ID.
  source: "services" | "products";
  record_id: string;
  item_id: string | null;
  service_id: string | null;
  product_id: string | null;
  professional_id: string | null;
  label: string;
  professional_name: string;
  currency: string;
  appointment_status: AppointmentOutcome | null;
  appointment_status_source: string | null;
  sale_status: SaleOutcome | null;
  customer_name: string;
  customer_phone: string;
  occurred_at: string;
  quantity: number;
  unit_price_cents: number;
  value_cents: number;
  recorded_value_cents: number;
};
export type ReportHistory = {
  period: ReportPeriod;
  source: ReportSource;
  observed_at: string;
  items: ReportHistoryItem[];
  next_cursor: string;
};
const outcomes = ["scheduled", "confirmed", "completed", "cancelled", "no_show"];
const provenance = ["legacy", "booking", "public", "staff", "automatic"];
const countKeys = [
  "scheduled",
  "confirmed",
  "completed",
  "completed_automatically",
  "cancelled",
  "no_show",
] as const;
const valueKeys = [
  "scheduled_service_value_cents",
  "completed_service_value_cents",
  "product_sale_value_cents",
  "combined_value_cents",
  "completed_appointment_count",
  "product_sale_count",
  "product_unit_count",
] as const;
function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "Os dados do relatório estão inconsistentes. Atualize a consulta.",
  );
}
function object(v: unknown): Record<string, unknown> {
  if (!v || typeof v !== "object" || Array.isArray(v)) invalid();
  return v as Record<string, unknown>;
}
function currency(v: unknown): v is string {
  return typeof v === "string" && /^[A-Z]{3}$/.test(v);
}
function instantKey(value: string) {
  const fraction = value.match(/\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/)?.[1] || "";
  if (fraction.length > 9) invalid();
  // Date.parse drops sub-millisecond precision; PostgreSQL ordering does not.
  return new Date(value).toISOString().slice(0, 19) + "." + fraction.padEnd(9, "0") + "Z";
}
export function validReportDate(v: string) {
  return (
    /^\d{4}-\d{2}-\d{2}$/.test(v) &&
    v >= "1900-01-01" &&
    v <= "9998-12-31" &&
    Number.isFinite(Date.parse(v + "T00:00:00Z")) &&
    new Date(v + "T00:00:00Z").toISOString().slice(0, 10) === v
  );
}
export function reportFilterError(f: ReportFilters): string {
  if (!validReportDate(f.from) || !validReportDate(f.through) || f.from > f.through)
    return "Confira a data inicial e a data final.";
  if ((Date.parse(f.through + "T00:00:00Z") - Date.parse(f.from + "T00:00:00Z")) / 86400000 >= 366)
    return "Consulte até 366 dias por vez.";
  if ((f.source === "services" && f.product_id) || (f.source === "products" && f.service_id))
    return "Limpe o filtro da outra origem.";
  return "";
}
export function reportLocalDate(at: string, zone: string) {
  const parts = new Intl.DateTimeFormat("en", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(at));
  return ["year", "month", "day"].map((key) => parts.find((p) => p.type === key)!.value).join("-");
}
export function reportMonth(at: string, zone: string): Pick<ReportFilters, "from" | "through"> {
  const day = reportLocalDate(at, zone),
    from = day.slice(0, 7) + "-01";
  const next = new Date(from + "T12:00:00Z");
  next.setUTCMonth(next.getUTCMonth() + 1);
  next.setUTCDate(0);
  return { from, through: next.toISOString().slice(0, 10) };
}
function metadata(v: Record<string, unknown>, f: ReportFilters) {
  const p = object(v.period) as unknown as ReportPeriod;
  if (
    p.from_date !== f.from ||
    p.through_date !== f.through ||
    !instant(p.from_at) ||
    !instant(p.until_at) ||
    Date.parse(p.from_at) >= Date.parse(p.until_at) ||
    typeof p.timezone !== "string" ||
    !instant(v.observed_at) ||
    v.source !== f.source
  )
    invalid();
  try {
    const next = new Date(f.through + "T12:00:00Z");
    next.setUTCDate(next.getUTCDate() + 1);
    if (
      reportLocalDate(p.from_at, p.timezone) !== f.from ||
      reportLocalDate(p.until_at, p.timezone) !== next.toISOString().slice(0, 10)
    )
      invalid();
  } catch {
    invalid();
  }
  return { period: p, source: v.source as ReportSource, observed_at: v.observed_at as string };
}
function measures(rawCounts: unknown, rawValues: unknown, source: ReportSource) {
  const c = object(rawCounts) as unknown as ReportCounts;
  if (
    countKeys.some((k) => !integer(c[k])) ||
    c.completed_automatically > c.completed ||
    !Array.isArray(rawValues)
  )
    invalid();
  const currencies = new Set<string>();
  let completed = 0,
    receipts = 0,
    units = 0;
  const values = rawValues.map((raw) => {
    const v = object(raw) as unknown as ReportValues;
    if (
      !currency(v.currency) ||
      currencies.has(v.currency) ||
      valueKeys.some((k) => !integer(v[k])) ||
      v.completed_service_value_cents > Number.MAX_SAFE_INTEGER - v.product_sale_value_cents ||
      v.combined_value_cents !== v.completed_service_value_cents + v.product_sale_value_cents ||
      (!v.completed_appointment_count && v.completed_service_value_cents !== 0) ||
      (!v.product_sale_count && (v.product_sale_value_cents !== 0 || v.product_unit_count !== 0)) ||
      (v.product_sale_count > 0 &&
        (v.product_sale_value_cents === 0 || v.product_unit_count < v.product_sale_count)) ||
      (source === "services" &&
        (v.product_sale_value_cents !== 0 ||
          v.product_sale_count !== 0 ||
          v.product_unit_count !== 0)) ||
      (source === "products" &&
        (v.completed_service_value_cents !== 0 ||
          v.completed_appointment_count !== 0 ||
          v.scheduled_service_value_cents !== 0))
    )
      invalid();
    currencies.add(v.currency);
    completed += v.completed_appointment_count;
    receipts += v.product_sale_count;
    units += v.product_unit_count;
    if (![completed, receipts, units].every(Number.isSafeInteger)) invalid();
    return v;
  });
  if (completed !== c.completed || (source === "products" && countKeys.some((k) => c[k] !== 0)))
    invalid();
  return { counts: c, values };
}
function effectiveProfessional(s: ReportScope, f: ReportFilters) {
  return s.professionalID || f.professional_id || null;
}
function parseOverview(raw: unknown, scope: ReportScope, f: ReportFilters): ReportOverview {
  const v = object(raw),
    meta = metadata(v, f),
    total = measures(v.counts, v.values, f.source);
  if (!Array.isArray(v.by_day)) invalid();
  let prior = "";
  const by_day = v.by_day.map((raw) => {
    const d = object(raw);
    if (
      typeof d.local_date !== "string" ||
      !validReportDate(d.local_date) ||
      d.local_date < f.from ||
      d.local_date > f.through ||
      d.local_date <= prior
    )
      invalid();
    prior = d.local_date;
    return { local_date: d.local_date, ...measures(d.counts, d.values, f.source) };
  });
  function groups(
    raw: unknown,
    source: ReportSource,
    filter: string | null,
    professional = false,
  ): ReportGroup[] {
    if (!Array.isArray(raw)) invalid();
    const seen = new Set<string | null>();
    return raw.map((raw) => {
      const g = object(raw);
      if (
        ((!professional || g.id !== null) && !isID(g.id)) ||
        seen.has(g.id as string | null) ||
        (filter && g.id !== filter) ||
        typeof g.label !== "string" ||
        (g.id !== null && !g.label.trim())
      )
        invalid();
      seen.add(g.id as string | null);
      return { id: g.id as string | null, label: g.label, ...measures(g.counts, g.values, source) };
    });
  }
  const by_service = groups(v.by_service, "services", f.service_id || null),
    by_product = groups(v.by_product, "products", f.product_id || null);
  if (
    (f.source === "services" && by_product.length) ||
    (f.source === "products" && by_service.length)
  )
    invalid();
  return {
    ...meta,
    ...total,
    by_day,
    by_service,
    by_product,
    by_professional: groups(v.by_professional, f.source, effectiveProfessional(scope, f), true),
  };
}
function parseHistory(
  raw: unknown,
  scope: ReportScope,
  f: ReportFilters,
  status: HistoryStatuses,
  cursor?: string,
): ReportHistory {
  const v = object(raw),
    meta = metadata(v, f);
  if (
    !Array.isArray(v.items) ||
    v.items.length > 25 ||
    typeof v.next_cursor !== "string" ||
    v.next_cursor.length > 1024 ||
    (v.next_cursor && (!/^[A-Za-z0-9_-]+$/.test(v.next_cursor) || v.next_cursor === cursor))
  )
    invalid();
  const ids = new Set<string>();
  let previous: ReportHistoryItem | undefined;
  const items = v.items.map((raw) => {
    const r = object(raw) as unknown as ReportHistoryItem;
    if (
      !["services", "products"].includes(r.source) ||
      (f.source !== "all" && r.source !== f.source) ||
      !isID(r.record_id) ||
      !currency(r.currency) ||
      typeof r.label !== "string" ||
      !r.label.trim() ||
      typeof r.professional_name !== "string" ||
      typeof r.customer_name !== "string" ||
      typeof r.customer_phone !== "string" ||
      (r.customer_phone && !/^\+[1-9]\d{1,14}$/.test(r.customer_phone)) ||
      !instant(r.occurred_at) ||
      Date.parse(r.occurred_at) < Date.parse(meta.period.from_at) ||
      Date.parse(r.occurred_at) >= Date.parse(meta.period.until_at) ||
      (r.professional_id !== null && !isID(r.professional_id)) ||
      (effectiveProfessional(scope, f) && r.professional_id !== effectiveProfessional(scope, f)) ||
      !integer(r.quantity, 1, maxQuantity) ||
      !integer(r.unit_price_cents) ||
      !integer(r.value_cents) ||
      !integer(r.recorded_value_cents) ||
      r.unit_price_cents > Math.floor(Number.MAX_SAFE_INTEGER / r.quantity) ||
      r.quantity * r.unit_price_cents !== r.value_cents
    )
      invalid();
    if (r.source === "services") {
      if (
        !isID(r.service_id) ||
        !isID(r.professional_id) ||
        !r.professional_name.trim() ||
        r.product_id !== null ||
        r.item_id !== null ||
        r.quantity !== 1 ||
        !outcomes.includes(r.appointment_status || "") ||
        !provenance.includes(r.appointment_status_source || "") ||
        r.sale_status !== null ||
        (f.service_id && r.service_id !== f.service_id) ||
        (status.appointment_status && r.appointment_status !== status.appointment_status) ||
        r.recorded_value_cents !== (r.appointment_status === "completed" ? r.value_cents : 0)
      )
        invalid();
    } else if (
      !isID(r.product_id) ||
      !isID(r.item_id) ||
      r.service_id !== null ||
      r.appointment_status !== null ||
      r.appointment_status_source !== null ||
      !["recorded", "cancelled"].includes(r.sale_status || "") ||
      !r.unit_price_cents ||
      (r.professional_id !== null && !r.professional_name.trim()) ||
      (f.product_id && r.product_id !== f.product_id) ||
      (status.sale_status && r.sale_status !== status.sale_status) ||
      r.recorded_value_cents !== (r.sale_status === "recorded" ? r.value_cents : 0)
    )
      invalid();
    const id = `${r.source}:${r.record_id}:${r.item_id || ""}`;
    if (ids.has(id)) invalid();
    ids.add(id);
    const item = { ...r, id };
    if (previous) {
      const time = instantKey(previous.occurred_at).localeCompare(instantKey(item.occurred_at));
      const a = `${previous.source}:${previous.record_id.toLowerCase()}:${previous.item_id?.toLowerCase() || ""}`,
        b = `${item.source}:${item.record_id.toLowerCase()}:${item.item_id?.toLowerCase() || ""}`;
      if (time < 0 || (time === 0 && a <= b)) invalid();
    }
    previous = item;
    return item;
  });
  if (v.next_cursor && items.length !== 25) invalid();
  return { ...meta, items, next_cursor: v.next_cursor };
}
function path(scope: ReportScope, f: ReportFilters, status?: HistoryStatuses, cursor?: string) {
  if (!isID(scope.userID) || !isID(scope.shopID))
    throw new ApiError("authentication_required", "Entre novamente para consultar os relatórios.");
  const error = reportFilterError(f);
  if (error) throw new ApiError("invalid_report_query", error);
  const q = new URLSearchParams({ from: f.from, through: f.through, source: f.source });
  for (const key of [
    "customer_name",
    "phone",
    "phone_country",
    "professional_id",
    "service_id",
    "product_id",
  ] as const)
    if (f[key]?.trim()) q.set(key, f[key]!.trim());
  if (status) {
    q.set("limit", "25");
    for (const key of ["appointment_status", "sale_status"] as const)
      if (status[key]) q.set(key, status[key]!);
    if (cursor) q.set("cursor", cursor);
  }
  if (q.toString().length > 4096)
    throw new ApiError("invalid_report_query", "Reduza o texto dos filtros e consulte novamente.");
  return (
    "/api/v1/admin/shops/" +
    shopPathSegment(scope.slug) +
    "/reports/activity" +
    (status ? "/history" : "") +
    "?" +
    q
  );
}
export async function getActivityReport(
  scope: ReportScope,
  f: ReportFilters,
  signal?: AbortSignal,
) {
  return parseOverview(
    await requestJSON(path(scope, f), {
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope,
    f,
  );
}
export async function getActivityHistory(
  scope: ReportScope,
  f: ReportFilters,
  status: HistoryStatuses,
  cursor?: string,
  signal?: AbortSignal,
) {
  return parseHistory(
    await requestJSON(path(scope, f, status, cursor), {
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope,
    f,
    status,
    cursor,
  );
}
export function reportMoney(cents: number, currency = "BRL") {
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency }).format(cents / 100);
}
