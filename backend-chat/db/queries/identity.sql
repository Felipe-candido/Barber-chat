-- name: GetUserByID :one
SELECT
    id,
    display_name,
    active
FROM public.users
WHERE id = $1;

-- name: LockShopSettingsOwner :one
-- Do not lock shops here: the settings CAS obtains its own write lock. Shared
-- shop locks followed by upgrades would deadlock simultaneous settings writes.
SELECT m.user_id FROM public.shop_memberships m
JOIN public.users u ON u.id = m.user_id
WHERE m.user_id = $1 AND m.shop_id = $2 AND m.active = TRUE AND u.active = TRUE AND m.role = 'owner'
FOR SHARE OF m, u;


-- name: LockBillingOwner :one
SELECT m.user_id FROM public.shop_memberships m
JOIN public.users u ON u.id = m.user_id
WHERE m.user_id = $1 AND m.shop_id = $2 AND m.active = TRUE AND u.active = TRUE AND m.role = 'owner'
FOR SHARE OF m, u;

-- name: GetMembership :one
SELECT
    m.user_id,
    m.shop_id,
    (m.active AND (m.role = 'owner' OR (m.role = 'barber' AND COALESCE(p.active, FALSE))))::boolean AS active,
    m.role,
    COALESCE(m.professional_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS professional_id
FROM public.shop_memberships m
LEFT JOIN professionals p ON p.shop_id = m.shop_id AND p.id = m.professional_id
WHERE m.user_id = $1
  AND m.shop_id = $2;


-- name: ListMembershipsByUser :many
SELECT
    user_id,
    shop_id,
    active,
    role,
    COALESCE(professional_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS professional_id
FROM public.shop_memberships
WHERE user_id = $1
ORDER BY shop_id;


-- name: ListActiveShopsByUser :many
SELECT
    s.id AS shop_id,
    s.name,
    s.slug
FROM public.shop_memberships AS m
JOIN public.users AS u ON u.id = m.user_id
JOIN public.shops AS s ON s.id = m.shop_id
WHERE m.user_id = $1
  AND u.active = TRUE
  AND m.active = TRUE
  AND s.active = TRUE
  AND (m.role = 'owner' OR (m.role = 'barber' AND EXISTS (
      SELECT 1 FROM professionals p
      WHERE p.shop_id = m.shop_id AND p.id = m.professional_id AND p.active = TRUE
  )))
ORDER BY s.name, s.id;
