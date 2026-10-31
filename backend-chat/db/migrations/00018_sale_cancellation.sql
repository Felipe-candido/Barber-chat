-- +goose Up
ALTER TABLE public.sales
    DROP CONSTRAINT sales_status_check, DROP CONSTRAINT sales_version_check,
    ADD COLUMN cancelled_by_user_id UUID,
    ADD COLUMN cancellation_reason VARCHAR(500),
    ADD COLUMN restock BOOLEAN,
    ADD COLUMN cancelled_at TIMESTAMPTZ,
    ADD CONSTRAINT sale_cancellation_actor FOREIGN KEY(cancelled_by_user_id,shop_id)
        REFERENCES public.shop_memberships(user_id,shop_id) ON DELETE RESTRICT,
    ADD CONSTRAINT sale_cancellation_shape CHECK(
        (status='recorded' AND version=1 AND cancelled_by_user_id IS NULL AND cancellation_reason IS NULL AND restock IS NULL AND cancelled_at IS NULL)
        OR (status='cancelled' AND version=2 AND cancelled_by_user_id IS NOT NULL
            AND cancellation_reason IS NOT NULL AND length(btrim(cancellation_reason))>0
            AND cancellation_reason !~ '[[:cntrl:]]' AND restock IS NOT NULL
            AND cancelled_at IS NOT NULL AND isfinite(cancelled_at) AND cancelled_at>=sold_at));

ALTER TABLE public.sale_commands
    DROP CONSTRAINT sale_commands_operation_check,
    DROP CONSTRAINT sale_commands_shop_id_sale_id_actor_id_fkey,
    ADD CONSTRAINT sale_command_operation CHECK(operation IN('record_sale','cancel_sale')),
    ADD CONSTRAINT sale_command_sale FOREIGN KEY(shop_id,sale_id) REFERENCES public.sales(shop_id,id) ON DELETE RESTRICT;

ALTER TABLE public.product_stock_movements
    DROP CONSTRAINT stock_sale_item_fk, DROP CONSTRAINT stock_sale_origin,
    DROP CONSTRAINT stock_movement_kind, DROP CONSTRAINT stock_movement_sign,
    DROP COLUMN sale_quantity,
    ADD COLUMN reverses_movement_id UUID,
    ADD COLUMN sale_quantity INTEGER GENERATED ALWAYS AS
        (CASE WHEN kind='sale' THEN -quantity_delta WHEN kind='sale_reversal' THEN quantity_delta END) STORED,
    ADD CONSTRAINT stock_movement_kind CHECK(kind IN('opening','receipt','adjustment','sale','sale_reversal')),
    ADD CONSTRAINT stock_movement_sign CHECK(kind='adjustment' OR
        (kind IN('opening','receipt','sale_reversal') AND quantity_delta>0) OR (kind='sale' AND quantity_delta<0)),
    ADD CONSTRAINT stock_sale_origin CHECK((kind IN('sale','sale_reversal'))=(sale_item_id IS NOT NULL)),
    ADD CONSTRAINT stock_reversal_origin CHECK((kind='sale_reversal')=(reverses_movement_id IS NOT NULL)
        AND reverses_movement_id IS DISTINCT FROM id),
    ADD CONSTRAINT stock_sale_item_fk FOREIGN KEY(shop_id,product_id,sale_item_id,sale_quantity)
        REFERENCES public.sale_items(shop_id,product_id,id,quantity) ON DELETE RESTRICT,
    ADD CONSTRAINT stock_movement_exact_origin UNIQUE(shop_id,product_id,id,sale_item_id,sale_quantity),
    ADD CONSTRAINT stock_reversal_exact_origin FOREIGN KEY(shop_id,product_id,reverses_movement_id,sale_item_id,sale_quantity)
        REFERENCES public.product_stock_movements(shop_id,product_id,id,sale_item_id,sale_quantity) ON DELETE RESTRICT;
CREATE UNIQUE INDEX stock_reversal_once ON public.product_stock_movements(shop_id,reverses_movement_id) WHERE kind='sale_reversal';

-- +goose StatementBegin
CREATE FUNCTION public.guard_sale_cancellation() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF TG_OP='UPDATE' AND OLD.status='recorded' AND NEW.status='cancelled'
        AND NEW.version=OLD.version+1
        AND (to_jsonb(OLD)-ARRAY['status','version','cancelled_by_user_id','cancellation_reason','restock','cancelled_at'])
            = (to_jsonb(NEW)-ARRAY['status','version','cancelled_by_user_id','cancellation_reason','restock','cancelled_at']) THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='sale_history_immutable',MESSAGE='only the first audited cancellation may change a sale';
