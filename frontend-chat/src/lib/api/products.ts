import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { isID } from "./identity";

export type Product = {
  id: string;
  shop_id: string;
  name: string;
  description: string;
  price_cents: number;
  currency: "BRL";
  active: boolean;
  version: number;
  quantity_on_hand: number;
  minimum_quantity: number;
  stock_version: number;
  low_stock: boolean;
  created_at: string;
  updated_at: string;
};
export type ProductDraft = {
  name: string;
  description: string;
  price_cents: number;
  initial_quantity: number;
  minimum_quantity: number;
};
export type ProductEdit = Pick<Product, "name" | "description" | "price_cents" | "active"> & {
  expected_version: number;
};
export type StockInput = {
  operation: "receipt" | "adjustment";
  quantity_delta: number;
  reason: string;
};
export type Movement = {
  id: string;
  product_id: string;
  actor_id: string;
  kind: "opening" | "receipt" | "adjustment" | "sale" | "sale_reversal";
  quantity_delta: number;
  reason: string;
  created_at: string;
  sale_item_id: string | null;
  reverses_movement_id: string | null;
};
export type Page<T> = { items: T[]; next_cursor: string | null };
export type ProductMutation = { product: Product; movement_id: string | null; replayed: boolean };
export type ShopRequest = { slug: string; shopID: string; userID: string };
export const maxQuantity = 2_147_483_647;
export const integer = (v: unknown, min = 0, max = Number.MAX_SAFE_INTEGER): v is number =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= min && v <= max;
export const instant = (v: unknown): v is string =>
  typeof v === "string" && /(?:Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v));
