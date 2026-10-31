-- name: InsertProduct :exec
INSERT INTO public.products(id,shop_id,name,description,price_cents,currency,active,version,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: LockProduct :one
SELECT * FROM public.products WHERE shop_id=$1 AND id=$2 FOR UPDATE;

-- name: UpdateProduct :execrows
UPDATE public.products SET name=sqlc.arg(name),description=sqlc.arg(description),price_cents=sqlc.arg(price_cents),
    active=sqlc.arg(active),version=version+1,updated_at=sqlc.arg(updated_at)
WHERE shop_id=sqlc.arg(shop_id) AND id=sqlc.arg(product_id) AND version=sqlc.arg(expected_version);

-- name: InsertProductStock :exec
INSERT INTO public.product_stock(shop_id,product_id,minimum_quantity) VALUES($1,$2,$3);

-- name: LockProductStock :one
SELECT * FROM public.product_stock WHERE shop_id=$1 AND product_id=$2 FOR NO KEY UPDATE;

-- name: AppendProductStockMovement :exec
INSERT INTO public.product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: UpdateProductStockMinimum :execrows
UPDATE public.product_stock SET minimum_quantity=sqlc.arg(minimum_quantity),version=version+1
WHERE shop_id=sqlc.arg(shop_id) AND product_id=sqlc.arg(product_id) AND version=sqlc.arg(expected_version);

-- name: FindProductOutput :one
SELECT p.*,s.quantity_on_hand,s.minimum_quantity,s.version AS stock_version
FROM public.products p JOIN public.product_stock s ON s.shop_id=p.shop_id AND s.product_id=p.id
WHERE p.shop_id=$1 AND p.id=$2;

-- name: ListProducts :many
SELECT p.*,s.quantity_on_hand,s.minimum_quantity,s.version AS stock_version
FROM public.products p JOIN public.product_stock s ON s.shop_id=p.shop_id AND s.product_id=p.id
WHERE p.shop_id=sqlc.arg(shop_id) AND (p.active OR sqlc.arg(include_inactive)::boolean)
    AND (sqlc.arg(search)::text='' OR strpos(lower(p.name),lower(sqlc.arg(search)::text))>0)
    AND (NOT sqlc.arg(has_after)::boolean OR (p.created_at,p.id)<(sqlc.arg(after_time)::timestamptz,sqlc.arg(after_id)::uuid))
ORDER BY p.created_at DESC,p.id DESC LIMIT sqlc.arg(page_limit);

-- name: ListProductStockMovements :many
SELECT * FROM public.product_stock_movements
WHERE shop_id=sqlc.arg(shop_id) AND product_id=sqlc.arg(product_id)
    AND (NOT sqlc.arg(has_after)::boolean OR (created_at,id)<(sqlc.arg(after_time)::timestamptz,sqlc.arg(after_id)::uuid))
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit);

-- name: ClaimProductCommand :execrows
INSERT INTO public.product_commands(shop_id,actor_id,operation,key_digest,fingerprint)
VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;

-- name: LockProductCommand :one
SELECT fingerprint,completed,product_id,movement_id FROM public.product_commands
WHERE shop_id=$1 AND actor_id=$2 AND operation=$3 AND key_digest=$4 FOR UPDATE;

-- name: CompleteProductCommand :execrows
UPDATE public.product_commands SET completed=TRUE,product_id=sqlc.arg(product_id),movement_id=sqlc.narg(movement_id)
WHERE shop_id=sqlc.arg(shop_id) AND actor_id=sqlc.arg(actor_id) AND operation=sqlc.arg(operation)
    AND key_digest=sqlc.arg(key_digest) AND fingerprint=sqlc.arg(fingerprint) AND NOT completed;
