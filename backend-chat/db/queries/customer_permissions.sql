-- name: InsertBookingMessagePermission :exec
INSERT INTO customer_booking_message_permissions(shop_id,appointment_id,customer_id,allowed,notice_version,source,actor_id,recorded_at)
VALUES($1,$2,$3,$4,1,$5,$6,$7);
