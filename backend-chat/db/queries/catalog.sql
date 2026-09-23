-- name: CreateService :one
INSERT INTO services (id, shop_id, name, description, duration_minutes, price_cents, currency, active)
SELECT sqlc.arg(id)::uuid, sh.id, sqlc.arg(name)::varchar, sqlc.arg(description)::text,
       sqlc.arg(duration_minutes)::integer, sqlc.arg(price_cents)::bigint, sqlc.arg(currency)::varchar, sqlc.arg(active)::boolean
FROM shops AS sh
WHERE sh.id = sqlc.arg(shop_id) AND sh.active = TRUE
RETURNING *;

-- name: ListActiveServicesByShopID :many
SELECT s.*
FROM services AS s
INNER JOIN shops AS sh ON sh.id = s.shop_id
WHERE s.shop_id = $1 AND s.active = TRUE AND sh.active = TRUE
ORDER BY s.name ASC, s.id ASC;
