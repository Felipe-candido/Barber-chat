import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { brazilianStates, type ShopAddress } from "../shop-address";

export type ShopSettings = {
  shop_id: string;
  name: string;
  slug: string;
  timezone: string;
  phone: string;
  description: string;
  address: ShopAddress | null;
  version: number;
};
export type SettingsDraft = {
  name: string;
  phone: string;
  description: string;
  address: ShopAddress;
};
export type SettingsUpdate = SettingsDraft & { version: number };

function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou configurações incompatíveis. Confira os dados na API antes de salvar novamente.",
  );
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid();
  return value as Record<string, unknown>;
}
function text(value: unknown, max: number, required = false): value is string {
  return typeof value === "string" && [...value].length <= max && (!required || !!value.trim());
}

function parseSettings(value: unknown, shopID: string, slug: string): ShopSettings {
  const item = record(value);
  if (
    item.shop_id !== shopID ||
    item.slug !== slug ||
    !text(item.name, 150, true) ||
    !text(item.timezone, 100, true) ||
    !text(item.phone, 40) ||
    !text(item.description, 1000) ||
    !Number.isSafeInteger(item.version) ||
    Number(item.version) < 1
  )
    invalid();
  try {
    new Intl.DateTimeFormat("pt-BR", { timeZone: item.timezone });
  } catch {
    invalid();
  }
  let address: ShopAddress | null = null;
  if (item.address !== null) {
    const a = record(item.address);
    if (
      !text(a.postal_code, 8) ||
      !/^[0-9]{8}$/.test(a.postal_code) ||
      !text(a.street, 150, true) ||
      !text(a.number, 20, true) ||
      !text(a.complement, 100) ||
      !text(a.district, 100, true) ||
      !text(a.city, 100, true) ||
      !text(a.state, 2) ||
      !brazilianStates.includes(a.state)
    )
      invalid();
    address = {
      postal_code: a.postal_code,
      street: a.street,
      number: a.number,
      complement: a.complement,
      district: a.district,
      city: a.city,
      state: a.state,
    };
  }
  return {
    shop_id: shopID,
    slug,
    name: item.name,
    timezone: item.timezone,
    phone: item.phone,
    description: item.description,
    version: Number(item.version),
    address,
  };
}

export async function getShopSettings(
  slug: string,
  shopID: string,
  userID: string,
  signal?: AbortSignal,
) {
  return parseSettings(
    await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/settings`, {
      authenticated: true,
      signal,
      expectedUserID: userID,
    }),
    shopID,
    slug,
  );
}

export async function updateShopSettings(
  slug: string,
  shopID: string,
  userID: string,
  input: SettingsUpdate,
  signal?: AbortSignal,
) {
  // Explicit allowlist: no tenant, user, role, slug, timezone or publication in JSON.
  const body: SettingsUpdate = {
    version: input.version,
    name: input.name,
    phone: input.phone,
    description: input.description,
    address: { ...input.address },
  };
  if (new TextEncoder().encode(JSON.stringify(body)).length > 8192)
    throw new ApiError(
      "body_too_large",
      "Os dados ultrapassam 8 KiB. Reduza os textos antes de salvar.",
      413,
    );
  const settings = parseSettings(
    await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/settings`, {
      method: "PUT",
      body,
      authenticated: true,
      expectedUserID: userID,
      signal,
    }),
    shopID,
    slug,
  );
  if (settings.version !== input.version + 1 || !settings.address) invalid();
  return settings;
}
