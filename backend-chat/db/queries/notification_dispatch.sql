-- name: MaterializeNotificationDispatches :many
WITH due AS (
 SELECT j.shop_id,j.id FROM notification_jobs j
 WHERE j.state='pending' AND j.dispatch_mode='automatic' AND j.dispatch_seq<9223372036854775807
  AND j.due_at<=clock_timestamp() AND j.expires_at>clock_timestamp() AND j.dispatch_next_at<=clock_timestamp()
  AND (j.delivery_state='ready' OR (j.delivery_state IN('leased','sending') AND j.delivery_lease_until<=clock_timestamp())
   OR (j.delivery_state='failed' AND j.delivery_reason='provider_unavailable' AND j.delivery_attempt_count<5
    AND EXISTS(SELECT 1 FROM notification_attempts a WHERE a.shop_id=j.shop_id AND a.job_id=j.id AND a.attempt_id=j.delivery_token
      AND a.outcome='not_sent' AND a.reason='provider_unavailable'
      AND a.finished_at+make_interval(secs=>LEAST(30*power(2,j.delivery_attempt_count-1),900)::double precision)<=clock_timestamp())))
  AND NOT EXISTS(SELECT 1 FROM notification_outbox o WHERE o.shop_id=j.shop_id AND o.job_id=j.id AND o.dispatch_seq=j.dispatch_seq AND o.consumed_at IS NULL)
 ORDER BY j.dispatch_next_at,j.due_at,j.shop_id,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 25
), advanced AS (
 UPDATE notification_jobs j SET dispatch_seq=j.dispatch_seq+1,
  delivery_state=CASE WHEN j.delivery_state='failed' THEN 'ready' ELSE j.delivery_state END,
  delivery_token=CASE WHEN j.delivery_state='failed' THEN NULL ELSE j.delivery_token END,
  delivery_reason=CASE WHEN j.delivery_state='failed' THEN '' ELSE j.delivery_reason END
 FROM due d WHERE j.shop_id=d.shop_id AND j.id=d.id RETURNING j.shop_id,j.id,j.dispatch_seq
)
INSERT INTO notification_outbox(id,shop_id,job_id,dispatch_seq)
SELECT gen_random_uuid(),a.shop_id,a.id,a.dispatch_seq FROM advanced a
RETURNING id,shop_id,job_id,dispatch_seq;

-- name: ClaimNotificationOutbox :one
WITH due AS (
 SELECT o.id FROM notification_outbox o JOIN notification_jobs j ON j.shop_id=o.shop_id AND j.id=o.job_id
 WHERE o.consumed_at IS NULL AND o.next_publish_at<=clock_timestamp()
  AND (o.lease_until IS NULL OR o.lease_until<=clock_timestamp())
  AND (o.published_at IS NULL OR o.published_at<=clock_timestamp()-interval '5 minutes')
  AND j.dispatch_seq=o.dispatch_seq AND j.state='pending' AND j.expires_at>clock_timestamp()
 ORDER BY o.next_publish_at,o.created_at FOR UPDATE OF o SKIP LOCKED LIMIT 1
)
UPDATE notification_outbox o SET operation_token=$1,lease_until=clock_timestamp()+interval '30 seconds',publish_count=CASE WHEN o.publish_count<2147483647 THEN o.publish_count+1 ELSE o.publish_count END
FROM due WHERE o.id=due.id RETURNING o.*;

-- name: CompleteNotificationPublish :execrows
UPDATE notification_outbox SET published_at=clock_timestamp(),operation_token=NULL,lease_until=NULL
WHERE id=$1 AND operation_token=$2 AND lease_until>clock_timestamp();

-- name: DeferNotificationPublish :execrows
UPDATE notification_outbox SET operation_token=NULL,lease_until=NULL,next_publish_at=clock_timestamp()+interval '15 seconds'
WHERE id=$1 AND operation_token=$2 AND lease_until>clock_timestamp();

-- name: ValidateNotificationEnvelope :one
SELECT EXISTS(SELECT 1 FROM notification_outbox WHERE id=$1 AND shop_id=$2 AND job_id=$3 AND dispatch_seq=$4)::boolean;

-- name: FinishNotificationDispatch :exec
WITH job AS (
 UPDATE notification_jobs AS j SET dispatch_next_at=clock_timestamp()+interval '30 seconds'
 WHERE j.shop_id=$1 AND j.id=$2 AND j.dispatch_seq=$3 RETURNING j.shop_id,j.id
)
UPDATE notification_outbox o SET consumed_at=COALESCE(o.consumed_at,clock_timestamp())
FROM job j WHERE o.shop_id=j.shop_id AND o.job_id=j.id AND o.dispatch_seq=$3;

-- name: RetireNotificationDispatch :exec
UPDATE notification_outbox SET consumed_at=COALESCE(consumed_at,clock_timestamp()) WHERE id=$1 AND shop_id=$2 AND job_id=$3 AND dispatch_seq=$4;

-- name: ClaimNotificationSender :execrows
UPDATE shop_whatsapp_connections SET send_token=$2,send_lease_until=clock_timestamp()+interval '90 seconds'
WHERE shop_id=$1 AND (send_lease_until IS NULL OR send_lease_until<=clock_timestamp())
 AND state='connected' AND desired_connected AND error_code='' AND verified_at>clock_timestamp()-interval '2 minutes';

-- name: ReleaseNotificationSender :exec
UPDATE shop_whatsapp_connections SET send_token=NULL,send_lease_until=NULL WHERE shop_id=$1 AND send_token=$2;

-- name: TouchNotificationWorker :exec
INSERT INTO notification_worker_heartbeats(worker_id,checked_at,healthy) VALUES($1,clock_timestamp(),$2)
ON CONFLICT(worker_id) DO UPDATE SET checked_at=EXCLUDED.checked_at,healthy=EXCLUDED.healthy;

-- name: NotificationWorkerAvailable :one
SELECT EXISTS(SELECT 1 FROM notification_worker_heartbeats WHERE healthy AND checked_at>clock_timestamp()-interval '2 minutes')::boolean;

-- name: ListNotificationRecoveryKeys :many
SELECT shop_id,id FROM notification_jobs
WHERE (delivery_state='sending' AND delivery_lease_until<=clock_timestamp())
 OR (delivery_state IN('ready','leased') AND expires_at<=clock_timestamp())
ORDER BY expires_at,shop_id,id LIMIT 25;

-- name: NotificationOperationalMetrics :one
SELECT (SELECT count(*) FROM notification_outbox WHERE consumed_at IS NULL AND published_at IS NULL)::bigint AS outbox_pending,
 (SELECT count(*) FROM notification_jobs WHERE delivery_state='unknown')::bigint AS uncertain,
 (SELECT count(*) FROM notification_jobs WHERE delivery_state='sending')::bigint AS sending,
 (SELECT GREATEST(0,COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(due_at)),0))::bigint FROM notification_jobs
  WHERE state='pending' AND dispatch_mode='automatic' AND delivery_state='ready' AND due_at<=clock_timestamp() AND expires_at>clock_timestamp())::bigint AS oldest_due_seconds;
