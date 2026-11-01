-- +goose Up
-- Notifications are opt-in. Existing shops/bookings receive no retroactive jobs.
CREATE TABLE public.shop_notification_settings (
    shop_id UUID PRIMARY KEY REFERENCES public.shops(id) ON DELETE RESTRICT,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    reminder_lead_minutes INTEGER NOT NULL DEFAULT 1440 CHECK(reminder_lead_minutes BETWEEN 5 AND 10080),
    version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK(isfinite(updated_at))
);

ALTER TABLE public.appointments ADD CONSTRAINT appointment_notification_customer UNIQUE(shop_id,id,customer_id);
-- Owned by Customers: permission for this booking only, never marketing consent.
CREATE TABLE public.customer_booking_message_permissions (
    shop_id UUID NOT NULL,
    appointment_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    allowed BOOLEAN NOT NULL,
    purpose VARCHAR(32) NOT NULL DEFAULT 'booking_whatsapp' CHECK(purpose='booking_whatsapp'),
    notice_version INTEGER NOT NULL CHECK(notice_version=1),
    source VARCHAR(20) NOT NULL CHECK(source IN('public_booking','staff_booking')),
    actor_id UUID,
    recorded_at TIMESTAMPTZ NOT NULL CHECK(isfinite(recorded_at)),
    revoked_at TIMESTAMPTZ CHECK(isfinite(revoked_at) AND revoked_at>=recorded_at),
    PRIMARY KEY(shop_id,appointment_id),
    FOREIGN KEY(shop_id,appointment_id,customer_id) REFERENCES public.appointments(shop_id,id,customer_id) ON DELETE RESTRICT,
    FOREIGN KEY(actor_id,shop_id) REFERENCES public.shop_memberships(user_id,shop_id) ON DELETE RESTRICT,
    CHECK((source='staff_booking')=(actor_id IS NOT NULL))
);

-- One immutable preparation per appointment generation. Ciphertext includes
-- contact and code; neither is copied to jobs, logs or a broker envelope.
CREATE TABLE public.appointment_notification_plans (
    shop_id UUID NOT NULL,
    appointment_id UUID NOT NULL,
    schedule_generation BIGINT NOT NULL CHECK(schedule_generation>0),
    policy_version BIGINT NOT NULL CHECK(policy_version>0),
    skip_reason VARCHAR(32) CHECK(skip_reason IN('policy_disabled','permission_missing')),
    reminder_skip_reason VARCHAR(32) CHECK(reminder_skip_reason='insufficient_lead'),
    payload_version INTEGER,
    payload_key_id VARCHAR(64),
    encrypted_payload BYTEA,
    payload_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL CHECK(isfinite(created_at)),
    PRIMARY KEY(shop_id,appointment_id,schedule_generation),
    FOREIGN KEY(shop_id,appointment_id) REFERENCES public.customer_booking_message_permissions(shop_id,appointment_id) ON DELETE RESTRICT,
    CHECK(
      (skip_reason IS NULL AND payload_version IS NOT NULL AND payload_version=1
        AND payload_key_id IS NOT NULL AND payload_key_id ~ '^[A-Za-z0-9_-]{1,64}$'
        AND encrypted_payload IS NOT NULL AND octet_length(encrypted_payload) BETWEEN 29 AND 16412
        AND payload_expires_at IS NOT NULL AND isfinite(payload_expires_at) AND payload_expires_at>created_at)
      OR (skip_reason IS NOT NULL AND payload_version IS NULL AND payload_key_id IS NULL
        AND encrypted_payload IS NULL AND payload_expires_at IS NULL AND reminder_skip_reason IS NULL)
    )
);

CREATE TABLE public.notification_jobs (
    id UUID NOT NULL,
    shop_id UUID NOT NULL,
    appointment_id UUID NOT NULL,
    schedule_generation BIGINT NOT NULL,
    kind VARCHAR(20) NOT NULL CHECK(kind IN('booking_confirmation','booking_reminder')),
    due_at TIMESTAMPTZ NOT NULL CHECK(isfinite(due_at)),
    expires_at TIMESTAMPTZ NOT NULL CHECK(isfinite(expires_at) AND expires_at>due_at),
    state VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(state IN('pending','invalidated')),
    invalidated_at TIMESTAMPTZ,
    PRIMARY KEY(shop_id,id),
    UNIQUE(shop_id,appointment_id,kind,schedule_generation),
    FOREIGN KEY(shop_id,appointment_id,schedule_generation) REFERENCES public.appointment_notification_plans(shop_id,appointment_id,schedule_generation) ON DELETE RESTRICT,
    CHECK((state='pending' AND invalidated_at IS NULL)
       OR (state='invalidated' AND invalidated_at IS NOT NULL AND isfinite(invalidated_at)))
);
CREATE INDEX notification_jobs_due ON public.notification_jobs(due_at,shop_id,id) WHERE state='pending';
CREATE INDEX notification_plans_expiry ON public.appointment_notification_plans(payload_expires_at) WHERE payload_expires_at IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION public.guard_new_notification_job() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    -- Serialize with cancellation before re-reading eligibility. The planner
    -- already holds this lock; this also protects a direct SQL job insertion.
    PERFORM 1 FROM public.professionals WHERE shop_id=NEW.shop_id AND id=(
      SELECT professional_id FROM public.appointments WHERE shop_id=NEW.shop_id AND id=NEW.appointment_id
    ) FOR UPDATE;
    IF NEW.state<>'pending' OR NOT EXISTS(
      SELECT 1 FROM public.appointment_notification_plans p
      JOIN public.customer_booking_message_permissions c USING(shop_id,appointment_id)
      JOIN public.appointments a ON a.shop_id=p.shop_id AND a.id=p.appointment_id
      WHERE p.shop_id=NEW.shop_id AND p.appointment_id=NEW.appointment_id
        AND p.schedule_generation=NEW.schedule_generation AND p.skip_reason IS NULL
        AND c.allowed AND c.revoked_at IS NULL AND a.status IN('scheduled','confirmed')
        AND NEW.due_at>=p.created_at AND NEW.expires_at<=p.payload_expires_at
        AND p.payload_expires_at<=a.starts_at
        AND (NEW.kind<>'booking_confirmation' OR NEW.due_at=p.created_at)
        AND (NEW.kind<>'booking_reminder' OR (NEW.due_at>p.created_at AND NEW.expires_at<a.starts_at))
    ) THEN
      RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='notification_job_eligible',MESSAGE='notification job requires an eligible protected plan';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER notification_job_eligible BEFORE INSERT ON public.notification_jobs
