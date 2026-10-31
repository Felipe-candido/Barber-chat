-- +goose Up
-- Existing operator-provisioned units retain their current public behavior.
-- API registration explicitly sets self_service; the client cannot choose this.
-- Strict subscription/publication enforcement for legacy shops is a later rollout.
ALTER TABLE public.shops ADD COLUMN provisioning_source VARCHAR(16) NOT NULL DEFAULT 'manual'
    CHECK (provisioning_source IN ('manual', 'self_service'));

-- Durable retry identity: never expire/delete these records to reuse a key.
-- No passwords, access tokens, raw keys or raw registration bodies are stored.
CREATE TABLE public.shop_creation_requests (
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    key_hash BYTEA NOT NULL CHECK (octet_length(key_hash) = 32),
    fingerprint BYTEA NOT NULL CHECK (octet_length(fingerprint) = 32),
    shop_id UUID NOT NULL UNIQUE REFERENCES public.shops(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key_hash)
);
ALTER TABLE public.shop_creation_requests ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.shop_creation_requests FROM PUBLIC;
-- +goose StatementBegin
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon', 'authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.shop_creation_requests FROM %I', browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Do not discard retry records or accidentally make self-service drafts public.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM public.shop_creation_requests)
        OR EXISTS (SELECT 1 FROM public.shops WHERE provisioning_source = 'self_service') THEN
        RAISE EXCEPTION 'Registration rollback requires no self-service shops or creation records';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.shop_creation_requests;
ALTER TABLE public.shops DROP COLUMN provisioning_source;
