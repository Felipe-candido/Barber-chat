-- +goose Up
-- A movement's FK already holds KEY SHARE on its balance. FOR UPDATE would
-- upgrade two concurrent insertions into a deadlock. Quantity/version are not
-- keys: NO KEY UPDATE serializes writers while remaining compatible with that FK.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.apply_product_stock_movement() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
DECLARE balance public.product_stock%ROWTYPE; next_quantity BIGINT;
BEGIN
    SELECT * INTO balance FROM public.product_stock
    WHERE shop_id=NEW.shop_id AND product_id=NEW.product_id FOR NO KEY UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23503', CONSTRAINT='product_stock_exists', MESSAGE='product stock not found';
    END IF;
    next_quantity := balance.quantity_on_hand::bigint + NEW.quantity_delta::bigint;
    IF next_quantity < 0 THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_available', MESSAGE='insufficient stock';
    END IF;
    IF next_quantity > 2147483647 THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_limit', MESSAGE='stock quantity limit exceeded';
    END IF;
    IF NEW.kind='opening' AND (balance.version<>1 OR balance.quantity_on_hand<>0) THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_initial_zero', MESSAGE='opening must be the first stock operation';
    END IF;
    UPDATE public.product_stock SET quantity_on_hand=next_quantity::integer, version=version+1
    WHERE shop_id=NEW.shop_id AND product_id=NEW.product_id;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Never restore the earlier lock protocol around existing stock records.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.products) OR EXISTS(SELECT 1 FROM public.product_commands) THEN
        RAISE EXCEPTION 'Stock lock rollback requires no product/command records';
    END IF;
END $$;
CREATE OR REPLACE FUNCTION public.apply_product_stock_movement() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
DECLARE balance public.product_stock%ROWTYPE; next_quantity BIGINT;
BEGIN
    SELECT * INTO balance FROM public.product_stock
    WHERE shop_id=NEW.shop_id AND product_id=NEW.product_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23503', CONSTRAINT='product_stock_exists', MESSAGE='product stock not found';
    END IF;
    next_quantity := balance.quantity_on_hand::bigint + NEW.quantity_delta::bigint;
    IF next_quantity < 0 THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_available', MESSAGE='insufficient stock';
    END IF;
    IF next_quantity > 2147483647 THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_limit', MESSAGE='stock quantity limit exceeded';
    END IF;
    IF NEW.kind='opening' AND (balance.version<>1 OR balance.quantity_on_hand<>0) THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_initial_zero', MESSAGE='opening must be the first stock operation';
    END IF;
    UPDATE public.product_stock SET quantity_on_hand=next_quantity::integer, version=version+1
    WHERE shop_id=NEW.shop_id AND product_id=NEW.product_id;
    RETURN NEW;
END $$;
-- +goose StatementEnd
