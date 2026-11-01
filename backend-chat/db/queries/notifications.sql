-- name: GetBookingNotificationPolicy :one
SELECT enabled,reminder_lead_minutes,version FROM shop_notification_settings WHERE shop_id=$1 FOR SHARE;

-- name: InsertBookingNotificationPlan :exec
INSERT INTO appointment_notification_plans(shop_id,appointment_id,schedule_generation,policy_version,
    skip_reason,reminder_skip_reason,payload_version,payload_key_id,encrypted_payload,payload_expires_at,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: InsertBookingNotificationJob :exec
INSERT INTO notification_jobs(id,shop_id,appointment_id,schedule_generation,kind,due_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7);
