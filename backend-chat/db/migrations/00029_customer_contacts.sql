-- +goose Up
ALTER TABLE public.customers DROP CONSTRAINT customers_last_name_check;
ALTER TABLE public.customers ADD CONSTRAINT customers_last_name_check CHECK(last_name='' OR length(btrim(last_name))>0);
CREATE INDEX customers_shop_created_page ON public.customers(shop_id,created_at DESC,id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.customers WHERE last_name='') THEN
  RAISE EXCEPTION 'Contacts without a surname must be resolved explicitly before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP INDEX public.customers_shop_created_page;
ALTER TABLE public.customers DROP CONSTRAINT customers_last_name_check;
ALTER TABLE public.customers ADD CONSTRAINT customers_last_name_check CHECK(length(btrim(last_name))>0);
