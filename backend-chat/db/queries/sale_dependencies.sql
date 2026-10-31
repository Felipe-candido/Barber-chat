-- name: ReadSalesAccess :one
SELECT m.role,COALESCE(m.professional_id,'00000000-0000-0000-0000-000000000000'::uuid)::uuid AS professional_id
FROM public.shop_memberships m JOIN public.users u ON u.id=m.user_id JOIN public.shops s ON s.id=m.shop_id
WHERE m.user_id=$1 AND m.shop_id=$2 AND m.active AND u.active AND s.active
    AND (m.role='owner' OR EXISTS(SELECT 1 FROM public.professionals p
        WHERE p.shop_id=m.shop_id AND p.id=m.professional_id AND p.active));

-- name: LockSalesProfessional :one
SELECT id,shop_id,name,active FROM public.professionals WHERE shop_id=$1 AND id=$2 AND active FOR SHARE;

-- name: LockSaleCatalogProducts :many
SELECT * FROM public.products WHERE shop_id=sqlc.arg(shop_id) AND id=ANY(sqlc.arg(product_ids)::uuid[]) ORDER BY id FOR SHARE;

-- name: LockSaleProductStocks :many
SELECT * FROM public.product_stock WHERE shop_id=sqlc.arg(shop_id) AND product_id=ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id FOR NO KEY UPDATE;

-- name: AppendSaleStockMovements :execrows
INSERT INTO public.product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id,created_at)
SELECT x.id,x.shop_id,x.product_id,x.actor_id,'sale',x.quantity_delta,'Product sale',x.sale_item_id,x.created_at
FROM jsonb_to_recordset(sqlc.arg(movements)::jsonb) AS x(id uuid,shop_id uuid,product_id uuid,actor_id uuid,
    quantity_delta integer,sale_item_id uuid,created_at timestamptz) ORDER BY x.product_id;

-- name: ReadSaleAppointment :one
SELECT id,shop_id,professional_id,status FROM public.appointments WHERE shop_id=$1 AND id=$2;

-- name: LockSaleAppointment :one
SELECT id,shop_id,professional_id,status FROM public.appointments WHERE shop_id=$1 AND id=$2 FOR NO KEY UPDATE;

-- name: LockSalesBillingShop :one
SELECT id,active,provisioning_source FROM public.shops WHERE id=$1 FOR SHARE;

-- name: ReadSalesTimezone :one
SELECT timezone FROM public.shops WHERE id=$1 AND active;

-- name: ReadOriginalSaleStock :many
SELECT * FROM public.product_stock_movements WHERE shop_id=sqlc.arg(shop_id)
    AND sale_item_id=ANY(sqlc.arg(item_ids)::uuid[]) AND kind='sale' ORDER BY product_id;

-- name: AppendSaleStockReversals :execrows
INSERT INTO public.product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id,reverses_movement_id,created_at)
SELECT x.id,x.shop_id,x.product_id,x.actor_id,'sale_reversal',x.quantity_delta,x.reason,x.sale_item_id,x.reverses_movement_id,x.created_at
FROM jsonb_to_recordset(sqlc.arg(movements)::jsonb) AS x(id uuid,shop_id uuid,product_id uuid,actor_id uuid,
    quantity_delta integer,reason text,sale_item_id uuid,reverses_movement_id uuid,created_at timestamptz) ORDER BY x.product_id;
