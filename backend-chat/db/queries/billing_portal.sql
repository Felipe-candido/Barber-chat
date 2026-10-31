-- name: FindBillingPortalAccount :one
-- Past-due and canceled contracts still need card recovery and invoice history.
-- An abandoned Checkout/Customer alone does not expose a portal.
SELECT a.shop_id, a.stripe_customer_id, a.stripe_mode, a.created_at
FROM public.billing_accounts a
WHERE a.shop_id = $1 AND a.stripe_mode = 'test' AND a.stripe_customer_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM public.billing_subscriptions s
    JOIN public.billing_checkout_attempts c ON c.shop_id = s.shop_id AND c.id = s.checkout_attempt_id
    WHERE s.shop_id = a.shop_id AND s.stripe_customer_id = a.stripe_customer_id
      AND s.reconciliation_protocol = 1 AND c.protocol_version = 1
      AND c.stripe_price_id = s.stripe_price_id
  );
