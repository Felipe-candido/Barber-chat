import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { isID } from "./identity";

export type ContactScope = { slug: string; shopID: string; userID: string };
export type Contact = {
  id: string;
  shop_id: string;
  first_name: string;
  last_name: string;
  name: string;
  phone: string;
  created_at: string;
};
export type ContactInput = {
  first_name: string;
  last_name: string;
  phone: string;
  phone_country: string;
};
export type ContactFilters = {
  name?: string;
  phone?: string;
  phone_country?: string;
  cursor?: string;
  limit?: number;
};
export type ContactsPage = { items: Contact[]; next_cursor: string | null };
function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "Não foi possível validar a resposta da API. Confira o contato antes de tentar cadastrar novamente.",
  );
}
export function parseContact(value: unknown, shopID: string): Contact {
  const c = value as Contact;
  if (
    !c ||
    !isID(c.id) ||
    c.id === "00000000-0000-0000-0000-000000000000" ||
    c.shop_id !== shopID ||
    typeof c.first_name !== "string" ||
    !c.first_name.trim() ||
    [...c.first_name].length > 100 ||
    typeof c.last_name !== "string" ||
    [...c.last_name].length > 100 ||
    c.name !== `${c.first_name} ${c.last_name}`.trim() ||
    typeof c.phone !== "string" ||
    !/^\+[1-9]\d{6,14}$/.test(c.phone) ||
    typeof c.created_at !== "string" ||
    !/(?:Z|[+-]\d{2}:\d{2})$/.test(c.created_at) ||
    !Number.isFinite(Date.parse(c.created_at))
  )
    invalid();
  return c;
}
function path(s: ContactScope) {
  return "/api/v1/admin/shops/" + shopPathSegment(s.slug) + "/contacts";
}
function auth(s: ContactScope, signal?: AbortSignal) {
  return { authenticated: true, expectedUserID: s.userID, signal };
}
export async function listContacts(
  s: ContactScope,
  filters: ContactFilters = {},
  signal?: AbortSignal,
): Promise<ContactsPage> {
  const q = new URLSearchParams();
  for (const key of ["name", "phone", "phone_country", "cursor"] as const) {
    if (filters[key]) q.set(key, filters[key]!);
  }
  q.set("limit", String(filters.limit ?? 25));
  const raw = (await requestJSON(path(s) + "?" + q.toString(), auth(s, signal))) as ContactsPage;
  if (
    !raw ||
    !Array.isArray(raw.items) ||
    raw.items.length > (filters.limit ?? 25) ||
    !(
      raw.next_cursor === null ||
      (typeof raw.next_cursor === "string" && /^[A-Za-z0-9_-]{1,512}$/.test(raw.next_cursor))
    ) ||
    (raw.next_cursor !== null && raw.items.length === 0)
  )
    invalid();
  const items = raw.items.map((c) => parseContact(c, s.shopID));
  if (new Set(items.map((c) => c.id)).size !== items.length) invalid();
  return { items, next_cursor: raw.next_cursor };
}
export async function getContact(s: ContactScope, id: string, signal?: AbortSignal) {
  const c = parseContact(
    await requestJSON(path(s) + "/" + encodeURIComponent(id), auth(s, signal)),
    s.shopID,
  );
  if (c.id !== id) invalid();
  return c;
}
export async function createContact(s: ContactScope, incoming: ContactInput, signal?: AbortSignal) {
  return parseContact(
    await requestJSON(path(s), {
      ...auth(s, signal),
      method: "POST",
      body: {
        first_name: incoming.first_name.trim(),
        last_name: incoming.last_name.trim(),
        phone: incoming.phone.trim(),
        phone_country: incoming.phone_country,
      },
    }),
    s.shopID,
  );
}
export function contactPhone(phone: string) {
  const m = /^\+55(\d{2})(\d{4,5})(\d{4})$/.exec(phone);
  return m ? `+55 (${m[1]}) ${m[2]}-${m[3]}` : phone;
}
