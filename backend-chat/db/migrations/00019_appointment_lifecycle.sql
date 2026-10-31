-- +goose Up
CREATE TABLE public.appointment_completion_policy (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(id),
    activated_at TIMESTAMPTZ NOT NULL CHECK(isfinite(activated_at))
);
INSERT INTO public.appointment_completion_policy(id,activated_at) VALUES(TRUE,clock_timestamp());
ALTER TABLE public.appointments
    ADD CONSTRAINT appointment_lifecycle_identity UNIQUE(shop_id,id),
    ADD COLUMN status_source VARCHAR(16) NOT NULL DEFAULT 'legacy' CHECK(status_source IN('legacy','booking','public','staff','automatic')),
    ADD COLUMN status_reason VARCHAR(500) NOT NULL DEFAULT '' CHECK(status_reason !~ '[[:cntrl:]]'),
    ADD COLUMN status_actor_id UUID,
    ADD CONSTRAINT appointment_status_actor FOREIGN KEY(status_actor_id,shop_id) REFERENCES public.shop_memberships(user_id,shop_id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointment_status_metadata CHECK(
        (status_source='staff' AND status_actor_id IS NOT NULL AND length(btrim(status_reason))>0)
        OR (status_source<>'staff' AND status_actor_id IS NULL));
ALTER TABLE public.appointments ALTER COLUMN status_source SET DEFAULT 'booking';
CREATE INDEX appointments_completion_due ON public.appointments(ends_at,id) WHERE status IN('scheduled','confirmed');

CREATE TABLE public.appointment_status_events (
    shop_id UUID NOT NULL,
    appointment_id UUID NOT NULL,
    version BIGINT NOT NULL CHECK(version>1),
    previous_status VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    source VARCHAR(16) NOT NULL CHECK(source IN('public','staff','automatic')),
    actor_id UUID,
    reason VARCHAR(500) NOT NULL CHECK(length(btrim(reason))>0 AND reason !~ '[[:cntrl:]]'),
    occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
    PRIMARY KEY(shop_id,appointment_id,version),
    FOREIGN KEY(shop_id,appointment_id) REFERENCES public.appointments(shop_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(actor_id,shop_id) REFERENCES public.shop_memberships(user_id,shop_id) ON DELETE RESTRICT,
    CHECK((source='staff')=(actor_id IS NOT NULL))
);
CREATE TABLE public.appointment_status_commands (
    shop_id UUID NOT NULL REFERENCES public.shops(id),
    actor_id UUID NOT NULL REFERENCES public.users(id),
    key_digest BYTEA NOT NULL CHECK(octet_length(key_digest)=32),
    fingerprint BYTEA NOT NULL CHECK(octet_length(fingerprint)=32),
    appointment_id UUID,
    result_version BIGINT,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY(shop_id,actor_id,key_digest),
    FOREIGN KEY(shop_id,appointment_id,result_version) REFERENCES public.appointment_status_events(shop_id,appointment_id,version),
    CHECK((NOT completed AND appointment_id IS NULL AND result_version IS NULL)
        OR (completed AND appointment_id IS NOT NULL AND result_version IS NOT NULL))
);

-- +goose StatementBegin
CREATE FUNCTION public.guard_new_appointment_lifecycle() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.status<>'scheduled' OR NEW.version<>1 OR NEW.status_source<>'booking'
        OR NEW.status_reason<>'' OR NEW.status_actor_id IS NOT NULL
        OR NOT isfinite(NEW.starts_at) OR NOT isfinite(NEW.ends_at)
        OR NOT isfinite(NEW.created_at) OR NOT isfinite(NEW.updated_at) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_initial_state',MESSAGE='new appointments begin scheduled without a forged lifecycle';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER appointment_initial_state BEFORE INSERT ON public.appointments
FOR EACH ROW EXECUTE FUNCTION public.guard_new_appointment_lifecycle();

CREATE FUNCTION public.guard_appointment_transition() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_history_immutable',MESSAGE='appointment history cannot be deleted';
    END IF;
    IF (to_jsonb(OLD)-ARRAY['status','version','updated_at','status_source','status_reason','status_actor_id'])
        IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['status','version','updated_at','status_source','status_reason','status_actor_id'])
        OR NEW.version<>OLD.version+1 OR NEW.status=OLD.status OR NOT isfinite(NEW.updated_at)
        OR NEW.updated_at<OLD.updated_at OR length(btrim(NEW.status_reason))=0 THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_transition',MESSAGE='transition requires immutable snapshots, version and audit';
    END IF;
    IF NEW.status_source='automatic' THEN
        IF OLD.status NOT IN('scheduled','confirmed') OR NEW.status<>'completed' OR NEW.status_actor_id IS NOT NULL
            OR NEW.updated_at<OLD.ends_at OR OLD.ends_at>clock_timestamp()
            OR NOT EXISTS(SELECT 1 FROM public.appointment_completion_policy WHERE id AND OLD.ends_at>=activated_at) THEN
            RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_automatic_completion',MESSAGE='only eligible ended appointments may complete automatically';
        END IF;
    ELSIF NEW.status_source='public' THEN
        IF OLD.status NOT IN('scheduled','confirmed') OR NEW.status<>'cancelled' OR NEW.status_actor_id IS NOT NULL OR NEW.updated_at>=OLD.starts_at THEN
            RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_public_cancellation',MESSAGE='public cancellation must precede start';
        END IF;
    ELSIF NEW.status_source='staff' THEN
        IF NEW.status_actor_id IS NULL OR NEW.status NOT IN('completed','no_show','cancelled')
            OR (NEW.status='completed' AND NEW.updated_at<OLD.ends_at)
            OR (NEW.status='no_show' AND NEW.updated_at<OLD.starts_at) THEN
            RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_staff_transition',MESSAGE='staff transition is invalid for this time or state';
        END IF;
    ELSE
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_transition_source',MESSAGE='status changes require a trusted source';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER appointment_transition_guard BEFORE UPDATE OR DELETE ON public.appointments
FOR EACH ROW EXECUTE FUNCTION public.guard_appointment_transition();

CREATE FUNCTION public.record_appointment_transition() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    INSERT INTO public.appointment_status_events(shop_id,appointment_id,version,previous_status,status,source,actor_id,reason,occurred_at)
    VALUES(NEW.shop_id,NEW.id,NEW.version,OLD.status,NEW.status,NEW.status_source,NEW.status_actor_id,NEW.status_reason,NEW.updated_at);
    RETURN NULL;
END $$;
CREATE TRIGGER appointment_transition_audit AFTER UPDATE ON public.appointments
FOR EACH ROW EXECUTE FUNCTION public.record_appointment_transition();

CREATE FUNCTION public.guard_appointment_event() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF TG_OP='INSERT' THEN
      IF pg_trigger_depth()>=2 AND EXISTS(
        SELECT 1 FROM public.appointments a WHERE a.shop_id=NEW.shop_id AND a.id=NEW.appointment_id
            AND a.version=NEW.version AND a.status=NEW.status AND a.status_source=NEW.source
            AND a.status_actor_id IS NOT DISTINCT FROM NEW.actor_id AND a.status_reason=NEW.reason AND a.updated_at=NEW.occurred_at) THEN
        RETURN NEW;
      END IF;
    END IF;
    RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_audit_immutable',MESSAGE='appointment audit is append only';
END $$;
CREATE TRIGGER appointment_event_immutable BEFORE INSERT OR UPDATE OR DELETE ON public.appointment_status_events
FOR EACH ROW EXECUTE FUNCTION public.guard_appointment_event();
CREATE TRIGGER appointment_completion_policy_immutable BEFORE UPDATE OR DELETE ON public.appointment_completion_policy
FOR EACH ROW EXECUTE FUNCTION public.guard_appointment_event();

CREATE FUNCTION public.require_appointment_status_command() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM public.appointment_status_commands WHERE shop_id=NEW.shop_id AND actor_id=NEW.actor_id
        AND key_digest=NEW.key_digest AND NOT completed) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_command_complete',MESSAGE='incomplete status command cannot commit';
    END IF;
    IF NEW.completed AND NOT EXISTS(SELECT 1 FROM public.appointment_status_events WHERE shop_id=NEW.shop_id
        AND appointment_id=NEW.appointment_id AND version=NEW.result_version AND source='staff' AND actor_id=NEW.actor_id) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_command_actor',MESSAGE='command result must match its staff actor';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER appointment_status_command_complete AFTER INSERT OR UPDATE ON public.appointment_status_commands
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_appointment_status_command();

