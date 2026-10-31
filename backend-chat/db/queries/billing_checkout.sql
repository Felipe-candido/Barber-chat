-- name: EnsureCheckoutAccount :exec
INSERT INTO public.billing_accounts (shop_id, stripe_mode) VALUES ($1, 'test')
ON CONFLICT (shop_id) DO NOTHING;

-- name: LockCheckoutAccount :one
SELECT shop_id, COALESCE(stripe_customer_id, '')::text AS customer_id, stripe_mode, created_at
FROM public.billing_accounts WHERE shop_id = $1 FOR UPDATE;

-- name: HasCurrentBillingSubscription :one
SELECT EXISTS(SELECT 1 FROM public.billing_subscriptions WHERE shop_id = $1 AND status <> 'canceled')::boolean;

-- name: FindPendingBillingCheckout :one
SELECT id, shop_id, intent, plan_key, state, created_at, protocol_version,
    COALESCE(stripe_price_id, '')::text AS price_id,
    COALESCE(return_url, '')::text AS return_url,
    COALESCE(stripe_session_id, '')::text AS session_id
FROM public.billing_checkout_attempts WHERE shop_id = $1 AND state IN ('preparing', 'open', 'review')
FOR UPDATE;

-- name: InsertBillingCheckout :exec
INSERT INTO public.billing_checkout_attempts
    (id, shop_id, intent, plan_key, state, created_at, protocol_version, stripe_price_id, return_url)
VALUES ($1, $2, $3, $4, 'preparing', $5, 1, $6, $7);

-- name: LinkBillingCheckoutCustomer :execrows
UPDATE public.billing_accounts SET stripe_customer_id = sqlc.arg(customer_id), updated_at = now()
WHERE shop_id = sqlc.arg(shop_id) AND stripe_mode = 'test'
    AND (stripe_customer_id IS NULL OR stripe_customer_id = sqlc.arg(customer_id));

-- name: RecordBillingCheckoutSession :execrows
UPDATE public.billing_checkout_attempts a
SET stripe_session_id = sqlc.arg(session_id), checkout_url = sqlc.narg(checkout_url),
    expires_at = sqlc.arg(expires_at), state = sqlc.arg(next_state), updated_at = now()
WHERE a.id = sqlc.arg(attempt_id) AND a.shop_id = sqlc.arg(shop_id) AND a.protocol_version = 1
    AND (a.stripe_session_id IS NULL OR a.stripe_session_id = sqlc.arg(session_id))
    AND (a.state IN ('preparing', 'open') OR (a.state = 'review' AND sqlc.arg(next_state) = 'review'))
    AND EXISTS (SELECT 1 FROM public.billing_accounts b WHERE b.shop_id = a.shop_id
        AND b.stripe_mode = 'test' AND b.stripe_customer_id = sqlc.arg(customer_id));
