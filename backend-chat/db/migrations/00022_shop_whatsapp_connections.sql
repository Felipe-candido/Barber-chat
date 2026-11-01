-- +goose Up
CREATE TABLE public.shop_whatsapp_connections (
    shop_id UUID PRIMARY KEY REFERENCES public.shops(id) ON DELETE RESTRICT,
    instance_name VARCHAR(35) NOT NULL UNIQUE CHECK(instance_name ~ '^bc_[0-9a-f]{32}$'),
    generation BIGINT NOT NULL DEFAULT 1 CHECK(generation>0),
    desired_connected BOOLEAN NOT NULL DEFAULT FALSE,
    state VARCHAR(20) NOT NULL DEFAULT 'not_configured' CHECK(state IN('not_configured','connecting','qr_required','connected','disconnected','unknown')),
    verified_at TIMESTAMPTZ CHECK(isfinite(verified_at)),
    last_connected_at TIMESTAMPTZ CHECK(isfinite(last_connected_at)),
    last_event_at TIMESTAMPTZ CHECK(isfinite(last_event_at)),
    next_check_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK(isfinite(next_check_at)),
    dirty BOOLEAN NOT NULL DEFAULT FALSE,
    error_code VARCHAR(32) NOT NULL DEFAULT '' CHECK(error_code IN('','activation_required','provider_credentials','provider_protocol','provider_unavailable')),
    operation_token UUID,
    lease_until TIMESTAMPTZ,
    CHECK((operation_token IS NULL AND lease_until IS NULL) OR (operation_token IS NOT NULL AND lease_until IS NOT NULL AND isfinite(lease_until)))
);
CREATE INDEX whatsapp_connections_due ON public.shop_whatsapp_connections(next_check_at);
ALTER TABLE public.shop_whatsapp_connections ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.shop_whatsapp_connections FROM PUBLIC;
-- +goose StatementBegin
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
      EXECUTE format('REVOKE ALL ON public.shop_whatsapp_connections FROM %I',browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.shop_whatsapp_connections) THEN
      RAISE EXCEPTION 'WhatsApp rollback requires no configured connections';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.shop_whatsapp_connections;
