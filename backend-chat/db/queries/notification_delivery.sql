-- name: GetDeliveryReference :one
SELECT appointment_id FROM notification_jobs WHERE shop_id=$1 AND id=$2;

-- name: LockNotificationDelivery :one
SELECT j.*,p.payload_version,p.payload_key_id,p.encrypted_payload,p.payload_expires_at
FROM notification_jobs j JOIN appointment_notification_plans p USING(shop_id,appointment_id,schedule_generation)
WHERE j.shop_id=$1 AND j.id=$2 FOR UPDATE OF j;

-- name: DeliveryDatabaseTime :one
SELECT clock_timestamp()::timestamptz;

-- name: LockDeliveryConnection :one
SELECT instance_name,state,desired_connected,verified_at,error_code FROM shop_whatsapp_connections WHERE shop_id=$1 FOR SHARE;

-- name: ClaimNotificationDelivery :execrows
UPDATE notification_jobs SET delivery_state='leased',delivery_token=$3,
 delivery_lease_until=clock_timestamp()+interval '90 seconds',delivery_reason=''
WHERE shop_id=$1 AND id=$2 AND state='pending' AND delivery_state IN('ready','leased');

-- name: StopNotificationBeforeSending :execrows
UPDATE notification_jobs SET delivery_state=sqlc.arg(delivery_state),delivery_reason=sqlc.arg(delivery_reason),delivery_token=NULL,delivery_lease_until=NULL
WHERE shop_id=sqlc.arg(shop_id) AND id=sqlc.arg(id) AND delivery_state IN('ready','leased');

-- name: StartNotificationDelivery :execrows
UPDATE notification_jobs SET delivery_state='sending',delivery_attempt_count=delivery_attempt_count+1,
 delivery_lease_until=clock_timestamp()+interval '90 seconds'
WHERE shop_id=$1 AND id=$2 AND state='pending' AND delivery_state='leased' AND delivery_token=$3
 AND delivery_lease_until>clock_timestamp() AND delivery_attempt_count<5;

-- name: InsertNotificationAttempt :exec
INSERT INTO notification_attempts(attempt_id,shop_id,job_id,attempt_number,instance_name,started_at)
SELECT j.delivery_token,j.shop_id,j.id,j.delivery_attempt_count,$4,clock_timestamp()
FROM notification_jobs j WHERE j.shop_id=$1 AND j.id=$2 AND j.delivery_token=$3 AND j.delivery_state='sending';

-- name: CompleteNotificationAttempt :execrows
UPDATE notification_attempts SET outcome=sqlc.arg(outcome),provider_message_id=sqlc.narg(provider_message_id),
 reason=sqlc.arg(reason),finished_at=clock_timestamp()
WHERE shop_id=sqlc.arg(shop_id) AND job_id=sqlc.arg(job_id) AND attempt_id=sqlc.arg(attempt_id)
 AND (outcome='sending' OR (outcome='unknown' AND sqlc.arg(outcome)::text='accepted'));

-- name: CompleteNotificationDelivery :execrows
UPDATE notification_jobs SET delivery_state=sqlc.arg(delivery_state),delivery_reason=sqlc.arg(delivery_reason),delivery_lease_until=NULL
WHERE shop_id=sqlc.arg(shop_id) AND id=sqlc.arg(id) AND delivery_token=sqlc.arg(delivery_token)
 AND (delivery_state='sending' OR (delivery_state='unknown' AND sqlc.arg(delivery_state)::text='accepted'));

-- name: LockNotificationDeliveryShop :one
SELECT active FROM shops WHERE id=$1 FOR SHARE;

-- name: LockNotificationDeliveryPermission :one
SELECT p.customer_id,p.allowed,p.revoked_at,c.phone_e164
FROM customer_booking_message_permissions p JOIN customers c ON c.shop_id=p.shop_id AND c.id=p.customer_id
WHERE p.shop_id=$1 AND p.appointment_id=$2 FOR SHARE OF p,c;

-- name: FindNotificationDeliveryProfessional :one
SELECT professional_id FROM appointments WHERE shop_id=$1 AND id=$2;

-- name: LockNotificationDeliveryBooking :one
SELECT customer_id,professional_id,status,starts_at,ends_at,service_name,professional_name,price_cents,currency,shop_timezone
FROM appointments WHERE shop_id=$1 AND id=$2 FOR UPDATE;
