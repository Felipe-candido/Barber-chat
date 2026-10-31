-- name: InsertRegistrationUser :exec
INSERT INTO public.users(id, display_name, active) VALUES ($1, $2, TRUE)
ON CONFLICT (id) DO NOTHING;

-- name: LockRegistrationUser :one
SELECT id, active FROM public.users WHERE id = $1 FOR UPDATE;

-- name: InsertInitialShopOwner :exec
INSERT INTO public.shop_memberships(user_id, shop_id, role, active)
VALUES ($1, $2, 'owner', TRUE);

-- name: HasActiveRegistrationOwner :one
SELECT EXISTS (SELECT 1 FROM public.shop_memberships
WHERE user_id = $1 AND shop_id = $2 AND role = 'owner' AND active = TRUE)::boolean AS allowed;

-- name: InsertSelfServiceShop :exec
INSERT INTO public.shops(id, name, slug, timezone, active, publication_requested, provisioning_source)
VALUES ($1, $2, $3, $4, TRUE, FALSE, 'self_service');

-- name: FindShopCreation :one
SELECT r.fingerprint, s.id, s.name, s.slug, s.timezone, s.active,
    COALESCE(a.postal_code, '')::text AS postal_code, COALESCE(a.street, '')::text AS street,
    COALESCE(a.number, '')::text AS number, COALESCE(a.complement, '')::text AS complement,
    COALESCE(a.district, '')::text AS district, COALESCE(a.city, '')::text AS city, COALESCE(a.state, '')::text AS state
FROM public.shop_creation_requests r JOIN public.shops s ON s.id = r.shop_id
LEFT JOIN public.shop_addresses a ON a.shop_id = s.id
WHERE r.user_id = $1 AND r.key_hash = $2;

-- name: InsertShopAddress :exec
INSERT INTO public.shop_addresses(shop_id, postal_code, street, number, complement, district, city, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: RecordShopCreation :exec
INSERT INTO public.shop_creation_requests(user_id, key_hash, fingerprint, shop_id)
VALUES ($1, $2, $3, $4);

-- name: GetPublicShopState :one
SELECT id, active, publication_requested, provisioning_source FROM public.shops WHERE slug = $1;
