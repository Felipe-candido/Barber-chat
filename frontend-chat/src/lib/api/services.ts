import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

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
      "Não conseguimos carregar este serviço. Atualize a lista e tente novamente.",
    );
  return s;
}
export async function listServices(slug: string, signal?: AbortSignal): Promise<Service[]> {
  const payload = await requestJSON("/api/v1/public/shops/" + shopPathSegment(slug) + "/services", {
    signal,
  });
  if (!Array.isArray(payload))
    throw new ApiError(
      "invalid_response",
      "Não conseguimos carregar os serviços. Atualize a lista e tente novamente.",
    );
  return payload.map(service);
}

export async function listAdminServices(slug: string, signal?: AbortSignal): Promise<Service[]> {
  const payload = await requestJSON("/api/v1/admin/shops/" + shopPathSegment(slug) + "/services", {
    authenticated: true,
    signal,
  });
  if (!Array.isArray(payload))
    throw new ApiError(
      "invalid_response",
      "Não conseguimos carregar os serviços. Atualize a lista e tente novamente.",
    );
  return payload.map(service);
}

function serviceBody(input: CreateServiceInput): CreateServiceInput {
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
  return {
    name,
    description: input.description,
    duration_minutes: input.duration_minutes,
    price_cents: input.price_cents,
  };
}

export async function createService(
  slug: string,
  input: CreateServiceInput,
  signal?: AbortSignal,
): Promise<Service> {
  const payload = await requestJSON("/api/v1/admin/shops/" + shopPathSegment(slug) + "/services", {
    method: "POST",
    authenticated: true,
    signal,
    body: serviceBody(input),
  });
  try {
    return service(payload);
  } catch {
    throw new ApiError(
      "invalid_response",
      "O serviço pode ter sido salvo. Atualize a lista para conferir antes de cadastrar novamente.",
    );
  }
}

export async function updateService(
  slug: string,
  id: string,
  input: CreateServiceInput,
  signal?: AbortSignal,
): Promise<Service> {
  return service(
    await requestJSON(servicePath(slug, id), {
      method: "PUT",
      authenticated: true,
      signal,
      body: serviceBody(input),
    }),
  );
}

export async function deleteService(slug: string, id: string, signal?: AbortSignal): Promise<void> {
  await requestJSON(servicePath(slug, id), {
    method: "DELETE",
    authenticated: true,
    signal,
    expectedNoContent: true,
  });
}

function servicePath(slug: string, id: string) {
  return "/api/v1/admin/shops/" + shopPathSegment(slug) + "/services/" + encodeURIComponent(id);
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
      "Esse preço é muito alto. Confira o valor e tente novamente.",
    );
  return Number(amount);
}
