import { ApiError, requestJSON } from "./client";

import type { ShopAddress } from "../shop-address";
export type { ShopAddress } from "../shop-address";
export type ShopRegistration = {
  display_name: string;
  name: string;
  slug: string;
  timezone: string;
  address: ShopAddress;
};
export type RegisteredShop = {
  shop_id: string;
  name: string;
  slug: string;
  timezone: string;
  replayed: boolean;
};

export async function registerShop(
  input: ShopRegistration,
  key: string,
  userID: string,
): Promise<RegisteredShop> {
  const result = (await requestJSON("/api/v1/onboarding/shops", {
    method: "POST",
    body: input,
    authenticated: true,
    idempotencyKey: key,
    expectedUserID: userID,
  })) as Partial<RegisteredShop> | null;
  if (
    !result ||
    typeof result.shop_id !== "string" ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(result.shop_id) ||
    typeof result.name !== "string" ||
    !result.name.trim() ||
    typeof result.slug !== "string" ||
    !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(result.slug) ||
    typeof result.timezone !== "string" ||
    typeof result.replayed !== "boolean"
  ) {
    throw new ApiError(
      "invalid_response",
      "Não foi possível confirmar o resultado. Recupere a mesma tentativa antes de criar outra barbearia.",
    );
  }
  return result as RegisteredShop;
}
