import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

export type StaffIdentity = { user_id: string; display_name: string };
export type AccessibleShop = { shop_id: string; name: string; slug: string };

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function isID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    uuidPattern.test(value) &&
    value !== "00000000-0000-0000-0000-000000000000"
  );
}
function invalidResponse(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou dados de identidade ou barbearia incompatíveis.",
  );
}

export async function getMe(signal?: AbortSignal): Promise<StaffIdentity> {
  const value = await requestJSON("/api/v1/admin/me", { authenticated: true, signal });
  const item = value as StaffIdentity | null;
  if (
    !item ||
    !isID(item.user_id) ||
    typeof item.display_name !== "string" ||
    !item.display_name.trim()
  ) {
    invalidResponse();
  }
  return { user_id: item.user_id, display_name: item.display_name };
}

export async function listMyShops(signal?: AbortSignal): Promise<AccessibleShop[]> {
  const value = await requestJSON("/api/v1/admin/shops", { authenticated: true, signal });
  if (!Array.isArray(value)) invalidResponse();
  const ids = new Set<string>();
  const slugs = new Set<string>();
  return value.map((raw: unknown) => {
    const item = raw as AccessibleShop | null;
    if (
      !item ||
      !isID(item.shop_id) ||
      typeof item.name !== "string" ||
      !item.name.trim() ||
      typeof item.slug !== "string"
    ) {
      invalidResponse();
    }
    try {
      shopPathSegment(item.slug);
    } catch {
      invalidResponse();
    }
    if (ids.has(item.shop_id.toLowerCase()) || slugs.has(item.slug)) invalidResponse();
    ids.add(item.shop_id.toLowerCase());
    slugs.add(item.slug);
    return { shop_id: item.shop_id, name: item.name, slug: item.slug };
  });
}
