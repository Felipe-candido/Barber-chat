-- name: LockProfessionalCalendar :one
SELECT id FROM professionals WHERE shop_id = $1 AND id = $2 FOR UPDATE;

-- name: GetSlotPolicy :one
SELECT interval_minutes, buffer_minutes FROM schedule_settings WHERE shop_id = $1 AND professional_id = $2;

-- name: ListWorkingHours :many
SELECT opens_at_minute, closes_at_minute FROM working_hours
WHERE shop_id = $1 AND professional_id = $2 AND weekday = $3 ORDER BY opens_at_minute;

-- name: ListBusyIntervals :many
SELECT a.starts_at, a.ends_at FROM appointments a
WHERE a.shop_id = $1 AND a.professional_id = $2 AND a.status <> 'cancelled' AND a.starts_at < $4 AND a.ends_at > $3
UNION ALL
SELECT b.starts_at, b.ends_at FROM schedule_blocks b
WHERE b.shop_id = $1 AND b.professional_id = $2 AND b.starts_at < $4 AND b.ends_at > $3;

-- name: InsertAppointment :exec
INSERT INTO appointments (id, shop_id, customer_id, service_id, professional_id,
    service_name, duration_minutes, price_cents, currency, professional_name, shop_timezone,
    starts_at, ends_at, status, version, code_hash, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18);

-- name: FindAppointmentByCode :one
SELECT * FROM appointments WHERE shop_id = $1 AND code_hash = $2;

-- name: CancelAppointment :execrows
UPDATE appointments SET status = 'cancelled', version = $3, updated_at = $4
WHERE shop_id = $1 AND id = $2 AND version = $3 - 1 AND status IN ('scheduled','confirmed');

-- name: ClaimBookingRequest :execrows
INSERT INTO booking_requests (shop_id,key_hash,fingerprint,expires_at)
VALUES ($1,$2,$3,$4) ON CONFLICT (shop_id,key_hash) DO NOTHING;

-- name: GetBookingRequest :one
SELECT fingerprint,encrypted_receipt,expires_at FROM booking_requests WHERE shop_id = $1 AND key_hash = $2 FOR UPDATE;

-- name: CompleteBookingRequest :execrows
UPDATE booking_requests SET encrypted_receipt = $3 WHERE shop_id = $1 AND key_hash = $2 AND encrypted_receipt IS NULL;
