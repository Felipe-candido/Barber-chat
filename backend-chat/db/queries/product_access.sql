-- name: LockProductAccess :one
-- Commands order: professional (for barber) -> membership/user -> shop -> product -> stock.
-- Shared authority locks prevent revocation/deactivation during a write.
SELECT m.role,COALESCE(m.professional_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS professional_id
FROM public.shop_memberships m JOIN public.users u ON u.id=m.user_id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.active AND u.active
FOR SHARE OF m,u;

-- name: LockProductStaffProfessional :one
SELECT p.id FROM public.professionals p
JOIN public.shop_memberships m ON m.shop_id=p.shop_id AND m.professional_id=p.id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.role='barber' AND p.active
FOR SHARE OF p;

-- name: LockActiveProductShop :one
SELECT id FROM public.shops WHERE id=$1 AND active FOR SHARE;
