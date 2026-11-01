-- +goose Up
-- Job state remains the business intent (pending/invalidated). Delivery state
-- records the external effect separately, including when cancellation races it.
ALTER TABLE public.notification_jobs
 ADD COLUMN delivery_state VARCHAR(16) NOT NULL DEFAULT 'ready'
   CHECK(delivery_state IN('ready','leased','sending','accepted','failed','unknown','expired','invalidated')),
 ADD COLUMN delivery_token UUID,
 ADD COLUMN delivery_lease_until TIMESTAMPTZ CHECK(isfinite(delivery_lease_until)),
 ADD COLUMN delivery_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(delivery_attempt_count BETWEEN 0 AND 5),
 ADD COLUMN delivery_reason VARCHAR(32) NOT NULL DEFAULT ''
   CHECK(delivery_reason IN('','intent_invalidated','ineligible','expired','payload_unavailable','provider_rejected','provider_unavailable','transport_unknown','invalid_response')),
 ADD CONSTRAINT notification_delivery_claim CHECK(
   (delivery_state IN('ready','expired','invalidated') AND delivery_token IS NULL AND delivery_lease_until IS NULL)
   OR (delivery_state IN('leased','sending') AND delivery_token IS NOT NULL AND delivery_lease_until IS NOT NULL)
   OR (delivery_state IN('accepted','failed','unknown') AND delivery_token IS NOT NULL AND delivery_lease_until IS NULL)
 ),
 ADD CONSTRAINT notification_delivery_started CHECK(delivery_state NOT IN('sending','accepted','failed','unknown') OR delivery_attempt_count>0);

-- Migration never makes an already invalidated intent executable again.
UPDATE public.notification_jobs SET delivery_state='invalidated',delivery_reason='intent_invalidated' WHERE state='invalidated';
ALTER TABLE public.shop_whatsapp_connections ADD CONSTRAINT whatsapp_connection_delivery_instance UNIQUE(shop_id,instance_name);

CREATE TABLE public.notification_attempts (
 attempt_id UUID PRIMARY KEY,
 shop_id UUID NOT NULL,
 job_id UUID NOT NULL,
 attempt_number INTEGER NOT NULL CHECK(attempt_number BETWEEN 1 AND 5),
 instance_name VARCHAR(35) NOT NULL CHECK(instance_name ~ '^bc_[0-9a-f]{32}$'),
 outcome VARCHAR(16) NOT NULL DEFAULT 'sending' CHECK(outcome IN('sending','accepted','not_sent','unknown')),
 provider_message_id VARCHAR(200) CHECK(provider_message_id ~ '^[A-Za-z0-9:_-]{1,200}$'),
 reason VARCHAR(32) NOT NULL DEFAULT '' CHECK(reason IN('','provider_rejected','provider_unavailable','transport_unknown','invalid_response')),
 started_at TIMESTAMPTZ NOT NULL CHECK(isfinite(started_at)),
 finished_at TIMESTAMPTZ CHECK(isfinite(finished_at) AND finished_at>=started_at),
 UNIQUE(shop_id,job_id,attempt_number),
 FOREIGN KEY(shop_id,job_id) REFERENCES public.notification_jobs(shop_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(shop_id,instance_name) REFERENCES public.shop_whatsapp_connections(shop_id,instance_name) ON DELETE RESTRICT,
 CHECK((outcome='sending' AND finished_at IS NULL AND provider_message_id IS NULL AND reason='')
    OR (outcome='accepted' AND finished_at IS NOT NULL AND provider_message_id IS NOT NULL AND reason='')
    OR (outcome IN('not_sent','unknown') AND finished_at IS NOT NULL AND provider_message_id IS NULL AND reason<>''))
);
CREATE UNIQUE INDEX notification_provider_message ON public.notification_attempts(shop_id,instance_name,provider_message_id) WHERE provider_message_id IS NOT NULL;
CREATE INDEX notification_delivery_recovery ON public.notification_jobs(delivery_lease_until,shop_id,id) WHERE delivery_state IN('leased','sending');

-- +goose StatementBegin
CREATE FUNCTION public.invalidate_unstarted_notification_delivery() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF NEW.state='invalidated' AND NEW.delivery_state IN('ready','leased') THEN
   NEW.delivery_state='invalidated'; NEW.delivery_reason='intent_invalidated';
   NEW.delivery_token=NULL; NEW.delivery_lease_until=NULL;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER notification_delivery_invalidate BEFORE UPDATE OF state ON public.notification_jobs
FOR EACH ROW EXECUTE FUNCTION public.invalidate_unstarted_notification_delivery();
ALTER TABLE public.notification_attempts ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.notification_attempts FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
 FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
  EXECUTE format('REVOKE ALL ON public.notification_attempts FROM %I',browser_role);
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.notification_attempts)
    OR EXISTS(SELECT 1 FROM public.notification_jobs WHERE delivery_state NOT IN('ready','invalidated')) THEN
  RAISE EXCEPTION 'Delivery rollback requires no attempts or active delivery state';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.notification_attempts;
ALTER TABLE public.shop_whatsapp_connections DROP CONSTRAINT whatsapp_connection_delivery_instance;
DROP TRIGGER notification_delivery_invalidate ON public.notification_jobs;
DROP FUNCTION public.invalidate_unstarted_notification_delivery();
DROP INDEX public.notification_delivery_recovery;
ALTER TABLE public.notification_jobs
 DROP CONSTRAINT notification_delivery_started,DROP CONSTRAINT notification_delivery_claim,
 DROP COLUMN delivery_state,DROP COLUMN delivery_token,DROP COLUMN delivery_lease_until,
 DROP COLUMN delivery_attempt_count,DROP COLUMN delivery_reason;
