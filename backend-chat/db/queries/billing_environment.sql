-- name: BindBillingEnvironment :exec
INSERT INTO public.billing_environment(id,mode) VALUES(TRUE,$1) ON CONFLICT DO NOTHING;

-- name: GetBillingEnvironment :one
SELECT mode FROM public.billing_environment WHERE id;
