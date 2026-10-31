-- name: GetShopBillingState :one
-- One statement gives a coherent snapshot. Prefer the single current contract;
-- otherwise return the most recently recorded canceled contract for its reason.
SELECT s.id AS shop_id, s.active,
    COALESCE(sub.shop_id, '00000000-0000-0000-0000-000000000000'::uuid) AS subscription_shop_id,
    COALESCE(sub.status, '')::text AS status,
    COALESCE(sub.plan_key, '')::text AS plan_key,
    sub.paid_from, sub.paid_until, sub.grace_until,
    COALESCE(sub.cancel_at_period_end, FALSE) AS cancel_at_period_end,
    sub.confirmed_periods
FROM public.shops AS s
LEFT JOIN LATERAL (
    SELECT bs.shop_id, bs.status, bs.plan_key, bs.paid_from, bs.paid_until,
        bs.grace_until, bs.cancel_at_period_end,
        CASE WHEN bs.reconciliation_protocol=1 THEN COALESCE((
            SELECT jsonb_agg(jsonb_build_object('from',i.paid_from,'until',i.paid_until))
            FROM public.billing_paid_invoices i WHERE i.shop_id=bs.shop_id
                AND i.stripe_subscription_id=bs.stripe_subscription_id AND i.eligible=TRUE
        ),'[]'::jsonb) ELSE NULL::jsonb END AS confirmed_periods
    FROM public.billing_subscriptions AS bs
    WHERE bs.shop_id = s.id
    ORDER BY (bs.status <> 'canceled') DESC, bs.created_at DESC, bs.id DESC
    LIMIT 1
) AS sub ON TRUE
WHERE s.id = $1;

-- name: LockBillingOperationAccount :many
-- The webhook serializes financial writes on this row. Never create an account
-- while checking operation eligibility, and never hold it for provider I/O.
SELECT shop_id FROM public.billing_accounts WHERE shop_id=$1 FOR SHARE;
