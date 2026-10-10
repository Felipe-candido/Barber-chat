-- +goose Up
-- Records provisioning attempts before contacting Auth; never stores passwords.
CREATE TABLE staff_accounts (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL,
    professional_id UUID NOT NULL,
    email VARCHAR(254) NOT NULL CHECK (length(btrim(email)) > 0),
    auth_user_id UUID REFERENCES public.users(id) ON DELETE SET NULL,
    state VARCHAR(10) NOT NULL CHECK (state IN ('pending', 'linked', 'review')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, professional_id),
    FOREIGN KEY (shop_id, professional_id) REFERENCES professionals(shop_id, id)
);
ALTER TABLE staff_accounts ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON staff_accounts FROM PUBLIC;
-- +goose StatementBegin
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.staff_accounts FROM %I', browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE staff_accounts;