END $$;
DROP TRIGGER sales_history_guard ON public.sales;
CREATE TRIGGER sales_history_guard BEFORE UPDATE OR DELETE ON public.sales
FOR EACH ROW EXECUTE FUNCTION public.guard_sale_cancellation();

CREATE OR REPLACE FUNCTION public.check_sale_stock_origin() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.kind='sale' AND NOT EXISTS(
        SELECT 1 FROM public.sale_items i JOIN public.sales s ON s.shop_id=i.shop_id AND s.id=i.sale_id
        WHERE i.shop_id=NEW.shop_id AND i.id=NEW.sale_item_id AND i.product_id=NEW.product_id
            AND s.recorded_by_user_id=NEW.actor_id AND s.status='recorded') THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='stock_sale_actor',MESSAGE='stock movement does not match its recorded sale';
    END IF;
    IF NEW.kind='sale_reversal' AND NOT EXISTS(
        SELECT 1 FROM public.product_stock_movements m
        JOIN public.sale_items i ON i.shop_id=m.shop_id AND i.id=m.sale_item_id
        JOIN public.sales s ON s.shop_id=i.shop_id AND s.id=i.sale_id
        WHERE m.shop_id=NEW.shop_id AND m.id=NEW.reverses_movement_id AND m.kind='sale'
            AND m.product_id=NEW.product_id AND m.sale_item_id=NEW.sale_item_id
            AND -m.quantity_delta=NEW.quantity_delta AND NEW.created_at>=m.created_at
            AND s.status='cancelled' AND s.restock AND s.cancelled_by_user_id=NEW.actor_id
            AND s.cancellation_reason=NEW.reason AND s.cancelled_at=NEW.created_at) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='stock_reversal_cancellation',MESSAGE='reversal must exactly match its cancelled sale and original withdrawal';
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION public.require_complete_recorded_sale() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE target UUID; item_count BIGINT; item_total NUMERIC; header public.sales%ROWTYPE;
BEGIN
    IF TG_TABLE_NAME='sales' THEN target:=NEW.id;
    ELSIF TG_TABLE_NAME='sale_items' THEN target:=NEW.sale_id;
    ELSE
        IF NEW.kind NOT IN('sale','sale_reversal') THEN RETURN NULL; END IF;
        SELECT sale_id INTO target FROM public.sale_items WHERE shop_id=NEW.shop_id AND id=NEW.sale_item_id;
    END IF;
    SELECT * INTO header FROM public.sales WHERE shop_id=NEW.shop_id AND id=target;
    SELECT count(*),COALESCE(sum(line_total_cents),0) INTO item_count,item_total
        FROM public.sale_items WHERE shop_id=NEW.shop_id AND sale_id=target;
    IF header.id IS NULL OR item_count NOT BETWEEN 1 AND 50 OR item_total<>header.total_cents OR EXISTS(
        SELECT 1 FROM public.sale_items i WHERE i.shop_id=NEW.shop_id AND i.sale_id=target
        AND (NOT EXISTS(SELECT 1 FROM public.product_stock_movements m
                WHERE m.shop_id=i.shop_id AND m.sale_item_id=i.id AND m.kind='sale')
            OR (header.status='cancelled' AND header.restock) IS DISTINCT FROM EXISTS(
                SELECT 1 FROM public.product_stock_movements r
                WHERE r.shop_id=i.shop_id AND r.sale_item_id=i.id AND r.kind='sale_reversal'))) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='recorded_sale_complete',MESSAGE='sale, items, withdrawals and requested reversals must commit together';
    END IF;
    RETURN NULL;
END $$;
DROP TRIGGER sales_complete ON public.sales;
CREATE CONSTRAINT TRIGGER sales_complete AFTER INSERT OR UPDATE ON public.sales
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_complete_recorded_sale();

CREATE FUNCTION public.check_sale_command_result() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    IF NEW.completed AND NOT EXISTS(SELECT 1 FROM public.sales s WHERE s.shop_id=NEW.shop_id AND s.id=NEW.sale_id
        AND ((NEW.operation='record_sale' AND s.recorded_by_user_id=NEW.actor_id)
            OR (NEW.operation='cancel_sale' AND s.status='cancelled'))) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='sale_command_result',MESSAGE='command result must match its operation and tenant';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER sale_command_result BEFORE INSERT OR UPDATE ON public.sale_commands
