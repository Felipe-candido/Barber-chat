-- name: GetShopAppointmentCompletion :one
SELECT * FROM shop_appointment_completion_settings WHERE shop_id=$1;

-- name: ReplaceShopAppointmentCompletion :execrows
UPDATE shop_appointment_completion_settings SET mode=sqlc.arg(mode)::text,version=version+1,updated_at=clock_timestamp(),
 automatic_since=CASE WHEN sqlc.arg(mode)::text='manual' THEN NULL
  WHEN mode='automatic' THEN automatic_since ELSE clock_timestamp() END
WHERE shop_id=sqlc.arg(shop_id) AND version=sqlc.arg(expected_version) AND version<9007199254740991;
-- name: LockAppointmentCompletionAccess :one
SELECT m.user_id FROM shop_memberships m JOIN users u ON u.id=m.user_id JOIN shops s ON s.id=m.shop_id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.active AND u.active AND s.active
 AND m.role='owner' FOR SHARE OF s,u,m;

