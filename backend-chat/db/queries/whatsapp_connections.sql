-- name: LockWhatsAppConnectionOwner :one
SELECT m.user_id FROM shop_memberships m JOIN users u ON u.id=m.user_id JOIN shops s ON s.id=m.shop_id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.active AND u.active AND s.active AND m.role='owner'
FOR SHARE OF m,u,s;

-- name: ReadWhatsAppConnection :one
SELECT *,clock_timestamp()::timestamptz AS observed_at FROM shop_whatsapp_connections WHERE shop_id=$1 FOR UPDATE;

-- name: EnsureWhatsAppConnection :exec
INSERT INTO shop_whatsapp_connections(shop_id,instance_name) VALUES($1,$2) ON CONFLICT(shop_id) DO NOTHING;

-- name: ClaimOwnerWhatsAppAction :one
UPDATE shop_whatsapp_connections SET generation=generation+1,desired_connected=$2,state='connecting',
    operation_token=$3,lease_until=clock_timestamp()+interval '90 seconds',error_code=''
WHERE shop_id=$1 AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND generation<9223372036854775807
RETURNING *,clock_timestamp()::timestamptz AS observed_at;

-- name: ClaimDueWhatsAppConnection :one
WITH due AS (
 SELECT shop_id FROM shop_whatsapp_connections WHERE (lease_until IS NULL OR lease_until<=clock_timestamp())
   AND (next_check_at<=clock_timestamp() OR (dirty AND (verified_at IS NULL OR verified_at<=clock_timestamp()-interval '5 seconds')))
 ORDER BY next_check_at,shop_id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE shop_whatsapp_connections c SET operation_token=$1,lease_until=clock_timestamp()+interval '90 seconds',dirty=FALSE
FROM due WHERE c.shop_id=due.shop_id RETURNING c.*,clock_timestamp()::timestamptz AS observed_at;

-- name: CompleteWhatsAppObservation :execrows
UPDATE shop_whatsapp_connections SET state=sqlc.arg(state)::text,error_code=sqlc.arg(error_code)::text,
    verified_at=CASE WHEN sqlc.arg(error_code)::text='' THEN clock_timestamp() ELSE verified_at END,
    last_connected_at=CASE WHEN sqlc.arg(state)::text='connected' AND sqlc.arg(error_code)::text='' AND (state<>'connected' OR last_connected_at IS NULL) THEN clock_timestamp() ELSE last_connected_at END,
    next_check_at=clock_timestamp()+interval '60 seconds',operation_token=NULL,lease_until=NULL
WHERE shop_id=sqlc.arg(shop_id) AND generation=sqlc.arg(generation) AND operation_token=sqlc.arg(operation_token) AND lease_until>clock_timestamp();

-- name: MarkWhatsAppConnectionDirty :exec
UPDATE shop_whatsapp_connections SET dirty=TRUE,last_event_at=clock_timestamp() WHERE instance_name=$1;
