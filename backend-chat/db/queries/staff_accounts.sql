-- name: LockAccountOwner :one
SELECT m.user_id FROM shop_memberships m
JOIN users u ON u.id = m.user_id
JOIN shops s ON s.id = m.shop_id
WHERE m.user_id = $1 AND m.shop_id = $2 AND m.role = 'owner'
  AND m.active AND u.active AND s.active
FOR SHARE OF m,u,s;

-- name: LockAccountProfessional :one
SELECT name, active FROM professionals WHERE shop_id = $1 AND id = $2 FOR UPDATE;

-- name: ReserveStaffAccount :one
INSERT INTO staff_accounts(id,shop_id,professional_id,email,state)
SELECT $1,$2,$3,$4,'pending'
WHERE NOT EXISTS (SELECT 1 FROM shop_memberships WHERE shop_id=$2 AND professional_id=$3)
ON CONFLICT (shop_id,professional_id) WHERE replaced_at IS NULL DO NOTHING
RETURNING id;

-- name: FindStaffAccount :one
SELECT id,professional_id,email,COALESCE(auth_user_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS user_id,state
FROM staff_accounts WHERE shop_id=$1 AND id=$2 AND replaced_at IS NULL;

-- name: InsertStaffProfile :exec
INSERT INTO users(id,display_name) VALUES ($1,$2) ON CONFLICT(id) DO NOTHING;

-- name: LockStaffProfile :one
SELECT active FROM users WHERE id=$1 FOR SHARE;

-- name: InsertBarberMembership :exec
INSERT INTO shop_memberships(user_id,shop_id,role,professional_id)
VALUES ($1,$2,'barber',$3) ON CONFLICT(user_id,shop_id) DO NOTHING;

-- name: LinkStaffAccount :execrows
UPDATE staff_accounts SET auth_user_id=$3,state='linked' WHERE shop_id=$1 AND id=$2 AND replaced_at IS NULL;

-- name: ReviewStaffAccount :exec
UPDATE staff_accounts SET state='review' WHERE shop_id=$1 AND id=$2 AND state='pending' AND replaced_at IS NULL;

-- name: SetBarberAccountActive :execrows
UPDATE shop_memberships SET active=$4
WHERE shop_id=$1 AND user_id=$2 AND professional_id=$3 AND role='barber';

-- name: ListStaffAccounts :many
SELECT a.id,a.professional_id,a.email,
    COALESCE(a.auth_user_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS user_id,
    CASE WHEN a.state='linked' AND a.auth_user_id IS NULL THEN 'review' ELSE a.state END::text AS state,
    COALESCE(m.active AND u.active AND p.active AND m.role='barber',FALSE)::boolean AS active
FROM staff_accounts a
JOIN professionals p ON p.shop_id=a.shop_id AND p.id=a.professional_id
LEFT JOIN shop_memberships m ON m.shop_id=a.shop_id AND m.user_id=a.auth_user_id AND m.professional_id=a.professional_id
LEFT JOIN users u ON u.id=a.auth_user_id
WHERE a.shop_id=$1 AND a.replaced_at IS NULL ORDER BY a.created_at,a.id;

-- name: LockAccountMembership :one
SELECT role,active,COALESCE(professional_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS professional_id
FROM shop_memberships WHERE shop_id=$1 AND user_id=$2 FOR UPDATE;

-- name: RemoveInactiveBarberMembership :execrows
DELETE FROM shop_memberships WHERE shop_id=$1 AND user_id=$2
    AND professional_id=$3 AND role='barber' AND NOT active;

-- name: RetireStaffAccount :execrows
UPDATE staff_accounts SET replaced_at=now() WHERE shop_id=$1 AND id=$2 AND replaced_at IS NULL;

-- name: LockStaffBookingAccess :one
SELECT m.user_id FROM shop_memberships m JOIN users u ON u.id=m.user_id
LEFT JOIN professionals p ON p.shop_id=m.shop_id AND p.id=m.professional_id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.active AND u.active
  AND (m.role='owner' OR (m.role='barber' AND m.professional_id=$3 AND p.active))
FOR SHARE OF m,u;
