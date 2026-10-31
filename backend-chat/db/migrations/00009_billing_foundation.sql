-- +goose Up
-- Foundation only: no automatic activation, backfill or changes to route guards.
-- Existing shops keep their active flag and access. New publication workflows
-- must validate readiness before setting this independent owner intent.
ALTER TABLE public.shops ADD COLUMN publication_requested BOOLEAN NOT NULL DEFAULT FALSE;

-- Reserve the row before creating the external Customer with an idempotent key.
-- A Customer belongs to one shop, so its portal cannot expose another tenant.
CREATE TABLE public.billing_accounts (
    shop_id UUID PRIMARY KEY REFERENCES public.shops(id) ON DELETE RESTRICT,
    stripe_customer_id VARCHAR(255) UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, stripe_customer_id),
    CHECK (stripe_customer_id IS NULL OR length(btrim(stripe_customer_id)) > 0)
);

CREATE TABLE public.billing_subscriptions (
    id UUID PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    shop_id UUID NOT NULL,
    stripe_customer_id VARCHAR(255) NOT NULL,
    stripe_subscription_id VARCHAR(255) NOT NULL UNIQUE CHECK (length(btrim(stripe_subscription_id)) > 0),
    plan_key VARCHAR(100) NOT NULL CHECK (length(btrim(plan_key)) > 0),
    status VARCHAR(16) NOT NULL CHECK (status IN ('pending', 'active', 'past_due', 'suspended', 'canceled')),
    paid_from TIMESTAMPTZ,
    paid_until TIMESTAMPTZ,
    grace_until TIMESTAMPTZ,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (shop_id, stripe_customer_id)
        REFERENCES public.billing_accounts(shop_id, stripe_customer_id) ON DELETE RESTRICT,
    CONSTRAINT billing_subscription_coverage CHECK (
        (paid_from IS NULL AND paid_until IS NULL)
        OR (paid_from IS NOT NULL AND paid_until IS NOT NULL
            AND isfinite(paid_from) AND isfinite(paid_until) AND paid_until > paid_from)
    ),
    CONSTRAINT billing_subscription_active_coverage CHECK (status <> 'active' OR paid_from IS NOT NULL),
    CONSTRAINT billing_subscription_pending_coverage CHECK (
        status <> 'pending' OR (paid_from IS NULL AND NOT cancel_at_period_end)
    ),
    CONSTRAINT billing_subscription_grace CHECK (
        grace_until IS NULL OR (status = 'past_due' AND paid_until IS NOT NULL
            AND isfinite(grace_until) AND grace_until > paid_until)
    )
);
-- Suspended or pending contracts must be recovered, not silently duplicated.
CREATE UNIQUE INDEX billing_subscription_current_shop ON public.billing_subscriptions(shop_id)
    WHERE status <> 'canceled';
CREATE INDEX billing_subscription_history ON public.billing_subscriptions(shop_id, created_at DESC, id DESC);

-- No booking/contact payload is stored here. A checkout return never executes
-- a booking or publishes a shop. Review holds uncertain external results closed
-- to a second checkout until the original attempt has been reconciled.
CREATE TABLE public.billing_checkout_attempts (
    id UUID PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    shop_id UUID NOT NULL REFERENCES public.billing_accounts(shop_id) ON DELETE RESTRICT,
    intent VARCHAR(16) NOT NULL CHECK (intent IN ('publish', 'manual_booking')),
    plan_key VARCHAR(100) NOT NULL CHECK (length(btrim(plan_key)) > 0),
    state VARCHAR(16) NOT NULL CHECK (state IN ('preparing', 'open', 'review', 'completed', 'expired')),
    stripe_session_id VARCHAR(255) UNIQUE CHECK (stripe_session_id IS NULL OR length(btrim(stripe_session_id)) > 0),
    expires_at TIMESTAMPTZ CHECK (expires_at IS NULL OR isfinite(expires_at)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (state NOT IN ('open', 'completed') OR (stripe_session_id IS NOT NULL AND expires_at IS NOT NULL))
);
CREATE UNIQUE INDEX billing_checkout_current_shop ON public.billing_checkout_attempts(shop_id)
    WHERE state IN ('preparing', 'open', 'review');

-- Transport inbox: shop_id may be unknown when an authenticated Stripe event
-- arrives before local linking. Resolve it from trusted provider associations;
-- do not grant access from metadata alone. Payload may contain personal data:
-- server-only access, no payload logging; retention belongs to webhook delivery.
CREATE TABLE public.billing_webhook_events (
    stripe_event_id VARCHAR(255) PRIMARY KEY CHECK (length(btrim(stripe_event_id)) > 0),
    shop_id UUID REFERENCES public.billing_accounts(shop_id) ON DELETE RESTRICT,
    event_type VARCHAR(150) NOT NULL CHECK (length(btrim(event_type)) > 0),
    livemode BOOLEAN NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    state VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'processed', 'ignored', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    failure_code VARCHAR(64),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    CHECK (payload->>'id' IS NOT NULL AND payload->>'id' = stripe_event_id),
    CHECK (payload->>'type' IS NOT NULL AND payload->>'type' = event_type),
    CHECK ((state IN ('processed', 'ignored')) = (processed_at IS NOT NULL))
);
CREATE INDEX billing_webhook_pending ON public.billing_webhook_events(next_attempt_at, stripe_event_id)
    WHERE state = 'pending';
CREATE INDEX billing_webhook_shop ON public.billing_webhook_events(shop_id);

-- No browser Data API access, even if Supabase default grants are configured.
-- +goose StatementBegin
DO $$
DECLARE table_name TEXT; browser_role TEXT;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['billing_accounts', 'billing_subscriptions',
        'billing_checkout_attempts', 'billing_webhook_events']
    LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', table_name);
        EXECUTE format('REVOKE ALL ON public.%I FROM PUBLIC', table_name);
        FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon', 'authenticated')
        LOOP
            EXECUTE format('REVOKE ALL ON public.%I FROM %I', table_name, browser_role);
        END LOOP;
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- Refuse to erase commercial records or publication intent in a real database.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM public.billing_accounts)
        OR EXISTS (SELECT 1 FROM public.billing_subscriptions)
        OR EXISTS (SELECT 1 FROM public.billing_checkout_attempts)
        OR EXISTS (SELECT 1 FROM public.billing_webhook_events)
        OR EXISTS (SELECT 1 FROM public.shops WHERE publication_requested) THEN
        RAISE EXCEPTION 'Billing rollback requires empty billing tables and unpublished shops';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.billing_webhook_events;
DROP TABLE public.billing_checkout_attempts;
DROP TABLE public.billing_subscriptions;
DROP TABLE public.billing_accounts;
ALTER TABLE public.shops DROP COLUMN publication_requested;
