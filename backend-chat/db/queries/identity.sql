-- name: GetUserByID :one
SELECT
    id,
    display_name,
    active
FROM public.users
WHERE id = $1;


-- name: GetMembership :one
SELECT
    user_id,
    shop_id,
    active
FROM public.shop_memberships
WHERE user_id = $1
  AND shop_id = $2;


-- name: ListMembershipsByUser :many
SELECT
    user_id,
    shop_id,
    active
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
ORDER BY s.name, s.id;
