-- name: GetActiveAgendaShop :one
SELECT id, slug, name, timezone FROM shops WHERE id = $1 AND active = TRUE;

-- name: ListAdminAgenda :many
SELECT a.id, a.status, a.service_name, a.duration_minutes, a.price_cents,
       a.currency, a.professional_id, a.professional_name, a.shop_timezone,
       a.starts_at, a.ends_at,
       (c.first_name || ' ' || c.last_name)::text AS customer_name, c.phone_e164
FROM appointments a
JOIN customers c ON c.shop_id = a.shop_id AND c.id = a.customer_id
JOIN shops s ON s.id = a.shop_id AND s.active = TRUE
WHERE a.shop_id = sqlc.arg(shop_id)
  AND (sqlc.arg(professional_id)::uuid = '00000000-0000-0000-0000-000000000000'::uuid
       OR a.professional_id = sqlc.arg(professional_id)::uuid)
  AND a.starts_at >= sqlc.arg(from_at)::timestamptz
  AND a.starts_at < sqlc.arg(until_at)::timestamptz
  AND (NOT sqlc.arg(has_cursor)::boolean OR
       (a.starts_at, a.id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid))
ORDER BY a.starts_at, a.id
LIMIT sqlc.arg(page_limit)::integer;
