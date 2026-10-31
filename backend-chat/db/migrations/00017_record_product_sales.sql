-- +goose Up
ALTER TABLE public.appointments ADD CONSTRAINT appointment_sale_professional UNIQUE(shop_id,id,professional_id);

CREATE TABLE public.sales (
    id UUID PRIMARY KEY CHECK(id<>'00000000-0000-0000-0000-000000000000'),
    shop_id UUID NOT NULL REFERENCES public.shops(id) ON DELETE RESTRICT,
    recorded_by_user_id UUID NOT NULL,
    professional_id UUID,
    appointment_id UUID,
    professional_name_snapshot VARCHAR(100) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'recorded' CHECK(status='recorded'),
    currency VARCHAR(3) NOT NULL DEFAULT 'BRL' CHECK(currency='BRL'),
    total_cents BIGINT NOT NULL CHECK(total_cents BETWEEN 1 AND 9007199254740991),
    version BIGINT NOT NULL DEFAULT 1 CHECK(version=1),
    sold_at TIMESTAMPTZ NOT NULL CHECK(isfinite(sold_at)),
    created_at TIMESTAMPTZ NOT NULL CHECK(isfinite(created_at)),
    UNIQUE(shop_id,id),
    UNIQUE(shop_id,id,recorded_by_user_id),
    FOREIGN KEY(recorded_by_user_id,shop_id) REFERENCES public.shop_memberships(user_id,shop_id) ON DELETE RESTRICT,
    FOREIGN KEY(shop_id,professional_id) REFERENCES public.professionals(shop_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(shop_id,appointment_id,professional_id) REFERENCES public.appointments(shop_id,id,professional_id) ON DELETE RESTRICT,
    CHECK(appointment_id IS NULL OR professional_id IS NOT NULL),
    CHECK((professional_id IS NULL AND professional_name_snapshot='') OR
        (professional_id IS NOT NULL AND length(btrim(professional_name_snapshot))>0 AND professional_name_snapshot !~ '[[:cntrl:]]'))
);
CREATE INDEX sales_shop_page ON public.sales(shop_id,sold_at DESC,id DESC);
CREATE INDEX sales_professional_page ON public.sales(shop_id,professional_id,sold_at DESC,id DESC);

CREATE TABLE public.sale_items (
    id UUID PRIMARY KEY CHECK(id<>'00000000-0000-0000-0000-000000000000'),
    shop_id UUID NOT NULL,
    sale_id UUID NOT NULL,
    product_id UUID NOT NULL,
    product_name_snapshot VARCHAR(100) NOT NULL CHECK(length(btrim(product_name_snapshot))>0 AND product_name_snapshot !~ '[[:cntrl:]]'),
    quantity INTEGER NOT NULL CHECK(quantity>0),
    unit_price_cents BIGINT NOT NULL CHECK(unit_price_cents BETWEEN 1 AND 9007199254740991),
    currency VARCHAR(3) NOT NULL DEFAULT 'BRL' CHECK(currency='BRL'),
    line_total_cents BIGINT GENERATED ALWAYS AS (unit_price_cents*quantity::bigint) STORED
        CHECK(line_total_cents BETWEEN 1 AND 9007199254740991),
    FOREIGN KEY(shop_id,sale_id) REFERENCES public.sales(shop_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(shop_id,product_id) REFERENCES public.products(shop_id,id) ON DELETE RESTRICT,
    UNIQUE(shop_id,sale_id,product_id),
    UNIQUE(shop_id,product_id,id,quantity)
);

CREATE TABLE public.sale_commands (
    shop_id UUID NOT NULL REFERENCES public.shops(id) ON DELETE RESTRICT,
    actor_id UUID NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    operation VARCHAR(16) NOT NULL CHECK(operation='record_sale'),
    key_digest BYTEA NOT NULL CHECK(octet_length(key_digest)=32),
    fingerprint BYTEA NOT NULL CHECK(octet_length(fingerprint)=32),
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    sale_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(shop_id,actor_id,operation,key_digest),
    FOREIGN KEY(shop_id,sale_id,actor_id) REFERENCES public.sales(shop_id,id,recorded_by_user_id) ON DELETE RESTRICT,
    CHECK((NOT completed AND sale_id IS NULL) OR (completed AND sale_id IS NOT NULL))
);

ALTER TABLE public.product_stock_movements
    DROP CONSTRAINT product_stock_movements_kind_check,
    DROP CONSTRAINT product_stock_movements_check,
    ADD COLUMN sale_item_id UUID,
    ADD COLUMN sale_quantity INTEGER GENERATED ALWAYS AS (CASE WHEN kind='sale' THEN -quantity_delta END) STORED,
    ADD CONSTRAINT stock_movement_kind CHECK(kind IN('opening','receipt','adjustment','sale')),
    ADD CONSTRAINT stock_movement_sign CHECK(kind='adjustment' OR (kind IN('opening','receipt') AND quantity_delta>0) OR (kind='sale' AND quantity_delta<0)),
    ADD CONSTRAINT stock_sale_origin CHECK((kind='sale')=(sale_item_id IS NOT NULL)),
    ADD CONSTRAINT stock_sale_item_fk FOREIGN KEY(shop_id,product_id,sale_item_id,sale_quantity)
        REFERENCES public.sale_items(shop_id,product_id,id,quantity) ON DELETE RESTRICT;
CREATE UNIQUE INDEX stock_sale_item_once ON public.product_stock_movements(shop_id,sale_item_id) WHERE kind='sale';

-- +goose StatementBegin
CREATE FUNCTION public.guard_recorded_sale_history() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='sale_history_immutable',MESSAGE='recorded sale history is immutable';
END $$;
CREATE TRIGGER sales_history_guard BEFORE UPDATE OR DELETE ON public.sales
FOR EACH ROW EXECUTE FUNCTION public.guard_recorded_sale_history();
CREATE TRIGGER sale_items_history_guard BEFORE UPDATE OR DELETE ON public.sale_items
FOR EACH ROW EXECUTE FUNCTION public.guard_recorded_sale_history();

CREATE FUNCTION public.check_sale_stock_origin() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.kind='sale' AND NOT EXISTS(
        SELECT 1 FROM public.sale_items i JOIN public.sales s ON s.shop_id=i.shop_id AND s.id=i.sale_id
        WHERE i.shop_id=NEW.shop_id AND i.id=NEW.sale_item_id AND i.product_id=NEW.product_id
            AND s.recorded_by_user_id=NEW.actor_id AND s.status='recorded') THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='stock_sale_actor',MESSAGE='stock movement does not match its recorded sale';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER stock_sale_origin_guard BEFORE INSERT ON public.product_stock_movements
FOR EACH ROW EXECUTE FUNCTION public.check_sale_stock_origin();

CREATE FUNCTION public.require_complete_recorded_sale() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE target UUID; item_count BIGINT; item_total NUMERIC; expected BIGINT;
BEGIN
    IF TG_TABLE_NAME='sales' THEN target:=NEW.id;
    ELSIF TG_TABLE_NAME='sale_items' THEN target:=NEW.sale_id;
    ELSE
        IF NEW.kind<>'sale' THEN RETURN NULL; END IF;
        SELECT sale_id INTO target FROM public.sale_items WHERE shop_id=NEW.shop_id AND id=NEW.sale_item_id;
    END IF;
    SELECT total_cents INTO expected FROM public.sales WHERE shop_id=NEW.shop_id AND id=target;
    SELECT count(*),COALESCE(sum(line_total_cents),0) INTO item_count,item_total
        FROM public.sale_items WHERE shop_id=NEW.shop_id AND sale_id=target;
    IF expected IS NULL OR item_count NOT BETWEEN 1 AND 50 OR item_total<>expected OR EXISTS(
        SELECT 1 FROM public.sale_items i WHERE i.shop_id=NEW.shop_id AND i.sale_id=target
        AND NOT EXISTS(SELECT 1 FROM public.product_stock_movements m
            WHERE m.shop_id=i.shop_id AND m.product_id=i.product_id AND m.sale_item_id=i.id AND m.kind='sale')) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='recorded_sale_complete',MESSAGE='sale totals, items and stock must commit together';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER sales_complete AFTER INSERT ON public.sales
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_complete_recorded_sale();
CREATE CONSTRAINT TRIGGER sale_items_complete AFTER INSERT ON public.sale_items
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_complete_recorded_sale();
CREATE CONSTRAINT TRIGGER sale_stock_complete AFTER INSERT ON public.product_stock_movements
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_complete_recorded_sale();

CREATE FUNCTION public.require_completed_sale_command() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM public.sale_commands WHERE shop_id=NEW.shop_id AND actor_id=NEW.actor_id
        AND operation=NEW.operation AND key_digest=NEW.key_digest AND NOT completed) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='sale_command_complete',MESSAGE='incomplete sale command cannot commit';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER sale_command_complete AFTER INSERT OR UPDATE ON public.sale_commands
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_completed_sale_command();

ALTER TABLE public.sales ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sale_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sale_commands ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.sales,public.sale_items,public.sale_commands FROM PUBLIC;
REVOKE ALL ON FUNCTION public.guard_recorded_sale_history(),public.check_sale_stock_origin(),
    public.require_complete_recorded_sale(),public.require_completed_sale_command() FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.sales,public.sale_items,public.sale_commands FROM %I',browser_role);
        EXECUTE format('REVOKE ALL ON FUNCTION public.guard_recorded_sale_history(),public.check_sale_stock_origin(),public.require_complete_recorded_sale(),public.require_completed_sale_command() FROM %I',browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.sales) OR EXISTS(SELECT 1 FROM public.sale_commands) THEN
        RAISE EXCEPTION 'Sales rollback requires no sales/command records';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER sale_stock_complete ON public.product_stock_movements;
DROP TRIGGER stock_sale_origin_guard ON public.product_stock_movements;
ALTER TABLE public.product_stock_movements
    DROP CONSTRAINT stock_sale_item_fk,DROP CONSTRAINT stock_sale_origin,
    DROP CONSTRAINT stock_movement_sign,DROP CONSTRAINT stock_movement_kind,
    DROP COLUMN sale_quantity,DROP COLUMN sale_item_id,
    ADD CONSTRAINT product_stock_movements_kind_check CHECK(kind IN('opening','receipt','adjustment')),
    ADD CONSTRAINT product_stock_movements_check CHECK(kind='adjustment' OR quantity_delta>0);
DROP TABLE public.sale_commands;
DROP TABLE public.sale_items;
DROP TABLE public.sales;
DROP FUNCTION public.require_completed_sale_command();
DROP FUNCTION public.require_complete_recorded_sale();
DROP FUNCTION public.check_sale_stock_origin();
DROP FUNCTION public.guard_recorded_sale_history();
ALTER TABLE public.appointments DROP CONSTRAINT appointment_sale_professional;
