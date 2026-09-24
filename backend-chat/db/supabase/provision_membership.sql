-- Run manually in the Supabase SQL Editor AFTER both migration streams.
-- Replace the three values below. Never insert passwords or rows into auth.users.
-- Repeating an active association is safe; disabled access is never reactivated.
BEGIN;

DO $$
DECLARE
    target_user_id UUID := '00000000-0000-0000-0000-000000000000';
    target_shop_slug TEXT := 'barbearia-do-felipe';
    target_display_name TEXT := 'SUBSTITUA PELO NOME';
    target_shop_id UUID;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_auth_user_fk'
          AND conrelid = 'public.users'::regclass
          AND confrelid = 'auth.users'::regclass
          AND convalidated
    ) THEN
        RAISE EXCEPTION 'Apply the Supabase identity migration first';
    END IF;
    IF target_user_id = '00000000-0000-0000-0000-000000000000'::uuid
       OR target_display_name = 'SUBSTITUA PELO NOME' THEN
        RAISE EXCEPTION 'Replace the user UUID, shop slug and display name first';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM auth.users WHERE id = target_user_id) THEN
        RAISE EXCEPTION 'Create the user in Supabase Authentication first';
    END IF;
    SELECT id INTO target_shop_id FROM public.shops
    WHERE slug = target_shop_slug AND active FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Active shop not found';
    END IF;

    INSERT INTO public.users (id, display_name)
    VALUES (target_user_id, target_display_name)
    ON CONFLICT (id) DO NOTHING;

    PERFORM 1 FROM public.users WHERE id = target_user_id AND active FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'User is disabled; review access before reactivating';
    END IF;

    INSERT INTO public.shop_memberships (user_id, shop_id)
    VALUES (target_user_id, target_shop_id)
    ON CONFLICT (user_id, shop_id) DO NOTHING;

    PERFORM 1 FROM public.shop_memberships
    WHERE user_id = target_user_id AND shop_id = target_shop_id AND active FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Membership is disabled; review access before reactivating';
    END IF;
END;
$$;

COMMIT;
