import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

export type StaffProfessional = {
  id: string;
  name: string;
  active: boolean;
  service_ids: string[];
};
export type ProfessionalInput = {
  name: string;
  service_ids: string[];
  schedule: {
    interval_minutes: number;
    buffer_minutes: number;
    weekdays: number[];
    opens_at_minute: number;
    closes_at_minute: number;
  };
};
function professional(value: unknown): StaffProfessional {
  const item = value as StaffProfessional;
  if (
    !item ||
    typeof item.id !== "string" ||
    !item.id ||
    typeof item.name !== "string" ||
    !item.name.trim() ||
    typeof item.active !== "boolean" ||
    !Array.isArray(item.service_ids) ||
    item.service_ids.some((id) => typeof id !== "string" || !id)
  ) {
    throw new ApiError(
      "invalid_response",
      "Não conseguimos confirmar os dados da equipe. Confira a lista antes de cadastrar novamente.",
    );
  }
  return item;
}
function path(slug: string) {
  return "/api/v1/admin/shops/" + shopPathSegment(slug) + "/professionals";
}
export async function listStaffProfessionals(
  slug: string,
  signal?: AbortSignal,
): Promise<StaffProfessional[]> {
  const value = await requestJSON(path(slug), { authenticated: true, signal });
  if (!Array.isArray(value))
    throw new ApiError(
      "invalid_response",
      "Não conseguimos carregar a equipe. Atualize a lista e tente novamente.",
    );
  return value.map(professional);
}
export async function createProfessional(
  slug: string,
  input: ProfessionalInput,
): Promise<StaffProfessional> {
  return professional(
    await requestJSON(path(slug), { method: "POST", authenticated: true, body: input }),
  );
}
export function timeToMinutes(value: string): number {
  if (!/^\d{2}:\d{2}$/.test(value)) throw new Error("Informe o horário no formato HH:MM.");
  const [hours, minutes] = value.split(":").map(Number);
  if (hours > 23 || minutes > 59) throw new Error("Informe um horário válido.");
  return hours * 60 + minutes;
}
