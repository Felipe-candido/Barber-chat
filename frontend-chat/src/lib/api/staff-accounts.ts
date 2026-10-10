import { ApiError, requestJSON } from "./client";
import { isID } from "./identity";
import { shopPathSegment } from "./config";

export type StaffAccount = {
  id: string;
  professional_id: string;
  email: string;
  user_id: string | null;
  state: "pending" | "linked" | "review";
  active: boolean;
};
function account(value: unknown): StaffAccount {
  const item = value as StaffAccount | null;
  if (
    !item ||
    !isID(item.id) ||
    !isID(item.professional_id) ||
    typeof item.email !== "string" ||
    !item.email.includes("@") ||
    (item.user_id !== null && !isID(item.user_id)) ||
    !["pending", "linked", "review"].includes(item.state) ||
    typeof item.active !== "boolean"
  ) {
    throw new ApiError(
      "invalid_response",
      "Resposta de contas incompatível. Atualize antes de repetir um convite.",
    );
  }
  return item;
}
function base(slug: string) {
  return "/api/v1/admin/shops/" + shopPathSegment(slug) + "/accounts";
}
function accountPath(slug: string, id: string) {
  if (!isID(id)) throw new Error("Conta inválida.");
  return base(slug) + "/" + id;
}
export async function listStaffAccounts(
  slug: string,
  signal?: AbortSignal,
): Promise<StaffAccount[]> {
  const value = await requestJSON(base(slug), { authenticated: true, signal });
  if (!Array.isArray(value))
    throw new ApiError("invalid_response", "Resposta de contas incompatível.");
  return value.map(account);
}
export async function inviteStaffAccount(
  slug: string,
  professional_id: string,
  email: string,
): Promise<StaffAccount> {
  return account(
    await requestJSON(base(slug), {
      authenticated: true,
      method: "POST",
      body: { professional_id, email },
    }),
  );
}
export async function replaceStaffAccount(
  slug: string,
  id: string,
  email: string,
): Promise<StaffAccount> {
  return account(
    await requestJSON(accountPath(slug, id) + "/replace", {
      authenticated: true,
      method: "POST",
      body: { email },
    }),
  );
}
export async function linkStaffAccount(
  slug: string,
  id: string,
  user_id: string,
): Promise<StaffAccount> {
  if (!isID(user_id)) throw new Error("Informe o UUID da conta em Supabase Authentication.");
  return account(
    await requestJSON(accountPath(slug, id) + "/link", {
      authenticated: true,
      method: "POST",
      body: { user_id },
    }),
  );
}
export async function setStaffAccountActive(
  slug: string,
  id: string,
  active: boolean,
): Promise<void> {
  await requestJSON(accountPath(slug, id) + "/status", {
    authenticated: true,
    method: "POST",
    body: { active },
  });
}
