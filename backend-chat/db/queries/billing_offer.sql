-- name: GetBillingCheckoutOfferState :one
-- A coherent read, without creating Customer/account/attempt rows or holding
-- locks during Stripe price reads. Reservation rechecks the selected price.
SELECT EXISTS(SELECT 1 FROM public.billing_subscriptions bs WHERE bs.shop_id=s.id AND bs.status<>'canceled')::boolean AS has_subscription,
    COALESCE(b.stripe_mode,'')::text AS account_mode, b.created_at AS account_created_at,
    COALESCE(b.stripe_customer_id,'')::text AS customer_id,
    COALESCE(a.id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS attempt_id,
    COALESCE(a.protocol_version,0)::smallint AS protocol_version,
    COALESCE(a.intent,'')::text AS intent, COALESCE(a.plan_key,'')::text AS plan_key,
    COALESCE(a.stripe_price_id,'')::text AS price_id, COALESCE(a.state,'')::text AS state,
    COALESCE(a.stripe_session_id,'')::text AS session_id, a.created_at AS attempt_created_at
FROM public.shops s
LEFT JOIN public.billing_accounts b ON b.shop_id=s.id
LEFT JOIN public.billing_checkout_attempts a ON a.shop_id=s.id AND a.state IN ('preparing','open','review')
WHERE s.id=$1;
