-- name: InsertBillingReceipt :exec
INSERT INTO public.billing_webhook_events
    (stripe_event_id,event_type,livemode,payload,state,processed_at,receipt_protocol,resource_kind,resource_id,event_created_at,api_version)
VALUES ($1,$2,FALSE,$3::jsonb,$4,$5,1,$6,$7,$8,$9)
ON CONFLICT (stripe_event_id) DO NOTHING;

-- name: LockBillingReceipt :one
SELECT stripe_event_id,event_type,state,receipt_protocol,
    COALESCE(resource_kind,'')::text AS resource_kind,COALESCE(resource_id,'')::text AS resource_id,
    event_created_at,COALESCE(api_version,'')::text AS api_version
FROM public.billing_webhook_events WHERE stripe_event_id=$1 FOR UPDATE;

-- name: LockReconciliationCustomer :one
SELECT shop_id,stripe_customer_id,stripe_mode,created_at,reconciliation_revision
FROM public.billing_accounts WHERE stripe_customer_id=$1 FOR UPDATE;

-- name: LockReconciliationShop :one
SELECT shop_id,stripe_customer_id,stripe_mode,created_at,reconciliation_revision
FROM public.billing_accounts WHERE shop_id=$1 FOR UPDATE;

-- name: FindReconciliationBinding :one
SELECT shop_id,stripe_customer_id,reconciliation_protocol,
    COALESCE(checkout_attempt_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS attempt_id
FROM public.billing_subscriptions WHERE stripe_subscription_id=$1;

-- name: LockReconciliationAttempt :one
SELECT id,shop_id,intent,plan_key,state,created_at,protocol_version,
    COALESCE(stripe_price_id,'')::text AS price_id,COALESCE(return_url,'')::text AS return_url,
    COALESCE(stripe_session_id,'')::text AS session_id
FROM public.billing_checkout_attempts WHERE id=$1 AND shop_id=$2 FOR UPDATE;

-- name: AdvanceReconciliationRevision :one
UPDATE public.billing_accounts SET reconciliation_revision=reconciliation_revision+1,updated_at=now()
WHERE shop_id=$1 AND reconciliation_revision<9223372036854775807 RETURNING reconciliation_revision;

-- name: StartBillingReceiptAttempt :exec
UPDATE public.billing_webhook_events SET shop_id=$2,attempts=attempts+1,failure_code=NULL
WHERE stripe_event_id=$1 AND state='pending';

-- name: FinishBillingReceipt :exec
UPDATE public.billing_webhook_events SET state=$2,processed_at=now(),failure_code=NULL
WHERE stripe_event_id=$1 AND state='pending';

-- name: NoteBillingReceiptFailure :exec
UPDATE public.billing_webhook_events SET failure_code='reconciliation_pending',next_attempt_at=now()+interval '1 minute'
WHERE stripe_event_id=$1 AND state='pending';

-- name: EnsureReconciledSubscription :execrows
INSERT INTO public.billing_subscriptions
    (id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,reconciliation_protocol,checkout_attempt_id,stripe_price_id)
VALUES ($1,$2,$3,$4,$5,$6,1,$7,$8)
ON CONFLICT (stripe_subscription_id) DO UPDATE SET status=EXCLUDED.status,paid_from=NULL,paid_until=NULL,
    grace_until=NULL,cancel_at_period_end=FALSE,updated_at=now()
WHERE billing_subscriptions.shop_id=EXCLUDED.shop_id AND billing_subscriptions.stripe_customer_id=EXCLUDED.stripe_customer_id
    AND billing_subscriptions.checkout_attempt_id=EXCLUDED.checkout_attempt_id AND billing_subscriptions.stripe_price_id=EXCLUDED.stripe_price_id
    AND billing_subscriptions.reconciliation_protocol=1;

-- name: ConfirmBillingPaidInvoice :execrows
INSERT INTO public.billing_paid_invoices
    (stripe_invoice_id,shop_id,stripe_subscription_id,stripe_price_id,paid_from,paid_until,amount_paid)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (stripe_invoice_id) DO UPDATE SET eligible=TRUE
WHERE billing_paid_invoices.shop_id=EXCLUDED.shop_id AND billing_paid_invoices.stripe_subscription_id=EXCLUDED.stripe_subscription_id
    AND billing_paid_invoices.stripe_price_id=EXCLUDED.stripe_price_id AND billing_paid_invoices.paid_from=EXCLUDED.paid_from
    AND billing_paid_invoices.paid_until=EXCLUDED.paid_until AND billing_paid_invoices.amount_paid=EXCLUDED.amount_paid;

-- name: InvalidateBillingPaidInvoice :exec
UPDATE public.billing_paid_invoices SET eligible=FALSE
WHERE stripe_invoice_id=$1 AND shop_id=$2 AND stripe_subscription_id=$3;

-- name: LatestConfirmedBillingPeriod :one
SELECT paid_from,paid_until FROM public.billing_paid_invoices
WHERE shop_id=$1 AND stripe_subscription_id=$2 AND eligible=TRUE ORDER BY paid_until DESC LIMIT 1;

-- name: UpdateReconciledSubscription :execrows
UPDATE public.billing_subscriptions SET status=$3,paid_from=$4,paid_until=$5,cancel_at_period_end=$6,
    grace_until=NULL,version=version+1,updated_at=now()
WHERE shop_id=$1 AND stripe_subscription_id=$2 AND reconciliation_protocol=1;

-- name: LinkReconciledCheckout :execrows
UPDATE public.billing_checkout_attempts a SET stripe_session_id=$3,expires_at=$4,state=sqlc.arg(next_state),checkout_url=NULL,updated_at=now()
WHERE a.shop_id=$1 AND a.id=$2 AND a.protocol_version=1 AND (a.stripe_session_id IS NULL OR a.stripe_session_id=$3)
    AND ((sqlc.arg(next_state)='completed' AND a.state IN ('preparing','open','review','completed'))
        OR (sqlc.arg(next_state)='expired' AND a.state IN ('preparing','open','review','expired')
            AND NOT EXISTS(SELECT 1 FROM public.billing_subscriptions bs WHERE bs.shop_id=a.shop_id
                AND bs.checkout_attempt_id=a.id AND bs.status<>'canceled')));
