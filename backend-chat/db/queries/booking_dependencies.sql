-- name: ResolveBookingShop :one
SELECT id,slug,name,timezone,provisioning_source FROM shops WHERE slug = $1 AND active = TRUE FOR SHARE;

-- name: FindBookableSelection :one
SELECT s.shop_id,s.id AS service_id,p.id AS professional_id,p.name AS professional_name,
    s.name AS service_name,s.duration_minutes,s.price_cents,s.currency
FROM professional_services ps
JOIN services s ON s.shop_id = ps.shop_id AND s.id = ps.service_id
JOIN professionals p ON p.shop_id = ps.shop_id AND p.id = ps.professional_id
WHERE ps.shop_id = $1 AND ps.service_id = $2 AND ps.professional_id = $3
    AND s.active = TRUE AND p.active = TRUE
FOR SHARE OF s, p, ps;

-- name: ListBookableProfessionals :many
SELECT p.id,p.name FROM professionals p
JOIN professional_services ps ON ps.shop_id = p.shop_id AND ps.professional_id = p.id
JOIN services s ON s.shop_id = ps.shop_id AND s.id = ps.service_id
WHERE p.shop_id = $1 AND ps.service_id = $2 AND p.active = TRUE AND s.active = TRUE
    AND s.duration_minutes BETWEEN 1 AND 1440 AND s.price_cents <= 9007199254740991
ORDER BY p.name,p.id;

-- name: InsertBookingCustomer :execrows
INSERT INTO customers (id,shop_id,first_name,last_name,phone_e164,created_at)
VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (shop_id,phone_e164) DO NOTHING;

-- name: FindBookingCustomer :one
SELECT id FROM customers WHERE shop_id = $1 AND phone_e164 = $2;
