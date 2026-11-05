-- name: LockCustomerContactsAccess :one
SELECT m.user_id FROM shop_memberships m JOIN users u ON u.id=m.user_id JOIN shops s ON s.id=m.shop_id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.active AND u.active AND s.active
 AND (m.role='owner' OR (m.role='barber' AND m.professional_id IS NOT NULL)) FOR SHARE OF s,u,m;

-- name: CreateCustomerContact :one
INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at)
VALUES($1,$2,$3,$4,$5,clock_timestamp()) RETURNING *;

-- name: GetCustomerContact :one
SELECT * FROM customers WHERE shop_id=$1 AND id=$2;

-- name: ListCustomerContacts :many
SELECT * FROM customers WHERE shop_id=sqlc.arg(shop_id)
 AND (sqlc.arg(name_pattern)::text='' OR btrim(first_name||' '||last_name) ILIKE sqlc.arg(name_pattern) ESCAPE E'\\')
 AND (sqlc.arg(phone)::text='' OR phone_e164=sqlc.arg(phone))
 AND (NOT sqlc.arg(has_cursor)::boolean OR (created_at,id)<(sqlc.arg(after_at)::timestamptz,sqlc.arg(after_id)::uuid))
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit)::integer;
