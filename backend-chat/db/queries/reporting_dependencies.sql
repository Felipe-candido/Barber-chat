-- name: ReadActivityReportAccess :one
SELECT m.role,COALESCE(m.professional_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS professional_id
FROM public.shop_memberships m
JOIN public.users u ON u.id=m.user_id AND u.active
JOIN public.shops s ON s.id=m.shop_id AND s.active
WHERE m.user_id=sqlc.arg(actor_id) AND m.shop_id=sqlc.arg(shop_id) AND m.active
  AND (m.role='owner' OR (m.role='barber' AND EXISTS(
    SELECT 1 FROM public.professionals p WHERE p.shop_id=m.shop_id AND p.id=m.professional_id AND p.active)));

-- name: FindActiveReportShop :one
SELECT id,timezone FROM public.shops WHERE id=$1 AND active;
