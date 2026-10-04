-- name: LockActiveProfessionalShop :one
SELECT id FROM shops WHERE id=$1 AND active=TRUE FOR SHARE;

-- name: LockProfessionalServices :many
SELECT id FROM services WHERE shop_id=$1 AND id=ANY(sqlc.arg(service_ids)::uuid[]) AND active=TRUE
    AND duration_minutes BETWEEN 1 AND 1440 AND price_cents <= 9007199254740991
ORDER BY id FOR SHARE;

-- name: InsertProfessional :exec
INSERT INTO professionals (id,shop_id,name,active) VALUES ($1,$2,$3,$4);

-- name: AssignProfessionalServices :exec
INSERT INTO professional_services (shop_id,professional_id,service_id)
SELECT sqlc.arg(shop_id), sqlc.arg(professional_id), unnest(sqlc.arg(service_ids)::uuid[]);

-- name: ListShopProfessionals :many
SELECT p.id,p.name,p.active,
    COALESCE(array_agg(ps.service_id ORDER BY ps.service_id) FILTER (WHERE ps.service_id IS NOT NULL),'{}'::uuid[])::uuid[] AS service_ids
FROM professionals p
JOIN shops s ON s.id=p.shop_id AND s.active=TRUE
LEFT JOIN professional_services ps ON ps.shop_id=p.shop_id AND ps.professional_id=p.id
WHERE p.shop_id=$1
GROUP BY p.id,p.name,p.active ORDER BY p.name,p.id;

-- name: InsertInitialSlotPolicy :exec
INSERT INTO schedule_settings (shop_id,professional_id,interval_minutes,buffer_minutes) VALUES ($1,$2,$3,$4);

-- name: InsertInitialWorkingHours :exec
INSERT INTO working_hours (id,shop_id,professional_id,weekday,opens_at_minute,closes_at_minute) VALUES ($1,$2,$3,$4,$5,$6);

-- name: InsertInitialWorkingWeek :exec
INSERT INTO working_hours (id,shop_id,professional_id,weekday,opens_at_minute,closes_at_minute)
SELECT gen_random_uuid(), sqlc.arg(shop_id), sqlc.arg(professional_id),
    unnest(sqlc.arg(weekdays)::integer[]), sqlc.arg(opens_at_minute), sqlc.arg(closes_at_minute);
