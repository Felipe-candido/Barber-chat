-- name: GetShopSettings :one
SELECT s.id, s.name, s.slug, s.timezone, COALESCE(s.phone, '')::text AS phone,
    s.description, s.settings_version, (a.shop_id IS NOT NULL)::boolean AS has_address,
    COALESCE(a.postal_code, '')::text AS postal_code, COALESCE(a.street, '')::text AS street,
    COALESCE(a.number, '')::text AS number, COALESCE(a.complement, '')::text AS complement,
    COALESCE(a.district, '')::text AS district, COALESCE(a.city, '')::text AS city, COALESCE(a.state, '')::text AS state
FROM public.shops s
LEFT JOIN public.shop_addresses a ON a.shop_id = s.id
WHERE s.id = $1 AND s.active = TRUE;

-- name: ReplaceShopBasicSettings :execrows
UPDATE public.shops SET name = sqlc.arg(name), phone = NULLIF(sqlc.arg(phone)::text, ''),
    description = sqlc.arg(description), settings_version = settings_version + 1, updated_at = now()
WHERE id = sqlc.arg(shop_id) AND active = TRUE AND settings_version = sqlc.arg(expected_version);

-- name: UpsertShopSettingsAddress :exec
INSERT INTO public.shop_addresses (shop_id, postal_code, street, number, complement, district, city, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (shop_id) DO UPDATE SET postal_code = EXCLUDED.postal_code, street = EXCLUDED.street,
    number = EXCLUDED.number, complement = EXCLUDED.complement, district = EXCLUDED.district,
    city = EXCLUDED.city, state = EXCLUDED.state, updated_at = now();
