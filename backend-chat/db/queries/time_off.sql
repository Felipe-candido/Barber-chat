-- name: ListTimeOffProfessionals :many
SELECT id, name FROM professionals WHERE shop_id=$1 AND active
AND (sqlc.arg(professional_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR id=sqlc.arg(professional_id))
ORDER BY name,id;

-- name: ListTimeOff :many
SELECT b.id,b.professional_id,p.name AS professional_name,b.starts_at,b.ends_at
FROM schedule_blocks b JOIN professionals p ON p.shop_id=b.shop_id AND p.id=b.professional_id AND p.active
WHERE b.shop_id=$1 AND b.ends_at > sqlc.arg(now_at)
AND (sqlc.arg(professional_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR b.professional_id=sqlc.arg(professional_id))
ORDER BY b.starts_at,b.id;

-- name: FindTimeOff :one
SELECT b.id,b.professional_id,p.name AS professional_name,b.starts_at,b.ends_at
FROM schedule_blocks b JOIN professionals p ON p.shop_id=b.shop_id AND p.id=b.professional_id AND p.active
WHERE b.shop_id=$1 AND b.professional_id=$2 AND b.id=$3;

-- name: TimeOffHasAppointments :one
SELECT EXISTS(SELECT 1 FROM appointments WHERE shop_id=$1 AND professional_id=$2
AND status <> 'cancelled' AND starts_at < sqlc.arg(ends_at) AND ends_at > sqlc.arg(starts_at));

-- name: TimeOffHasBlocks :one
SELECT EXISTS(SELECT 1 FROM schedule_blocks WHERE shop_id=$1 AND professional_id=$2
AND starts_at < sqlc.arg(ends_at) AND ends_at > sqlc.arg(starts_at));

-- name: InsertTimeOff :execrows
INSERT INTO schedule_blocks (id,shop_id,professional_id,starts_at,ends_at)
SELECT sqlc.arg(id)::uuid, p.shop_id, p.id, sqlc.arg(starts_at)::timestamptz, sqlc.arg(ends_at)::timestamptz
FROM professionals p WHERE p.shop_id=sqlc.arg(shop_id) AND p.id=sqlc.arg(professional_id) AND p.active
ON CONFLICT (id) DO NOTHING;

-- name: DeleteTimeOff :exec
DELETE FROM schedule_blocks WHERE shop_id=$1 AND professional_id=$2 AND id=$3;
