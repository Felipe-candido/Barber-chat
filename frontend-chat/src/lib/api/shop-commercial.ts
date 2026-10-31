import { ApiError, requestJSON } from "./client";
import { shopPathSegment } from "./config";

const reasons = [
  "legacy_access",
  "subscribed",
  "grace_period",
  "subscription_required",
  "payment_pending",
  "payment_required",
  "subscription_suspended",
  "subscription_canceled",
] as const;
const blockers = [
  "operator_managed",
  ...reasons.filter((value) => !["legacy_access", "subscribed", "grace_period"].includes(value)),
  "address_required",
  "service_required",
  "professional_required",
  "schedule_required",
];
export type CommercialReason = (typeof reasons)[number];
export type CommercialStatus = {
  shop_id: string;
  slug: string;
  publication_managed: boolean;
  publication_requested: boolean;
  public_booking_enabled: boolean;
  can_publish: boolean;
  blockers: string[];
  readiness: { address: boolean; service: boolean; professional: boolean; schedule: boolean };
  subscription: {
    status: string;
    plan_key: string;
    operation_allowed: boolean;
    reason: CommercialReason;
    coverage_until: string | null;
    cancel_at_period_end: boolean;
  };
};

function invalid(): never {
  throw new ApiError(
    "invalid_response",
    "A API retornou uma situação comercial incompatível. Consulte novamente antes de publicar ou compartilhar.",
  );
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) invalid();
  return value as Record<string, unknown>;
}

export function parseCommercialStatus(
  value: unknown,
  shopID: string,
  slug: string,
): CommercialStatus {
  const item = record(value);
  const sub = record(item.subscription);
  const ready = record(item.readiness);
  if (
    item.shop_id !== shopID ||
    item.slug !== slug ||
    ![
      item.publication_managed,
      item.publication_requested,
      item.public_booking_enabled,
      item.can_publish,
      sub.operation_allowed,
      sub.cancel_at_period_end,
      ready.address,
      ready.service,
      ready.professional,
      ready.schedule,
    ].every((value) => typeof value === "boolean") ||
    typeof sub.status !== "string" ||
    !["none", "not_required", "pending", "active", "past_due", "suspended", "canceled"].includes(
      sub.status,
    ) ||
    typeof sub.plan_key !== "string" ||
    sub.plan_key.length > 100 ||
    !reasons.includes(sub.reason as CommercialReason) ||
    !Array.isArray(item.blockers) ||
    item.blockers.length > 8 ||
    item.blockers.some((value) => typeof value !== "string" || !blockers.includes(value)) ||
    (sub.coverage_until !== null &&
      (typeof sub.coverage_until !== "string" ||
        sub.coverage_until.length > 64 ||
        !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(
          sub.coverage_until,
        ) ||
        !Number.isFinite(Date.parse(sub.coverage_until))))
  )
    invalid();
  if (item.publication_managed) {
    if (
      sub.status === "not_required" ||
      sub.reason === "legacy_access" ||
      (sub.operation_allowed && !["active", "past_due"].includes(sub.status)) ||
      item.public_booking_enabled !== (item.publication_requested && sub.operation_allowed) ||
      item.can_publish !==
        (sub.operation_allowed &&
          ready.address &&
          ready.service &&
          ready.professional &&
          ready.schedule) ||
      (sub.operation_allowed
        ? sub.coverage_until === null ||
          !["subscribed", "grace_period"].includes(String(sub.reason))
        : sub.coverage_until !== null ||
          ["subscribed", "grace_period"].includes(String(sub.reason)))
    )
      invalid();
  } else if (
    sub.status !== "not_required" ||
    sub.reason !== "legacy_access" ||
    !sub.operation_allowed ||
    sub.coverage_until !== null ||
    !item.public_booking_enabled ||
    item.can_publish
  )
    invalid();
  // Return only this contract's fields, never provider IDs/URLs or other tenants.
  return {
    shop_id: shopID,
    slug,
    publication_managed: item.publication_managed as boolean,
    publication_requested: item.publication_requested as boolean,
    public_booking_enabled: item.public_booking_enabled as boolean,
    can_publish: item.can_publish as boolean,
    blockers: [...item.blockers] as string[],
    readiness: {
      address: ready.address as boolean,
      service: ready.service as boolean,
      professional: ready.professional as boolean,
      schedule: ready.schedule as boolean,
    },
    subscription: {
      status: sub.status,
      plan_key: sub.plan_key,
      operation_allowed: sub.operation_allowed as boolean,
      reason: sub.reason as CommercialReason,
      coverage_until: sub.coverage_until as string | null,
      cancel_at_period_end: sub.cancel_at_period_end as boolean,
    },
  };
}

// This only hides stale UI conservatively. The API remains the authority.
export function hasCurrentCoverage(status: CommercialStatus, now = Date.now()) {
  return (
    !status.publication_managed ||
    (status.subscription.operation_allowed &&
      status.subscription.coverage_until !== null &&
      Date.parse(status.subscription.coverage_until) > now)
  );
}

export async function getCommercialStatus(
  slug: string,
  shopID: string,
  userID: string,
  signal?: AbortSignal,
) {
  return parseCommercialStatus(
    await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/commercial-status`, {
      authenticated: true,
      expectedUserID: userID,
      signal,
    }),
    shopID,
    slug,
  );
}
export async function setShopPublication(
  slug: string,
  userID: string,
  requested: boolean,
  signal?: AbortSignal,
) {
  await requestJSON(`/api/v1/admin/shops/${shopPathSegment(slug)}/publication`, {
    method: "PUT",
    body: { requested },
    authenticated: true,
    expectedUserID: userID,
    signal,
    expectedNoContent: true,
  });
}
