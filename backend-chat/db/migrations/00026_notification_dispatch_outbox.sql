-- +goose Up
ALTER TABLE public.notification_jobs
 ADD COLUMN dispatch_seq BIGINT NOT NULL DEFAULT 0 CHECK(dispatch_seq>=0),
 ADD COLUMN dispatch_next_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK(isfinite(dispatch_next_at));
ALTER TABLE public.shop_whatsapp_connections
 ADD COLUMN send_token UUID,
 ADD COLUMN send_lease_until TIMESTAMPTZ,
 ADD CONSTRAINT whatsapp_sender_lease CHECK((send_token IS NULL AND send_lease_until IS NULL)
  OR (send_token IS NOT NULL AND send_lease_until IS NOT NULL AND isfinite(send_lease_until)));

CREATE TABLE public.notification_outbox(
 id UUID PRIMARY KEY,
 shop_id UUID NOT NULL,
 job_id UUID NOT NULL,
 dispatch_seq BIGINT NOT NULL CHECK(dispatch_seq>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK(isfinite(created_at)),
 next_publish_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK(isfinite(next_publish_at)),
 operation_token UUID,
 lease_until TIMESTAMPTZ,
 published_at TIMESTAMPTZ CHECK(isfinite(published_at)),
 consumed_at TIMESTAMPTZ CHECK(isfinite(consumed_at)),
 publish_count INTEGER NOT NULL DEFAULT 0 CHECK(publish_count>=0),
 UNIQUE(shop_id,job_id,dispatch_seq),
 FOREIGN KEY(shop_id,job_id) REFERENCES public.notification_jobs(shop_id,id) ON DELETE RESTRICT,
 CHECK((operation_token IS NULL AND lease_until IS NULL)
  OR (operation_token IS NOT NULL AND lease_until IS NOT NULL AND isfinite(lease_until)))
);
CREATE INDEX notification_outbox_work ON public.notification_outbox(next_publish_at,created_at) WHERE consumed_at IS NULL;
CREATE INDEX notification_dispatch_due ON public.notification_jobs(dispatch_next_at,due_at) WHERE state='pending' AND dispatch_mode='automatic';
CREATE TABLE public.notification_worker_heartbeats(
 worker_id UUID PRIMARY KEY,
 checked_at TIMESTAMPTZ NOT NULL CHECK(isfinite(checked_at)),
 healthy BOOLEAN NOT NULL
);
-- +goose StatementBegin
DO $$ DECLARE table_name TEXT; browser_role TEXT; BEGIN
 FOREACH table_name IN ARRAY ARRAY['notification_outbox','notification_worker_heartbeats'] LOOP
  EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY',table_name);
  EXECUTE format('REVOKE ALL ON public.%I FROM PUBLIC',table_name);
  FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
   EXECUTE format('REVOKE ALL ON public.%I FROM %I',table_name,browser_role);
  END LOOP;
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.notification_outbox) OR EXISTS(SELECT 1 FROM public.shop_whatsapp_connections WHERE send_token IS NOT NULL) THEN
  RAISE EXCEPTION 'Dispatch rollback requires no outbox or sender leases';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.notification_worker_heartbeats;
DROP TABLE public.notification_outbox;
DROP INDEX public.notification_dispatch_due;
ALTER TABLE public.notification_jobs DROP COLUMN dispatch_seq,DROP COLUMN dispatch_next_at;
ALTER TABLE public.shop_whatsapp_connections DROP CONSTRAINT whatsapp_sender_lease,DROP COLUMN send_token,DROP COLUMN send_lease_until;
