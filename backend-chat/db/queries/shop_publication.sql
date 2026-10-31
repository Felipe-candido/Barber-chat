-- name: LockPublicationShop :one
SELECT id FROM public.shops WHERE id=$1 AND active=TRUE FOR UPDATE;

-- name: GetPublicationShop :one
SELECT id, slug, provisioning_source, publication_requested
FROM public.shops WHERE id=$1 AND active=TRUE;

-- name: SetPublicationRequested :execrows
UPDATE public.shops SET publication_requested=$2, updated_at=now()
WHERE id=$1 AND active=TRUE AND provisioning_source='self_service';
