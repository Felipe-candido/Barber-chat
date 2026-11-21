-- name: LockEditableProfessional :one
SELECT p.id FROM professionals p JOIN shops s ON s.id=p.shop_id AND s.active
WHERE p.shop_id=$1 AND p.id=$2 FOR UPDATE OF p;

-- name: ReadEditableProfessional :one
SELECT p.id,p.name,p.active,
COALESCE(array_agg(ps.service_id ORDER BY ps.service_id) FILTER(WHERE ps.service_id IS NOT NULL),'{}'::uuid[])::uuid[] AS service_ids
FROM professionals p LEFT JOIN professional_services ps ON ps.shop_id=p.shop_id AND ps.professional_id=p.id
WHERE p.shop_id=$1 AND p.id=$2 GROUP BY p.id,p.name,p.active;

-- name: LockEditableProfessionalServices :many
SELECT s.id FROM services s WHERE s.shop_id=sqlc.arg(shop_id) AND s.id=ANY(sqlc.arg(service_ids)::uuid[])
AND ((s.active AND s.duration_minutes BETWEEN 1 AND 1440 AND s.price_cents<=9007199254740991)
 OR EXISTS(SELECT 1 FROM professional_services ps WHERE ps.shop_id=s.shop_id AND ps.professional_id=sqlc.arg(professional_id) AND ps.service_id=s.id))
ORDER BY s.id FOR SHARE OF s;

-- name: UpdateProfessionalProfile :exec
UPDATE professionals SET name=$3,active=$4 WHERE shop_id=$1 AND id=$2;

-- name: RemoveProfessionalAssignments :exec
DELETE FROM professional_services WHERE shop_id=$1 AND professional_id=$2;

-- name: ReadProfessionalWorkingHours :many
SELECT weekday,opens_at_minute,closes_at_minute FROM working_hours WHERE shop_id=$1 AND professional_id=$2
ORDER BY weekday,opens_at_minute,closes_at_minute;

-- name: ReplaceProfessionalSlotPolicy :exec
INSERT INTO schedule_settings(shop_id,professional_id,interval_minutes,buffer_minutes) VALUES($1,$2,$3,$4)
ON CONFLICT(shop_id,professional_id) DO UPDATE SET interval_minutes=EXCLUDED.interval_minutes,buffer_minutes=EXCLUDED.buffer_minutes;

-- name: RemoveProfessionalWorkingHours :exec
DELETE FROM working_hours WHERE shop_id=$1 AND professional_id=$2;
