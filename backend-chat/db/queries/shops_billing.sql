-- name: LockBillingCheckoutShop :one
SELECT id, slug, provisioning_source FROM public.shops WHERE id = $1 AND active = TRUE FOR SHARE;