FOR EACH ROW EXECUTE FUNCTION public.check_sale_command_result();

REVOKE ALL ON FUNCTION public.guard_sale_cancellation(),public.check_sale_command_result() FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON FUNCTION public.guard_sale_cancellation(),public.check_sale_command_result() FROM %I',browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.sales) OR EXISTS(SELECT 1 FROM public.sale_commands) THEN
        RAISE EXCEPTION 'Cancellation rollback requires no sales/command records';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER sale_command_result ON public.sale_commands;
DROP FUNCTION public.check_sale_command_result();
DROP TRIGGER sales_history_guard ON public.sales;
CREATE TRIGGER sales_history_guard BEFORE UPDATE OR DELETE ON public.sales
FOR EACH ROW EXECUTE FUNCTION public.guard_recorded_sale_history();
DROP FUNCTION public.guard_sale_cancellation();
DROP TRIGGER sales_complete ON public.sales;
CREATE CONSTRAINT TRIGGER sales_complete AFTER INSERT ON public.sales
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_complete_recorded_sale();
ALTER TABLE public.product_stock_movements
    DROP CONSTRAINT stock_reversal_exact_origin, DROP CONSTRAINT stock_movement_exact_origin,
    DROP CONSTRAINT stock_sale_item_fk, DROP CONSTRAINT stock_reversal_origin,
    DROP CONSTRAINT stock_sale_origin, DROP CONSTRAINT stock_movement_sign, DROP CONSTRAINT stock_movement_kind;
DROP INDEX public.stock_reversal_once;
ALTER TABLE public.product_stock_movements
    DROP COLUMN reverses_movement_id, DROP COLUMN sale_quantity,
    ADD COLUMN sale_quantity INTEGER GENERATED ALWAYS AS (CASE WHEN kind='sale' THEN -quantity_delta END) STORED,
    ADD CONSTRAINT stock_movement_kind CHECK(kind IN('opening','receipt','adjustment','sale')),
    ADD CONSTRAINT stock_movement_sign CHECK(kind='adjustment' OR (kind IN('opening','receipt') AND quantity_delta>0) OR (kind='sale' AND quantity_delta<0)),
    ADD CONSTRAINT stock_sale_origin CHECK((kind='sale')=(sale_item_id IS NOT NULL)),
    ADD CONSTRAINT stock_sale_item_fk FOREIGN KEY(shop_id,product_id,sale_item_id,sale_quantity)
        REFERENCES public.sale_items(shop_id,product_id,id,quantity) ON DELETE RESTRICT;
ALTER TABLE public.sale_commands
    DROP CONSTRAINT sale_command_operation, DROP CONSTRAINT sale_command_sale,
    ADD CONSTRAINT sale_commands_operation_check CHECK(operation='record_sale'),
    ADD CONSTRAINT sale_commands_shop_id_sale_id_actor_id_fkey FOREIGN KEY(shop_id,sale_id,actor_id)
        REFERENCES public.sales(shop_id,id,recorded_by_user_id) ON DELETE RESTRICT;
ALTER TABLE public.sales
    DROP CONSTRAINT sale_cancellation_shape, DROP CONSTRAINT sale_cancellation_actor,
    DROP COLUMN cancelled_by_user_id, DROP COLUMN cancellation_reason, DROP COLUMN restock, DROP COLUMN cancelled_at,
    ADD CONSTRAINT sales_status_check CHECK(status='recorded'), ADD CONSTRAINT sales_version_check CHECK(version=1);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.check_sale_stock_origin() RETURNS trigger
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
CREATE OR REPLACE FUNCTION public.require_complete_recorded_sale() RETURNS trigger
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
    SELECT count(*),COALESCE(sum(line_total_cents),0) INTO item_count,item_total FROM public.sale_items WHERE shop_id=NEW.shop_id AND sale_id=target;
    IF expected IS NULL OR item_count NOT BETWEEN 1 AND 50 OR item_total<>expected OR EXISTS(
        SELECT 1 FROM public.sale_items i WHERE i.shop_id=NEW.shop_id AND i.sale_id=target AND NOT EXISTS(
            SELECT 1 FROM public.product_stock_movements m WHERE m.shop_id=i.shop_id AND m.product_id=i.product_id AND m.sale_item_id=i.id AND m.kind='sale')) THEN
        RAISE EXCEPTION USING ERRCODE='23514',CONSTRAINT='recorded_sale_complete',MESSAGE='sale totals, items and stock must commit together';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
