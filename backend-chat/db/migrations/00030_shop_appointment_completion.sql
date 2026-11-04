-- +goose Up
CREATE TABLE public.shop_appointment_completion_settings(
 shop_id UUID PRIMARY KEY REFERENCES public.shops(id) ON DELETE CASCADE,
 mode VARCHAR(12) NOT NULL DEFAULT 'automatic' CHECK(mode IN('automatic','manual')),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 automatic_since TIMESTAMPTZ,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 CHECK((mode='manual' AND automatic_since IS NULL) OR (mode='automatic' AND automatic_since IS NOT NULL AND isfinite(automatic_since)))
);
INSERT INTO public.shop_appointment_completion_settings(shop_id,mode,automatic_since)
SELECT s.id,'automatic',p.activated_at FROM public.shops s CROSS JOIN public.appointment_completion_policy p WHERE p.id;

-- +goose StatementBegin
CREATE FUNCTION public.initialize_shop_appointment_completion() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
 INSERT INTO public.shop_appointment_completion_settings(shop_id,mode,automatic_since)
 SELECT NEW.id,'automatic',activated_at FROM public.appointment_completion_policy WHERE id;
 RETURN NEW;
END $$;
CREATE FUNCTION public.guard_shop_automatic_completion() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE current_mode TEXT; enabled_since TIMESTAMPTZ;
BEGIN
 IF NEW.status_source='automatic' THEN
  SELECT mode,automatic_since INTO current_mode,enabled_since FROM public.shop_appointment_completion_settings WHERE shop_id=OLD.shop_id FOR SHARE;
  IF NOT FOUND OR current_mode<>'automatic' OR enabled_since IS NULL OR OLD.ends_at<enabled_since THEN
   RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='appointment_completion_mode',MESSAGE='automatic completion is disabled or outside the shop policy window';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER shop_appointment_completion_initialize AFTER INSERT ON public.shops
FOR EACH ROW EXECUTE FUNCTION public.initialize_shop_appointment_completion();
CREATE TRIGGER appointment_shop_completion_guard BEFORE UPDATE ON public.appointments
FOR EACH ROW EXECUTE FUNCTION public.guard_shop_automatic_completion();
ALTER TABLE public.shop_appointment_completion_settings ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.shop_appointment_completion_settings FROM PUBLIC;
REVOKE ALL ON FUNCTION public.initialize_shop_appointment_completion(),public.guard_shop_automatic_completion() FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
 FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
  EXECUTE format('REVOKE ALL ON public.shop_appointment_completion_settings FROM %I',browser_role);
  EXECUTE format('REVOKE ALL ON FUNCTION public.initialize_shop_appointment_completion(),public.guard_shop_automatic_completion() FROM %I',browser_role);
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.shop_appointment_completion_settings WHERE mode='manual' OR version>1) THEN
  RAISE EXCEPTION 'Completion policy changes require explicit review before removing manual-mode protection';
 END IF;
END $$;
DROP TRIGGER appointment_shop_completion_guard ON public.appointments;
DROP TRIGGER shop_appointment_completion_initialize ON public.shops;
DROP FUNCTION public.guard_shop_automatic_completion();
DROP FUNCTION public.initialize_shop_appointment_completion();
DROP TABLE public.shop_appointment_completion_settings;
-- +goose StatementEnd
