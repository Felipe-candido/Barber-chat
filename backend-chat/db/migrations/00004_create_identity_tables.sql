-- +goose Up

-- IDs are copied from Supabase Auth; the provider-specific migration adds the FK.
-- No default UUID: provisioning must explicitly supply the external identity.
CREATE TABLE public.users (
    id UUID PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_display_name_not_empty CHECK (CHAR_LENGTH(TRIM(display_name)) > 0)
);

-- All active members have the same business permissions in the first release.
CREATE TABLE public.shop_memberships (
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    shop_id UUID NOT NULL REFERENCES public.shops(id) ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, shop_id)
);

CREATE INDEX shop_memberships_shop_idx ON public.shop_memberships (shop_id);

-- Business access will go through Go. No browser access policies are granted.
ALTER TABLE public.users ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.shop_memberships ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.users, public.shop_memberships FROM PUBLIC;

-- Remove Supabase default grants immediately, including TRUNCATE (not covered by
-- RLS). Plain PostgreSQL need not define these provider-specific browser roles.
-- +goose StatementBegin
DO $$
DECLARE
    browser_role TEXT;
BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon', 'authenticated')
    LOOP
        EXECUTE format('REVOKE ALL ON public.users, public.shop_memberships FROM %I', browser_role);
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down

-- On Supabase, roll back the provider migration first (separate version table).
DROP TABLE public.shop_memberships;
DROP TABLE public.users;
