-- +goose Up
-- No assumptions about old external records. Unknown accounts/legacy attempts
-- are held for reconciliation, never silently reused in test mode.
ALTER TABLE public.billing_accounts ADD COLUMN stripe_mode VARCHAR(8) NOT NULL DEFAULT 'unknown'
    CHECK (stripe_mode IN ('unknown', 'test'));

ALTER TABLE public.billing_checkout_attempts
    ADD COLUMN protocol_version SMALLINT NOT NULL DEFAULT 0 CHECK (protocol_version IN (0, 1)),
    ADD COLUMN stripe_price_id VARCHAR(255),
    ADD COLUMN return_url VARCHAR(2048),
    ADD COLUMN checkout_url TEXT,
    ADD CONSTRAINT billing_checkout_frozen_request CHECK (
        protocol_version = 0 OR (
            stripe_price_id IS NOT NULL AND stripe_price_id ~ '^price_[a-zA-Z0-9]+$'
            AND return_url IS NOT NULL AND length(btrim(return_url)) > 0
        )
    );
-- Existing RLS and revoked browser privileges continue to cover new columns.

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM public.billing_accounts WHERE stripe_mode <> 'unknown')
        OR EXISTS (SELECT 1 FROM public.billing_checkout_attempts WHERE protocol_version <> 0) THEN
        RAISE EXCEPTION 'Checkout rollback requires no protocol-1 commercial records';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.billing_checkout_attempts
    DROP CONSTRAINT billing_checkout_frozen_request,
    DROP COLUMN checkout_url, DROP COLUMN return_url,
    DROP COLUMN stripe_price_id, DROP COLUMN protocol_version;
ALTER TABLE public.billing_accounts DROP COLUMN stripe_mode;
