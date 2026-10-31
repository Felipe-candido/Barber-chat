-- name: HasPublicationService :one
SELECT EXISTS(SELECT 1 FROM public.services WHERE shop_id=$1 AND active=TRUE
    AND duration_minutes BETWEEN 1 AND 1440 AND price_cents <= 9007199254740991)::boolean;

-- name: ListPublicationCandidates :many
SELECT p.id, MIN(s.duration_minutes)::integer AS duration_minutes
FROM public.professionals p
JOIN public.professional_services ps ON ps.shop_id=p.shop_id AND ps.professional_id=p.id
JOIN public.services s ON s.shop_id=ps.shop_id AND s.id=ps.service_id
WHERE p.shop_id=$1 AND p.active=TRUE AND s.active=TRUE
    AND s.duration_minutes BETWEEN 1 AND 1440 AND s.price_cents <= 9007199254740991
GROUP BY p.id;
