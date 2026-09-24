-- +goose Up

-- Apply AFTER db/migrations, using the separate goose_supabase_version table.
-- Fail if Supabase Auth is absent; never silently skip identity integrity.
ALTER TABLE public.users
    ADD CONSTRAINT users_auth_user_fk
    FOREIGN KEY (id) REFERENCES auth.users(id) ON DELETE CASCADE;

-- Supabase may give these roles default table privileges. They must not manage
-- profiles or grant themselves shop access through the Data API.
REVOKE ALL ON public.users, public.shop_memberships FROM anon, authenticated;

-- +goose Down

ALTER TABLE public.users DROP CONSTRAINT users_auth_user_fk;
-- Do not restore broad Data API privileges during rollback.
