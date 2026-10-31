-- +goose Up
ALTER TABLE public.billing_accounts ADD COLUMN reconciliation_revision BIGINT NOT NULL DEFAULT 0
    CHECK (reconciliation_revision >= 0);
ALTER TABLE public.billing_checkout_attempts ADD CONSTRAINT billing_checkout_shop_id UNIQUE (shop_id, id);
ALTER TABLE public.billing_subscriptions
    ADD COLUMN reconciliation_protocol SMALLINT NOT NULL DEFAULT 0 CHECK (reconciliation_protocol IN (0,1)),
    ADD COLUMN checkout_attempt_id UUID,
    ADD COLUMN stripe_price_id VARCHAR(255),
    ADD CONSTRAINT billing_subscription_attempt UNIQUE (checkout_attempt_id),
    ADD CONSTRAINT billing_subscription_shop_external UNIQUE (shop_id, stripe_subscription_id),
    ADD CONSTRAINT billing_subscription_attempt_tenant FOREIGN KEY (shop_id, checkout_attempt_id)
        REFERENCES public.billing_checkout_attempts(shop_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT billing_subscription_reconciled CHECK (reconciliation_protocol=0 OR
        (checkout_attempt_id IS NOT NULL AND stripe_price_id IS NOT NULL AND stripe_price_id ~ '^price_[A-Za-z0-9]+$'));

ALTER TABLE public.billing_webhook_events
    ADD COLUMN receipt_protocol SMALLINT NOT NULL DEFAULT 0 CHECK (receipt_protocol IN (0,1)),
    ADD COLUMN resource_kind VARCHAR(16),
    ADD COLUMN resource_id VARCHAR(255),
    ADD COLUMN event_created_at TIMESTAMPTZ,
    ADD COLUMN api_version VARCHAR(64),
    ADD CONSTRAINT billing_webhook_receipt CHECK (receipt_protocol=0 OR
        (NOT livemode AND resource_kind IS NOT NULL AND resource_kind IN ('checkout','subscription','invoice','ignored')
         AND resource_id IS NOT NULL AND event_created_at IS NOT NULL AND isfinite(event_created_at) AND api_version IS NOT NULL));

CREATE TABLE public.billing_paid_invoices (
    stripe_invoice_id VARCHAR(255) PRIMARY KEY CHECK (stripe_invoice_id ~ '^in_[A-Za-z0-9_]+$'),
    shop_id UUID NOT NULL,
    stripe_subscription_id VARCHAR(255) NOT NULL,
    stripe_price_id VARCHAR(255) NOT NULL,
    paid_from TIMESTAMPTZ NOT NULL CHECK (isfinite(paid_from)),
    paid_until TIMESTAMPTZ NOT NULL CHECK (isfinite(paid_until) AND paid_until > paid_from),
    amount_paid BIGINT NOT NULL CHECK (amount_paid > 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'BRL' CHECK (currency='BRL'),
    eligible BOOLEAN NOT NULL DEFAULT TRUE,
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (shop_id, stripe_subscription_id)
        REFERENCES public.billing_subscriptions(shop_id, stripe_subscription_id) ON DELETE RESTRICT
);
CREATE INDEX billing_paid_invoice_coverage ON public.billing_paid_invoices(shop_id, stripe_subscription_id, paid_until) WHERE eligible;
ALTER TABLE public.billing_paid_invoices ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.billing_paid_invoices FROM PUBLIC;
-- +goose StatementBegin
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.billing_paid_invoices FROM %I', browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM public.billing_paid_invoices)
       OR EXISTS (SELECT 1 FROM public.billing_webhook_events WHERE receipt_protocol=1)
       OR EXISTS (SELECT 1 FROM public.billing_subscriptions WHERE reconciliation_protocol=1)
       OR EXISTS (SELECT 1 FROM public.billing_accounts WHERE reconciliation_revision<>0) THEN
        RAISE EXCEPTION 'Webhook rollback requires no reconciliation records';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.billing_paid_invoices;
ALTER TABLE public.billing_webhook_events DROP CONSTRAINT billing_webhook_receipt,
    DROP COLUMN api_version, DROP COLUMN event_created_at, DROP COLUMN resource_id,
    DROP COLUMN resource_kind, DROP COLUMN receipt_protocol;
ALTER TABLE public.billing_subscriptions DROP CONSTRAINT billing_subscription_reconciled,
    DROP CONSTRAINT billing_subscription_attempt_tenant, DROP CONSTRAINT billing_subscription_shop_external,
    DROP CONSTRAINT billing_subscription_attempt,
    DROP COLUMN stripe_price_id, DROP COLUMN checkout_attempt_id, DROP COLUMN reconciliation_protocol;
ALTER TABLE public.billing_checkout_attempts DROP CONSTRAINT billing_checkout_shop_id;
ALTER TABLE public.billing_accounts DROP COLUMN reconciliation_revision;