ALTER TABLE public.appointment_completion_policy ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.appointment_status_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.appointment_status_commands ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.appointment_completion_policy,public.appointment_status_events,public.appointment_status_commands FROM PUBLIC;
REVOKE ALL ON FUNCTION public.guard_new_appointment_lifecycle(),public.guard_appointment_transition(),public.record_appointment_transition(),public.guard_appointment_event(),public.require_appointment_status_command() FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.appointment_completion_policy,public.appointment_status_events,public.appointment_status_commands FROM %I',browser_role);
        EXECUTE format('REVOKE ALL ON FUNCTION public.guard_new_appointment_lifecycle(),public.guard_appointment_transition(),public.record_appointment_transition(),public.guard_appointment_event(),public.require_appointment_status_command() FROM %I',browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.appointment_status_events) OR EXISTS(SELECT 1 FROM public.appointment_status_commands) THEN
        RAISE EXCEPTION 'Lifecycle rollback requires no lifecycle audit/commands';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER appointment_status_command_complete ON public.appointment_status_commands;
DROP TRIGGER appointment_transition_audit ON public.appointments;
DROP TRIGGER appointment_transition_guard ON public.appointments;
DROP TRIGGER appointment_event_immutable ON public.appointment_status_events;
DROP TRIGGER appointment_completion_policy_immutable ON public.appointment_completion_policy;
DROP TRIGGER appointment_initial_state ON public.appointments;
DROP FUNCTION public.guard_new_appointment_lifecycle();
DROP FUNCTION public.require_appointment_status_command();
DROP FUNCTION public.guard_appointment_event();
DROP FUNCTION public.record_appointment_transition();
DROP FUNCTION public.guard_appointment_transition();
DROP TABLE public.appointment_status_commands;
DROP TABLE public.appointment_status_events;
ALTER TABLE public.appointments DROP CONSTRAINT appointment_status_metadata,DROP CONSTRAINT appointment_status_actor,
    DROP COLUMN status_source,DROP COLUMN status_reason,DROP COLUMN status_actor_id,DROP CONSTRAINT appointment_lifecycle_identity;
DROP TABLE public.appointment_completion_policy;
DROP INDEX public.appointments_completion_due;
