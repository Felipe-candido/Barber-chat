-- name: ReadAppointmentReference :one
SELECT id,shop_id,professional_id FROM public.appointments WHERE shop_id=$1 AND id=$2;

-- name: LockLifecycleAppointment :one
SELECT * FROM public.appointments WHERE shop_id=$1 AND id=$2 FOR NO KEY UPDATE;

-- name: ReadLifecycleClock :one
SELECT clock_timestamp()::timestamptz AS now;

-- name: SetStaffAppointmentStatus :execrows
UPDATE public.appointments SET status=sqlc.arg(status),version=version+1,status_source='staff',status_reason=sqlc.arg(reason),
    status_actor_id=sqlc.arg(actor_id),updated_at=sqlc.arg(changed_at)
WHERE shop_id=sqlc.arg(shop_id) AND id=sqlc.arg(appointment_id) AND version=sqlc.arg(expected_version);

-- name: GetAdminAppointmentDetail :one
SELECT a.*,s.name AS shop_name,(c.first_name||' '||c.last_name)::text AS customer_name,c.phone_e164
FROM public.appointments a JOIN public.shops s ON s.id=a.shop_id
JOIN public.customers c ON c.shop_id=a.shop_id AND c.id=a.customer_id
WHERE a.shop_id=$1 AND a.id=$2;

-- name: ListAppointmentStatusEvents :many
SELECT * FROM public.appointment_status_events WHERE shop_id=$1 AND appointment_id=$2 ORDER BY version DESC LIMIT 101;

-- name: ClaimAppointmentStatusCommand :execrows
INSERT INTO public.appointment_status_commands(shop_id,actor_id,key_digest,fingerprint)
VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: LockAppointmentStatusCommand :one
SELECT fingerprint,appointment_id,result_version,completed FROM public.appointment_status_commands
WHERE shop_id=$1 AND actor_id=$2 AND key_digest=$3 FOR UPDATE;

-- name: CompleteAppointmentStatusCommand :execrows
UPDATE public.appointment_status_commands SET appointment_id=sqlc.arg(appointment_id),result_version=sqlc.arg(result_version),completed=TRUE
WHERE shop_id=sqlc.arg(shop_id) AND actor_id=sqlc.arg(actor_id) AND key_digest=sqlc.arg(key_digest)
    AND fingerprint=sqlc.arg(fingerprint) AND NOT completed;

-- name: CompleteDueAppointments :many
WITH candidates AS MATERIALIZED (
    SELECT a.shop_id,a.id,a.professional_id FROM public.appointments a
    JOIN public.appointment_completion_policy policy ON policy.id
    WHERE a.status IN('scheduled','confirmed') AND a.ends_at>=policy.activated_at AND a.ends_at<=statement_timestamp()
    ORDER BY a.ends_at,a.id LIMIT sqlc.arg(batch_size)::integer
), professionals_locked AS MATERIALIZED (
    SELECT p.shop_id,p.id FROM public.professionals p
    WHERE EXISTS(SELECT 1 FROM candidates c WHERE c.shop_id=p.shop_id AND c.professional_id=p.id)
    ORDER BY p.id FOR UPDATE OF p SKIP LOCKED
), appointments_locked AS MATERIALIZED (
    SELECT a.shop_id,a.id FROM public.appointments a JOIN candidates c ON c.shop_id=a.shop_id AND c.id=a.id
    JOIN professionals_locked p ON p.shop_id=a.shop_id AND p.id=a.professional_id
    WHERE a.status IN('scheduled','confirmed') AND a.ends_at<=statement_timestamp()
    ORDER BY a.id FOR NO KEY UPDATE OF a SKIP LOCKED
)
UPDATE public.appointments a SET status='completed',version=a.version+1,status_source='automatic',status_actor_id=NULL,
    status_reason='Reserved interval elapsed',updated_at=clock_timestamp()
FROM appointments_locked l WHERE a.shop_id=l.shop_id AND a.id=l.id AND a.status IN('scheduled','confirmed')
RETURNING a.id;
