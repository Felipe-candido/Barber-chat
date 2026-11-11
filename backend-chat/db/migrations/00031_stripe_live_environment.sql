-- +goose Up
ALTER TABLE public.billing_accounts DROP CONSTRAINT billing_accounts_stripe_mode_check;
ALTER TABLE public.billing_accounts ADD CONSTRAINT billing_accounts_stripe_mode_check CHECK(stripe_mode IN('unknown','test','live'));
ALTER TABLE public.billing_webhook_events DROP CONSTRAINT billing_webhook_receipt;
ALTER TABLE public.billing_webhook_events ADD CONSTRAINT billing_webhook_receipt CHECK(receipt_protocol=0 OR
 (resource_kind IS NOT NULL AND resource_kind IN('checkout','subscription','invoice','ignored')
 AND resource_id IS NOT NULL AND event_created_at IS NOT NULL AND isfinite(event_created_at) AND api_version IS NOT NULL));

-- A database belongs to one Stripe environment. Existing sandbox coverage must
-- never become real paid coverage just because a deployment changes its key.
CREATE TABLE public.billing_environment(
 id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(id),
 mode VARCHAR(8) NOT NULL CHECK(mode IN('test','live'))
);
INSERT INTO public.billing_environment(id,mode)
SELECT TRUE,'test' WHERE EXISTS(SELECT 1 FROM public.billing_accounts) OR EXISTS(SELECT 1 FROM public.billing_webhook_events);

-- +goose StatementBegin
CREATE FUNCTION public.guard_billing_environment() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE desired TEXT; actual TEXT;
BEGIN
 IF TG_TABLE_NAME='billing_environment' THEN
  RAISE EXCEPTION 'Billing environment is immutable; use an isolated database for another environment';
 END IF;
 IF TG_TABLE_NAME='billing_accounts' THEN
  desired:=NEW.stripe_mode;
  IF TG_OP='UPDATE' AND OLD.stripe_mode<>'unknown' AND OLD.stripe_mode<>desired THEN
   RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='billing_environment_matches',MESSAGE='billing account mode cannot change';
  END IF;
  IF desired='unknown' THEN RETURN NEW; END IF;
 ELSE desired:=CASE WHEN NEW.livemode THEN 'live' ELSE 'test' END;
 END IF;
 INSERT INTO public.billing_environment(id,mode) VALUES(TRUE,desired) ON CONFLICT DO NOTHING;
 SELECT mode INTO actual FROM public.billing_environment WHERE id FOR SHARE;
 IF actual IS DISTINCT FROM desired THEN
  RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='billing_environment_matches',MESSAGE='Stripe environment does not match this database';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER billing_environment_immutable BEFORE UPDATE OR DELETE ON public.billing_environment
 FOR EACH ROW EXECUTE FUNCTION public.guard_billing_environment();
CREATE TRIGGER billing_account_environment BEFORE INSERT OR UPDATE ON public.billing_accounts
 FOR EACH ROW EXECUTE FUNCTION public.guard_billing_environment();
CREATE TRIGGER billing_receipt_environment BEFORE INSERT OR UPDATE ON public.billing_webhook_events
 FOR EACH ROW EXECUTE FUNCTION public.guard_billing_environment();
ALTER TABLE public.billing_environment ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.billing_environment FROM PUBLIC;
REVOKE ALL ON FUNCTION public.guard_billing_environment() FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
 FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
  EXECUTE format('REVOKE ALL ON public.billing_environment FROM %I',browser_role);
  EXECUTE format('REVOKE ALL ON FUNCTION public.guard_billing_environment() FROM %I',browser_role);
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.billing_environment WHERE mode='live') OR EXISTS(SELECT 1 FROM public.billing_accounts WHERE stripe_mode='live') OR EXISTS(SELECT 1 FROM public.billing_webhook_events WHERE livemode) THEN
  RAISE EXCEPTION 'Live billing cannot be downgraded to a test-only application';
 END IF;
END $$;
DROP TRIGGER billing_receipt_environment ON public.billing_webhook_events;
DROP TRIGGER billing_account_environment ON public.billing_accounts;
DROP TRIGGER billing_environment_immutable ON public.billing_environment;
DROP FUNCTION public.guard_billing_environment();
DROP TABLE public.billing_environment;
ALTER TABLE public.billing_accounts DROP CONSTRAINT billing_accounts_stripe_mode_check;
ALTER TABLE public.billing_accounts ADD CONSTRAINT billing_accounts_stripe_mode_check CHECK(stripe_mode IN('unknown','test'));
ALTER TABLE public.billing_webhook_events DROP CONSTRAINT billing_webhook_receipt;
ALTER TABLE public.billing_webhook_events ADD CONSTRAINT billing_webhook_receipt CHECK(receipt_protocol=0 OR
 (NOT livemode AND resource_kind IS NOT NULL AND resource_kind IN('checkout','subscription','invoice','ignored')
 AND resource_id IS NOT NULL AND event_created_at IS NOT NULL AND isfinite(event_created_at) AND api_version IS NOT NULL));
-- +goose StatementEnd
