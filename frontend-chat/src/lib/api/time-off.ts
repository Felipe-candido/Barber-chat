import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";
import { isID } from "./identity";

export type TimeOffBlock = {
  id: string;
  professional_id: string;
  professional_name: string;
  starts_at: string;
  ends_at: string;
};
export type TimeOffData = {
  today: string;
  max_date: string;
  shop_timezone: string;
  professionals: { id: string; name: string }[];
  blocks: TimeOffBlock[];
};
export type TimeOffAttempt = {
  id: string;
  professional: string;
  from_date: string;
  through_date: string;
};

function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "Não conseguimos confirmar as folgas. Atualize a lista antes de fazer outra alteração.",
  );
}
function block(value: unknown): TimeOffBlock {
  const b = value as TimeOffBlock;
  if (
    !b ||
    !isID(b.id) ||
    !isID(b.professional_id) ||
    typeof b.professional_name !== "string" ||
    !b.professional_name.trim() ||
    typeof b.starts_at !== "string" ||
    typeof b.ends_at !== "string" ||
    !/(?:Z|[+-]\d{2}:\d{2})$/.test(b.starts_at) ||
    !/(?:Z|[+-]\d{2}:\d{2})$/.test(b.ends_at) ||
    !Number.isFinite(Date.parse(b.starts_at)) ||
    !Number.isFinite(Date.parse(b.ends_at)) ||
    Date.parse(b.ends_at) <= Date.parse(b.starts_at)
  )
    invalid();
  return b;
}
function base(slug: string) {
  return `/api/v1/admin/shops/${shopPathSegment(slug)}`;
}
function path(slug: string, professional: string, id: string) {
  if (!isID(professional) || !isID(id)) invalid();
  return `${base(slug)}/professionals/${professional}/time-off/${id}`;
}
export async function listTimeOff(
  slug: string,
  ownProfessional: string | null,
  signal?: AbortSignal,
): Promise<TimeOffData> {
  const d = (await requestJSON(base(slug) + "/time-off", {
    authenticated: true,
    signal,
  })) as TimeOffData;
  if (
    !d ||
    !/^\d{4}-\d{2}-\d{2}$/.test(d.today) ||
    !/^\d{4}-\d{2}-\d{2}$/.test(d.max_date) ||
    d.max_date < d.today ||
    typeof d.shop_timezone !== "string" ||
    !d.shop_timezone ||
    !Array.isArray(d.professionals) ||
    !Array.isArray(d.blocks)
  )
    invalid();
  try {
    new Intl.DateTimeFormat("pt-BR", { timeZone: d.shop_timezone });
  } catch {
    invalid();
  }
  const ids = new Set<string>();
  for (const p of d.professionals) {
    if (
      !p ||
      !isID(p.id) ||
      typeof p.name !== "string" ||
      !p.name.trim() ||
      ids.has(p.id) ||
      (ownProfessional && p.id !== ownProfessional)
    )
      invalid();
    ids.add(p.id);
  }
  const seen = new Set<string>();
  for (const v of d.blocks) {
    const b = block(v);
    if (!ids.has(b.professional_id) || seen.has(b.id)) invalid();
    seen.add(b.id);
  }
  return d;
}
export async function saveTimeOff(slug: string, attempt: TimeOffAttempt, signal?: AbortSignal) {
  const saved = block(
    await requestJSON(path(slug, attempt.professional, attempt.id), {
      authenticated: true,
      method: "PUT",
      signal,
      body: { from_date: attempt.from_date, through_date: attempt.through_date },
    }),
  );
  if (saved.id !== attempt.id || saved.professional_id !== attempt.professional) invalid();
  return saved;
}
export async function deleteTimeOff(slug: string, b: TimeOffBlock, signal?: AbortSignal) {
  await requestJSON(path(slug, b.professional_id, b.id), {
    authenticated: true,
    method: "DELETE",
    expectedNoContent: true,
    signal,
  });
}
export function timeOffLabel(b: TimeOffBlock, timezone: string) {
  const day = new Intl.DateTimeFormat("pt-BR", { dateStyle: "medium", timeZone: timezone });
  const clock = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZone: timezone,
  });
  if (
    clock.format(new Date(b.starts_at)) === "00:00" &&
    clock.format(new Date(b.ends_at)) === "00:00"
  ) {
    const first = day.format(new Date(b.starts_at)),
      last = day.format(new Date(Date.parse(b.ends_at) - 1));
    return first === last ? first : `${first} a ${last}`;
  }
  const exact = new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "short",
    timeZone: timezone,
  });
  return `${exact.format(new Date(b.starts_at))} a ${exact.format(new Date(b.ends_at))}`;
}