export function isProductDraft(value: unknown): value is ProductDraft {
  const p = value as ProductDraft;
  return (
    !!p &&
    typeof p.name === "string" &&
    !!p.name.trim() &&
    typeof p.description === "string" &&
    integer(p.price_cents, 1) &&
    integer(p.initial_quantity, 0, maxQuantity) &&
    integer(p.minimum_quantity, 0, maxQuantity)
  );
}
export function isStockInput(value: unknown): value is StockInput {
  const s = value as StockInput;
  return (
    !!s &&
    ["receipt", "adjustment"].includes(s.operation) &&
    typeof s.quantity_delta === "number" &&
    integer(Math.abs(s.quantity_delta), 1, maxQuantity) &&
    (s.operation !== "receipt" || s.quantity_delta > 0) &&
    typeof s.reason === "string" &&
    !!s.reason.trim()
  );
}
export function invalidCommerceResponse(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou dados incompatíveis. Confira o histórico antes de iniciar outra operação.",
  );
}
export function productPath(slug: string) {
  return "/api/v1/admin/shops/" + shopPathSegment(slug) + "/products";
}
export function parseProduct(value: unknown, shopID: string): Product {
  const p = value as Product;
  if (
    !p ||
    !isID(p.id) ||
    p.shop_id !== shopID ||
    typeof p.name !== "string" ||
    !p.name.trim() ||
    typeof p.description !== "string" ||
    !integer(p.price_cents, 1) ||
    p.currency !== "BRL" ||
    typeof p.active !== "boolean" ||
    !integer(p.version, 1) ||
    !integer(p.stock_version, 1) ||
    !integer(p.quantity_on_hand, 0, maxQuantity) ||
    !integer(p.minimum_quantity, 0, maxQuantity) ||
    p.low_stock !== p.quantity_on_hand <= p.minimum_quantity ||
    !instant(p.created_at) ||
    !instant(p.updated_at)
  )
    invalidCommerceResponse();
  return p;
}
function page<T>(value: unknown, parse: (v: unknown) => T): Page<T> {
  const p = value as Page<unknown>;
  if (
    !p ||
    !Array.isArray(p.items) ||
    p.items.length > 100 ||
    (p.next_cursor !== null &&
      (typeof p.next_cursor !== "string" || !p.next_cursor || p.next_cursor.length > 4096))
  )
    invalidCommerceResponse();
  return { items: p.items.map(parse), next_cursor: p.next_cursor };
}
export async function listProducts(
  scope: ShopRequest,
  query: { search?: string; includeInactive?: boolean; cursor?: string } = {},
  signal?: AbortSignal,
): Promise<Page<Product>> {
  const q = new URLSearchParams({ limit: "100" });
  if (query.search) q.set("search", query.search);
  if (query.includeInactive) q.set("include_inactive", "true");
  if (query.cursor) q.set("cursor", query.cursor);
  const out = page(
    await requestJSON(productPath(scope.slug) + "?" + q, {
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    (v) => parseProduct(v, scope.shopID),
  );
  if (out.items.some((p) => !query.includeInactive && !p.active)) invalidCommerceResponse();
  return out;
}
function parseMutation(value: unknown, scope: ShopRequest, productID?: string): ProductMutation {
  const m = value as ProductMutation;
  if (!m || typeof m.replayed !== "boolean" || (m.movement_id !== null && !isID(m.movement_id)))
    invalidCommerceResponse();
  const product = parseProduct(m.product, scope.shopID);
  if (productID && product.id !== productID) invalidCommerceResponse();
  return { product, movement_id: m.movement_id, replayed: m.replayed };
}
export async function createProduct(
  scope: ShopRequest,
  body: ProductDraft,
  key: string,
  signal?: AbortSignal,
) {
  return parseMutation(
    await requestJSON(productPath(scope.slug), {
      method: "POST",
      body,
      idempotencyKey: key,
      authenticated: true,
      expectedUserID: scope.userID,
      signal,
    }),
    scope,
  );
}
export async function updateProduct(scope: ShopRequest, id: string, body: ProductEdit) {
  const out = parseProduct(
    await requestJSON(productPath(scope.slug) + "/" + encodeURIComponent(id), {
      method: "PUT",
      body,
      authenticated: true,
      expectedUserID: scope.userID,
    }),
    scope.shopID,
  );
  if (out.id !== id) invalidCommerceResponse();
  return out;
}
export async function updateMinimum(scope: ShopRequest, p: Product, minimum: number) {
  const out = parseProduct(
    await requestJSON(productPath(scope.slug) + "/" + encodeURIComponent(p.id) + "/stock-minimum", {
      method: "PUT",
      body: { minimum_quantity: minimum, expected_stock_version: p.stock_version },
      authenticated: true,
      expectedUserID: scope.userID,
    }),
    scope.shopID,
  );
  if (out.id !== p.id) invalidCommerceResponse();
  return out;
}
export async function recordStock(
  scope: ShopRequest,
  id: string,
  body: StockInput,
  key: string,
  signal?: AbortSignal,
) {
  const result = parseMutation(
    await requestJSON(productPath(scope.slug) + "/" + encodeURIComponent(id) + "/stock-movements", {
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
  if (result.movement_id === null) invalidCommerceResponse();
  return result;
}
export async function listMovements(
  scope: ShopRequest,
  id: string,
  cursor?: string,
  signal?: AbortSignal,
): Promise<Page<Movement>> {
  const q = new URLSearchParams({ limit: "25" });
  if (cursor) q.set("cursor", cursor);
  return page(
    await requestJSON(
      productPath(scope.slug) + "/" + encodeURIComponent(id) + "/stock-movements?" + q,
      { authenticated: true, expectedUserID: scope.userID, signal },
    ),
    (v) => {
      const m = v as Movement;
      if (
        !m ||
        !isID(m.id) ||
        m.product_id !== id ||
        !isID(m.actor_id) ||
        !["opening", "receipt", "adjustment", "sale", "sale_reversal"].includes(m.kind) ||
        typeof m.quantity_delta !== "number" ||
        !integer(Math.abs(m.quantity_delta), 1, maxQuantity) ||
        typeof m.reason !== "string" ||
        !instant(m.created_at) ||
        (m.sale_item_id !== null && !isID(m.sale_item_id)) ||
        (m.reverses_movement_id !== null && !isID(m.reverses_movement_id))
      )
        invalidCommerceResponse();
      return m;
    },
  );
}
export function priceCents(raw: string): number {
  const value = raw.trim();
  if (!/^\d{1,14}([,.]\d{1,2})?$/.test(value))
    throw new Error("Informe um preço positivo, como 39,90, sem separador de milhares.");
  const [whole, fraction = ""] = value.replace(",", ".").split(".");
  const cents = Number(whole + fraction.padEnd(2, "0"));
  if (!integer(cents, 1))
    throw new Error("O preço deve ser positivo e estar dentro do limite permitido.");
  return cents;
}
export function quantity(raw: string, min = 0): number {
  const n = Number(raw);
  if (!/^\d+$/.test(raw) || !integer(n, min, maxQuantity))
    throw new Error("Informe uma quantidade inteira válida.");
  return n;
}
