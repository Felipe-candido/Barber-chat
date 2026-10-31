import { requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { isID } from "./identity";
import {
  instant,
  integer,
  invalidCommerceResponse,
  maxQuantity,
  type ShopRequest,
} from "./products";

export type SaleItemInput = {
  product_id: string;
  quantity: number;
  expected_product_version: number;
  expected_unit_price_cents: number;
};
export type SaleInput = {
  professional_id?: string;
  appointment_id?: string;
  items: SaleItemInput[];
};
export type SaleCancellation = {
  actor_id: string;
  reason: string;
  restock: boolean;
  cancelled_at: string;
};
export type Sale = {
  id: string;
  shop_id: string;
  recorded_by_user_id: string;
  professional_id: string | null;
  appointment_id: string | null;
  professional_name: string;
  items: {
    id: string;
    product_id: string;
    product_name: string;
    quantity: number;
    unit_price_cents: number;
    line_total_cents: number;
    currency: "BRL";
  }[];
  status: "recorded" | "cancelled";
  currency: "BRL";
  total_cents: number;
  version: number;
  sold_at: string;
  replayed: boolean;
  cancellation: SaleCancellation | null;
};
export type CancelSaleInput = { expected_version: number; reason: string; restock: boolean };
export type SalesFilters = {
  from: string;
  through: string;
  status?: "" | "recorded" | "cancelled";
  professional_id?: string;
};
export type SalesPage = { items: Sale[]; next_cursor: string; timezone: string };
export function isSaleInput(value: unknown): value is SaleInput {
  const s = value as SaleInput;
  const valid =
    !!s &&
    (s.professional_id === undefined || isID(s.professional_id)) &&
    (s.appointment_id === undefined || isID(s.appointment_id)) &&
    Array.isArray(s.items) &&
    s.items.length > 0 &&
    s.items.length <= 50 &&
    new Set(s.items.map((i) => i?.product_id)).size === s.items.length &&
    s.items.every(
      (i) =>
        i &&
        isID(i.product_id) &&
        integer(i.quantity, 1, maxQuantity) &&
        integer(i.expected_product_version, 1) &&
        integer(i.expected_unit_price_cents, 1),
    );
  if (!valid) return false;
  let total = 0;
  for (const item of s.items) {
    if (
      item.expected_unit_price_cents > Math.floor(Number.MAX_SAFE_INTEGER / item.quantity) ||
      total > Number.MAX_SAFE_INTEGER - item.quantity * item.expected_unit_price_cents
    )
      return false;
    total += item.quantity * item.expected_unit_price_cents;
  }
  return true;
}
export function isCancellationInput(value: unknown): value is CancelSaleInput {
  const c = value as CancelSaleInput;
  return (
    !!c &&
    integer(c.expected_version, 1) &&
    typeof c.reason === "string" &&
    !!c.reason.trim() &&
    typeof c.restock === "boolean"
  );
}
export function salesPath(slug: string) {
  return "/api/v1/admin/shops/" + shopPathSegment(slug) + "/sales";
}
export function parseSale(value: unknown, scope: ShopRequest, id?: string): Sale {
  const s = value as Sale;
  if (
    !s ||
    !isID(s.id) ||
    (id && s.id !== id) ||
    s.shop_id !== scope.shopID ||
    !isID(s.recorded_by_user_id) ||
    (s.professional_id !== null && !isID(s.professional_id)) ||
    (s.appointment_id !== null && !isID(s.appointment_id)) ||
    typeof s.professional_name !== "string" ||
    !["recorded", "cancelled"].includes(s.status) ||
    s.currency !== "BRL" ||
    !integer(s.total_cents, 1) ||
    s.version !== (s.status === "recorded" ? 1 : 2) ||
    !instant(s.sold_at) ||
    typeof s.replayed !== "boolean" ||
    !Array.isArray(s.items) ||
    s.items.length < 1 ||
    s.items.length > 50
  )
    invalidCommerceResponse();
  const ids = new Set<string>(),
    products = new Set<string>();
  let total = 0;
  for (const i of s.items) {
    if (
      !i ||
      !isID(i.id) ||
      ids.has(i.id) ||
      !isID(i.product_id) ||
      products.has(i.product_id) ||
      typeof i.product_name !== "string" ||
      !i.product_name.trim() ||
      !integer(i.quantity, 1, maxQuantity) ||
      !integer(i.unit_price_cents, 1) ||
      !integer(i.line_total_cents, 1) ||
      i.unit_price_cents > Math.floor(Number.MAX_SAFE_INTEGER / i.quantity) ||
      i.line_total_cents !== i.quantity * i.unit_price_cents ||
      i.currency !== "BRL" ||
      total > Number.MAX_SAFE_INTEGER - i.line_total_cents
    )
      invalidCommerceResponse();
    ids.add(i.id);
    products.add(i.product_id);
    total += i.line_total_cents;
  }
  if (total !== s.total_cents) invalidCommerceResponse();
  if (s.status === "recorded") {
    if (s.cancellation !== null) invalidCommerceResponse();
  } else {
    const c = s.cancellation;
    if (
      !c ||
      !isID(c.actor_id) ||
      typeof c.reason !== "string" ||
      !c.reason.trim() ||
      typeof c.restock !== "boolean" ||
      !instant(c.cancelled_at) ||
      Date.parse(c.cancelled_at) < Date.parse(s.sold_at)
    )
      invalidCommerceResponse();
  }
  return s;
}
export async function registerSale(
  scope: ShopRequest,
  body: SaleInput,
  key: string,
  signal?: AbortSignal,
) {
  const sale = parseSale(
    await requestJSON(salesPath(scope.slug), {
      method: "POST",
      body,
      idempotencyKey: key,
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope,
  );
  if (
    sale.recorded_by_user_id !== scope.userID ||
    sale.appointment_id !== (body.appointment_id ?? null) ||
    (body.professional_id && sale.professional_id !== body.professional_id) ||
    sale.items.length !== body.items.length ||
    body.items.some(
      (i) =>
        !sale.items.some(
          (saved) =>
            saved.product_id === i.product_id &&
            saved.quantity === i.quantity &&
            saved.unit_price_cents === i.expected_unit_price_cents,
        ),
    )
  )
    invalidCommerceResponse();
  return sale;
}
export async function getSale(scope: ShopRequest, id: string, signal?: AbortSignal) {
  return parseSale(
    await requestJSON(salesPath(scope.slug) + "/" + encodeURIComponent(id), {
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope,
    id,
  );
}
export async function cancelSale(
  scope: ShopRequest,
  id: string,
  body: CancelSaleInput,
  key: string,
  signal?: AbortSignal,
) {
  const s = parseSale(
    await requestJSON(salesPath(scope.slug) + "/" + encodeURIComponent(id) + "/cancel", {
      method: "POST",
      body,
      idempotencyKey: key,
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope,
    id,
  );
  if (
    s.status !== "cancelled" ||
    s.cancellation?.reason !== body.reason.trim() ||
    s.cancellation?.restock !== body.restock
  )
    invalidCommerceResponse();
  return s;
}
export async function listSales(
  scope: ShopRequest,
  filters: SalesFilters,
  cursor?: string,
  signal?: AbortSignal,
): Promise<SalesPage> {
  const q = new URLSearchParams({ from: filters.from, through: filters.through, limit: "25" });
  if (filters.status) q.set("status", filters.status);
  if (filters.professional_id) q.set("professional_id", filters.professional_id);
  if (cursor) q.set("cursor", cursor);
  const p = (await requestJSON(salesPath(scope.slug) + "?" + q, {
    authenticated: true,
    expectedUserID: scope.userID,
    signal,
  })) as SalesPage;
  if (
    !p ||
    !Array.isArray(p.items) ||
    p.items.length > 25 ||
    typeof p.next_cursor !== "string" ||
    p.next_cursor.length > 1024 ||
    typeof p.timezone !== "string" ||
    !p.timezone ||
    p.timezone === "Local"
  )
    invalidCommerceResponse();
  try {
    new Intl.DateTimeFormat("pt-BR", { timeZone: p.timezone }).format();
  } catch {
    invalidCommerceResponse();
  }
  const items = p.items.map((s) => parseSale(s, scope));
  if (
    items.some(
      (s) =>
        (filters.professional_id && s.professional_id !== filters.professional_id) ||
        (filters.status && s.status !== filters.status),
    )
  )
    invalidCommerceResponse();
  return { ...p, items };
}
