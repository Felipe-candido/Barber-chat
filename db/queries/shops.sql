-- name: GetActiveShopIDBySlug :one
SELECT id FROM shops WHERE slug = $1 AND active = TRUE;
