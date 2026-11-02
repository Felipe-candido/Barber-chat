-- name: GetNotificationMessageShop :one
SELECT s.id,s.name,s.slug,s.timezone,a.street,a.number,a.complement,a.district,a.city,a.state,a.postal_code
FROM shops s JOIN shop_addresses a ON a.shop_id=s.id WHERE s.id=$1 AND s.active=TRUE;
-- name: GetPublicNotificationOptions :one
SELECT COALESCE(n.enabled,FALSE)::boolean AS enabled FROM shops s LEFT JOIN shop_notification_settings n ON n.shop_id=s.id
WHERE s.slug=$1 AND s.active;