FOR EACH ROW EXECUTE FUNCTION public.guard_new_notification_job();

-- Covers public/staff cancellation, automatic completion and direct SQL updates.
-- This trigger only owns notification data; it never changes appointment state.
CREATE FUNCTION public.invalidate_appointment_notification_jobs() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.status NOT IN('scheduled','confirmed') OR
       ROW(NEW.starts_at,NEW.ends_at,NEW.professional_id,NEW.customer_id)
         IS DISTINCT FROM ROW(OLD.starts_at,OLD.ends_at,OLD.professional_id,OLD.customer_id) THEN
      UPDATE public.notification_jobs SET state='invalidated',invalidated_at=clock_timestamp()
      WHERE shop_id=NEW.shop_id AND appointment_id=NEW.id AND state='pending';
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER appointment_notifications_invalidate AFTER UPDATE ON public.appointments
FOR EACH ROW EXECUTE FUNCTION public.invalidate_appointment_notification_jobs();

CREATE FUNCTION public.invalidate_revoked_notification_jobs() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NOT NEW.allowed OR NEW.revoked_at IS NOT NULL THEN
      UPDATE public.notification_jobs SET state='invalidated',invalidated_at=clock_timestamp()
      WHERE shop_id=NEW.shop_id AND appointment_id=NEW.appointment_id AND state='pending';
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER notification_permission_invalidate AFTER UPDATE ON public.customer_booking_message_permissions
FOR EACH ROW EXECUTE FUNCTION public.invalidate_revoked_notification_jobs();

CREATE FUNCTION public.invalidate_disabled_shop_notification_jobs() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NOT NEW.enabled THEN
      UPDATE public.notification_jobs SET state='invalidated',invalidated_at=clock_timestamp()
      WHERE shop_id=NEW.shop_id AND state='pending';
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER shop_notifications_disable AFTER UPDATE ON public.shop_notification_settings
FOR EACH ROW EXECUTE FUNCTION public.invalidate_disabled_shop_notification_jobs();

DO $$ DECLARE table_name TEXT; browser_role TEXT; BEGIN
    FOREACH table_name IN ARRAY ARRAY['shop_notification_settings','customer_booking_message_permissions','appointment_notification_plans','notification_jobs'] LOOP
      EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY',table_name);
      EXECUTE format('REVOKE ALL ON public.%I FROM PUBLIC',table_name);
      FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.%I FROM %I',table_name,browser_role);
      END LOOP;
    END LOOP;
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION public.invalidate_appointment_notification_jobs(),public.invalidate_revoked_notification_jobs(),public.invalidate_disabled_shop_notification_jobs() FROM PUBLIC;

-- +goose Down
-- Do not accidentally erase live delivery history or accepted permissions.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.appointment_notification_plans)
       OR EXISTS(SELECT 1 FROM public.customer_booking_message_permissions)
       OR EXISTS(SELECT 1 FROM public.shop_notification_settings) THEN
      RAISE EXCEPTION 'Notification rollback requires empty notification tables';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER shop_notifications_disable ON public.shop_notification_settings;
DROP TRIGGER notification_permission_invalidate ON public.customer_booking_message_permissions;
DROP TRIGGER appointment_notifications_invalidate ON public.appointments;
DROP FUNCTION public.invalidate_disabled_shop_notification_jobs();
DROP FUNCTION public.invalidate_revoked_notification_jobs();
DROP FUNCTION public.invalidate_appointment_notification_jobs();
DROP TABLE public.notification_jobs;
DROP FUNCTION public.guard_new_notification_job();
DROP TABLE public.appointment_notification_plans;
DROP TABLE public.customer_booking_message_permissions;
ALTER TABLE public.appointments DROP CONSTRAINT appointment_notification_customer;
DROP TABLE public.shop_notification_settings;
