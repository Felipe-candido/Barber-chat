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