-- name: ClaimSaleCommand :execrows
INSERT INTO public.sale_commands(shop_id,actor_id,operation,key_digest,fingerprint)
VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;

-- name: LockSaleCommand :one
SELECT completed,sale_id,fingerprint FROM public.sale_commands
WHERE shop_id=$1 AND actor_id=$2 AND operation=$3 AND key_digest=$4 FOR UPDATE;

-- name: CompleteSaleCommand :execrows
UPDATE public.sale_commands SET completed=TRUE,sale_id=sqlc.arg(sale_id)
WHERE shop_id=sqlc.arg(shop_id) AND actor_id=sqlc.arg(actor_id) AND operation=sqlc.arg(operation)
    AND key_digest=sqlc.arg(key_digest) AND fingerprint=sqlc.arg(fingerprint) AND NOT completed;

-- name: InsertRecordedSale :exec
INSERT INTO public.sales(id,shop_id,recorded_by_user_id,professional_id,appointment_id,
    professional_name_snapshot,total_cents,sold_at,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: InsertRecordedSaleItems :execrows
INSERT INTO public.sale_items(id,shop_id,sale_id,product_id,product_name_snapshot,quantity,unit_price_cents)
SELECT x.id,x.shop_id,x.sale_id,x.product_id,x.product_name,x.quantity,x.unit_price_cents
FROM jsonb_to_recordset(sqlc.arg(items)::jsonb) AS x(id uuid,shop_id uuid,sale_id uuid,product_id uuid,
    product_name text,quantity integer,unit_price_cents bigint) ORDER BY x.product_id;

-- name: GetRecordedSale :one
SELECT * FROM public.sales WHERE shop_id=$1 AND id=$2;

-- name: GetReadableSale :one
SELECT * FROM public.sales WHERE shop_id=sqlc.arg(shop_id) AND id=sqlc.arg(sale_id)
    AND (sqlc.narg(professional_id)::uuid IS NULL OR professional_id=sqlc.narg(professional_id));

-- name: GetRecordedSaleItems :many
SELECT * FROM public.sale_items WHERE shop_id=$1 AND sale_id=$2 ORDER BY product_id;

-- name: LockSaleForCancellation :one
SELECT * FROM public.sales WHERE shop_id=$1 AND id=$2 FOR NO KEY UPDATE;

-- name: SaveSaleCancellation :execrows
UPDATE public.sales SET status='cancelled',version=version+1,cancelled_by_user_id=sqlc.arg(actor_id),
    cancellation_reason=sqlc.arg(reason),restock=sqlc.arg(restock),cancelled_at=sqlc.arg(cancelled_at)
WHERE shop_id=sqlc.arg(shop_id) AND id=sqlc.arg(sale_id) AND status='recorded' AND version=sqlc.arg(expected_version);

-- name: ListSalesPage :many
SELECT * FROM public.sales WHERE shop_id=sqlc.arg(shop_id)
    AND (sqlc.narg(professional_id)::uuid IS NULL OR professional_id=sqlc.narg(professional_id))
    AND sold_at>=sqlc.arg(from_at)::timestamptz AND sold_at<sqlc.arg(until_at)::timestamptz
    AND (sqlc.arg(status)::text='' OR status=sqlc.arg(status))
    AND (sqlc.narg(after_at)::timestamptz IS NULL OR (sold_at,id)<(sqlc.narg(after_at),sqlc.narg(after_id)::uuid))
ORDER BY sold_at DESC,id DESC LIMIT sqlc.arg(page_limit);

-- name: ListSalesPageItems :many
SELECT * FROM public.sale_items WHERE shop_id=sqlc.arg(shop_id) AND sale_id=ANY(sqlc.arg(sale_ids)::uuid[])
ORDER BY sale_id,product_id;
