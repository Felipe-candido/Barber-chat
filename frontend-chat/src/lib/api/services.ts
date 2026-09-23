import { ApiError, requestJSON } from "./client";

// These fields mirror serviceResponse and createServiceRequest in the Go handler.
export type Service = {
  id: string;
  name: string;
  description: string;
  duration_minutes: number;
  price_cents: number;
  currency: "BRL";
  active: boolean;
};
export type CreateServiceInput = Pick<
  Service,
  "name" | "description" | "duration_minutes" | "price_cents"
>;
function service(value: unknown): Service {
  const s = value as Service;
  if (
    !s ||
    typeof s.id !== "string" ||
    !s.id ||
    typeof s.name !== "string" ||
    typeof s.description !== "string" ||
    !Number.isInteger(s.duration_minutes) ||
    s.duration_minutes <= 0 ||
    s.duration_minutes > 2147483647 ||
    !Number.isSafeInteger(s.price_cents) ||
    s.price_cents < 0 ||
    s.currency !== "BRL" ||
    typeof s.active !== "boolean"
  )
    throw new ApiError(
      "invalid_response",
      "O formato de serviço retornado pela API não é compatível com a interface.",
    );
  return s;
}
export async function listServices(slug: string, signal?: AbortSignal): Promise<Service[]> {
  if (!slug.trim())
    throw new ApiError(
      "configuration",
      "Configure NEXT_PUBLIC_SHOP_SLUG para carregar os serviços da barbearia.",
    );
  const payload = await requestJSON(
    "/api/v1/public/shops/" + encodeURIComponent(slug) + "/services",
    { signal },
  );
  if (!Array.isArray(payload))
    throw new ApiError("invalid_response", "A API deveria retornar uma lista de serviços.");
  return payload.map(service);
}
export async function createService(input: CreateServiceInput): Promise<Service> {
  const name = input.name.trim();
  if (
    !name ||
    Array.from(name).length > 100 ||
    !Number.isInteger(input.duration_minutes) ||
    input.duration_minutes < 1 ||
    input.duration_minutes > 2147483647 ||
    !Number.isSafeInteger(input.price_cents) ||
    input.price_cents < 0
  )
    throw new ApiError(
      "invalid_service",
      "Informe um nome de até 100 caracteres, duração inteira positiva e preço válido.",
    );
  // Explicit mapping prevents tenant, category and active fields from leaking into the request.
  const payload = await requestJSON("/api/v1/admin/services", {
    method: "POST",
    body: {
      name,
      description: input.description,
      duration_minutes: input.duration_minutes,
      price_cents: input.price_cents,
    },
  });
  try {
    return service(payload);
  } catch {
    throw new ApiError(
      "invalid_response",
      "A API aceitou o envio, mas a resposta é incompatível. Atualize a lista antes de cadastrar novamente.",
    );
  }
}
export function priceToCents(value: string): number {
  const normalized = value.trim().replace(",", ".");
  if (!/^\d+(\.\d{1,2})?$/.test(normalized))
    throw new ApiError(
      "invalid_price",
      "Informe um preço positivo ou zero com até duas casas decimais.",
    );
  const [whole, fraction = ""] = normalized.split(".");
  const amount = BigInt(whole) * BigInt(100) + BigInt(fraction.padEnd(2, "0"));
  if (amount > BigInt(Number.MAX_SAFE_INTEGER))
    throw new ApiError(
      "invalid_price",
      "O preço informado ultrapassa o limite de precisão da interface.",
    );
  return Number(amount);
}
