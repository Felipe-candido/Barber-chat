-- Run manually in the Supabase SQL Editor AFTER both migration streams.
-- Edit only the five values in DECLARE. Use an existing Supabase Auth user UUID.
-- This creates application records, never a password or a row in auth.users.
-- Repeatable for matching active records; suspended access is never reactivated.
-- On failure, run ROLLBACK before retrying in the same SQL session.
BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

DO $$
DECLARE
    target_user_id UUID := 'c5b753f8-ceae-4ec1-b0ec-1594999aa83d';
    target_display_name TEXT := 'Felipe';
    target_shop_name TEXT := 'Barbearia do Felipe';
    target_shop_slug TEXT := 'barbaria-felipe';
    target_shop_timezone TEXT := 'America/Sao_Paulo';
    target_shop RECORD;
BEGIN
    IF target_user_id IS NULL
       OR target_user_id = '00000000-0000-0000-0000-000000000000'::uuid THEN
        RAISE EXCEPTION 'Replace target_user_id with an existing Supabase Auth UUID';
    END IF;

    target_display_name := btrim(target_display_name);
    target_shop_name := btrim(target_shop_name);

    IF target_display_name IS NULL
       OR char_length(target_display_name) NOT BETWEEN 1 AND 100
       OR target_display_name = 'SUBSTITUA PELO NOME DO USUARIO' THEN
        RAISE EXCEPTION 'Provide a user display name between 1 and 100 characters';
    END IF;
    IF target_shop_name IS NULL
       OR char_length(target_shop_name) NOT BETWEEN 1 AND 150
       OR target_shop_name = 'SUBSTITUA PELO NOME DA BARBEARIA' THEN
        RAISE EXCEPTION 'Provide a shop name between 1 and 150 characters';
    END IF;
    IF target_shop_slug IS NULL
       OR char_length(target_shop_slug) NOT BETWEEN 1 AND 100
       OR target_shop_slug !~ '^[a-z0-9]+(-[a-z0-9]+)*$'
       OR target_shop_slug = 'substitua-o-slug' THEN
        RAISE EXCEPTION 'Provide a lowercase shop slug with letters, digits and single hyphens';
    END IF;
    IF target_shop_timezone IS NULL
       OR char_length(target_shop_timezone) > 100
       OR NOT EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = target_shop_timezone) THEN
        RAISE EXCEPTION 'Provide a recognized timezone, such as America/Sao_Paulo';
    END IF;

    IF to_regclass('public.shops') IS NULL
       OR to_regclass('public.users') IS NULL
       OR to_regclass('public.shop_memberships') IS NULL THEN
        RAISE EXCEPTION 'Apply db/migrations before provisioning';
    END IF;
    IF to_regclass('auth.users') IS NULL THEN
        RAISE EXCEPTION 'Run this script in Supabase, not in the plain PostgreSQL Compose database';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_auth_user_fk'
          AND conrelid = 'public.users'::regclass
          AND confrelid = 'auth.users'::regclass
          AND contype = 'f'
          AND convalidated
    ) THEN
        RAISE EXCEPTION 'Apply db/supabase/migrations with its separate version table first';
    END IF;

    -- Keep the Auth identity from being deleted while provisioning its profile.
    PERFORM 1 FROM auth.users WHERE id = target_user_id FOR KEY SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'User UUID not found in this Supabase project; create the Auth account first';
    END IF;

    INSERT INTO public.shops (id, name, slug, timezone)
    VALUES (gen_random_uuid(), target_shop_name, target_shop_slug, target_shop_timezone)
    ON CONFLICT (slug) DO NOTHING;

    SELECT id, name, timezone, active INTO target_shop
    FROM public.shops WHERE slug = target_shop_slug FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Shop could not be resolved';
    END IF;
    IF NOT target_shop.active THEN
        RAISE EXCEPTION 'Shop is disabled; review access before reactivating';
    END IF;
    IF target_shop.name IS DISTINCT FROM target_shop_name
       OR target_shop.timezone IS DISTINCT FROM target_shop_timezone THEN
        RAISE EXCEPTION 'Slug already belongs to a shop with different data; review the existing shop';
    END IF;

    -- An existing active profile is reused without overwriting its display name.
    INSERT INTO public.users (id, display_name)
    VALUES (target_user_id, target_display_name)
    ON CONFLICT (id) DO NOTHING;

    PERFORM 1 FROM public.users WHERE id = target_user_id AND active FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'User is disabled; review access before reactivating';
    END IF;

    INSERT INTO public.shop_memberships (user_id, shop_id)
    VALUES (target_user_id, target_shop.id)
    ON CONFLICT (user_id, shop_id) DO NOTHING;

    PERFORM 1 FROM public.shop_memberships
    WHERE user_id = target_user_id AND shop_id = target_shop.id AND active FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Membership is disabled; review access before reactivating';
    END IF;

    -- Transaction-local settings let the result query reuse the inputs once.
    PERFORM set_config('barber_chat.provisioned_user_id', target_user_id::text, true);
    PERFORM set_config('barber_chat.provisioned_shop_id', target_shop.id::text, true);
END;
$$;

SELECT u.id AS user_id, u.display_name, u.active AS user_active,
       s.id AS shop_id, s.name AS shop_name, s.slug, s.timezone,
       s.active AS shop_active, m.active AS membership_active
FROM public.users AS u
JOIN public.shop_memberships AS m ON m.user_id = u.id
JOIN public.shops AS s ON s.id = m.shop_id
WHERE u.id = current_setting('barber_chat.provisioned_user_id')::uuid
  AND s.id = current_setting('barber_chat.provisioned_shop_id')::uuid;

COMMIT;
