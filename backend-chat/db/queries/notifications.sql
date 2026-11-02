-- name: GetBookingNotificationPolicy :one
SELECT enabled,reminder_lead_minutes,version,delivery_mode FROM shop_notification_settings WHERE shop_id=$1 FOR SHARE;

-- name: InsertBookingNotificationPlan :exec
INSERT INTO appointment_notification_plans(shop_id,appointment_id,schedule_generation,policy_version,
    skip_reason,reminder_skip_reason,payload_version,payload_key_id,encrypted_payload,payload_expires_at,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: InsertBookingNotificationJob :exec
INSERT INTO notification_jobs(id,shop_id,appointment_id,schedule_generation,kind,due_at,expires_at,dispatch_mode)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: CreateNotificationPolicy :execrows
INSERT INTO shop_notification_settings(shop_id,enabled,reminder_lead_minutes,delivery_mode)
VALUES($1,$2,$3,$4) ON CONFLICT(shop_id) DO NOTHING;

-- name: UpdateNotificationPolicy :execrows
UPDATE shop_notification_settings SET enabled=$2,reminder_lead_minutes=$3,delivery_mode=$4,version=version+1,updated_at=clock_timestamp()
WHERE shop_id=$1 AND version=$5;

-- name: FindAppointmentNotificationJobs :many
SELECT j.id,j.kind,j.delivery_state,j.delivery_reason,j.delivery_lease_until,j.dispatch_mode,j.due_at,j.expires_at,j.state,
 COALESCE((SELECT a.receipt_state FROM notification_attempts a WHERE a.shop_id=j.shop_id AND a.job_id=j.id ORDER BY a.attempt_number DESC LIMIT 1),0)::smallint AS receipt_state
FROM notification_jobs j WHERE j.shop_id=$1 AND j.appointment_id=$2 ORDER BY j.kind;
