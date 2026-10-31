-- +goose Up
CREATE TABLE public.products (
    id UUID PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    shop_id UUID NOT NULL REFERENCES public.shops(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) > 0 AND name !~ '[[:cntrl:]]'),
    description VARCHAR(1000) NOT NULL DEFAULT '',
    price_cents BIGINT NOT NULL CHECK (price_cents BETWEEN 1 AND 9007199254740991),
    currency VARCHAR(3) NOT NULL DEFAULT 'BRL' CHECK (currency = 'BRL'),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK (isfinite(created_at)),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK (isfinite(updated_at) AND updated_at >= created_at),
    UNIQUE (shop_id, id)
);
CREATE INDEX products_shop_page ON public.products(shop_id, created_at DESC, id DESC);

CREATE TABLE public.product_stock (
    shop_id UUID NOT NULL,
    product_id UUID NOT NULL,
    quantity_on_hand INTEGER NOT NULL DEFAULT 0 CHECK (quantity_on_hand >= 0),
    minimum_quantity INTEGER NOT NULL DEFAULT 0 CHECK (minimum_quantity >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    PRIMARY KEY (shop_id, product_id),
    FOREIGN KEY (shop_id, product_id) REFERENCES public.products(shop_id, id) ON DELETE RESTRICT
);

-- Manual movements only in this increment. Sale origins/reversal FKs are added
-- together with real sales tables in a subsequent migration, never as loose UUIDs.
CREATE TABLE public.product_stock_movements (
    id UUID PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    shop_id UUID NOT NULL,
    product_id UUID NOT NULL,
    actor_id UUID NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('opening', 'receipt', 'adjustment')),
    quantity_delta INTEGER NOT NULL CHECK (quantity_delta <> 0 AND quantity_delta >= -2147483647),
    reason VARCHAR(500) NOT NULL CHECK (length(btrim(reason)) > 0 AND reason !~ '[[:cntrl:]]'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now() CHECK (isfinite(created_at)),
    UNIQUE (shop_id, product_id, id),
    FOREIGN KEY (shop_id, product_id) REFERENCES public.product_stock(shop_id, product_id) ON DELETE RESTRICT,
    FOREIGN KEY (actor_id, shop_id) REFERENCES public.shop_memberships(user_id, shop_id) ON DELETE RESTRICT,
    CHECK (kind = 'adjustment' OR quantity_delta > 0)
);
CREATE UNIQUE INDEX product_stock_one_opening ON public.product_stock_movements(shop_id, product_id) WHERE kind='opening';
CREATE INDEX product_stock_movement_page ON public.product_stock_movements(shop_id, product_id, created_at DESC, id DESC);

CREATE TABLE public.product_commands (
    shop_id UUID NOT NULL,
    actor_id UUID NOT NULL,
    operation VARCHAR(24) NOT NULL CHECK (operation IN ('create_product', 'record_stock')),
    key_digest BYTEA NOT NULL CHECK (octet_length(key_digest)=32),
    fingerprint BYTEA NOT NULL CHECK (octet_length(fingerprint)=32),
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    product_id UUID,
    movement_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (shop_id, actor_id, operation, key_digest),
    FOREIGN KEY (actor_id, shop_id) REFERENCES public.shop_memberships(user_id, shop_id) ON DELETE RESTRICT,
    FOREIGN KEY (shop_id, product_id) REFERENCES public.products(shop_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (shop_id, product_id, movement_id) REFERENCES public.product_stock_movements(shop_id, product_id, id) ON DELETE RESTRICT,
    CHECK ((NOT completed AND product_id IS NULL AND movement_id IS NULL) OR
        (completed AND product_id IS NOT NULL AND (operation='create_product' OR movement_id IS NOT NULL)))
);

-- +goose StatementBegin
CREATE FUNCTION public.guard_product_stock_balance() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.quantity_on_hand <> 0 OR NEW.version <> 1 THEN
            RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_initial_zero', MESSAGE='stock must start at zero';
        END IF;
    ELSE
        IF NEW.shop_id <> OLD.shop_id OR NEW.product_id <> OLD.product_id OR
           (NEW.quantity_on_hand <> OLD.quantity_on_hand AND pg_trigger_depth() < 2) THEN
            RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_movement_required', MESSAGE='stock quantity requires a movement';
        END IF;
        IF OLD.version=9223372036854775807 OR NEW.version <> OLD.version+1 THEN
            RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_version', MESSAGE='stock version must advance once';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER product_stock_balance_guard BEFORE INSERT OR UPDATE ON public.product_stock
FOR EACH ROW EXECUTE FUNCTION public.guard_product_stock_balance();

CREATE FUNCTION public.apply_product_stock_movement() RETURNS trigger
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
-- AFTER INSERT matters: an ignored duplicate INSERT must never change quantity.
CREATE TRIGGER product_stock_apply AFTER INSERT ON public.product_stock_movements
FOR EACH ROW EXECUTE FUNCTION public.apply_product_stock_movement();

CREATE FUNCTION public.reject_product_stock_history_change() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_stock_history_immutable', MESSAGE='stock history is append only';
END $$;
CREATE TRIGGER product_stock_history_guard BEFORE UPDATE OR DELETE ON public.product_stock_movements
FOR EACH ROW EXECUTE FUNCTION public.reject_product_stock_history_change();

CREATE FUNCTION public.require_completed_product_command() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.product_commands WHERE shop_id=NEW.shop_id AND actor_id=NEW.actor_id
        AND operation=NEW.operation AND key_digest=NEW.key_digest AND NOT completed) THEN
        RAISE EXCEPTION USING ERRCODE='23514', CONSTRAINT='product_command_must_complete', MESSAGE='incomplete product command cannot commit';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER product_command_complete AFTER INSERT OR UPDATE ON public.product_commands
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.require_completed_product_command();

ALTER TABLE public.products ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.product_stock ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.product_stock_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.product_commands ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.products,public.product_stock,public.product_stock_movements,public.product_commands FROM PUBLIC;
REVOKE ALL ON FUNCTION public.guard_product_stock_balance(),public.apply_product_stock_movement(),
    public.reject_product_stock_history_change(),public.require_completed_product_command() FROM PUBLIC;
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon','authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.products,public.product_stock,public.product_stock_movements,public.product_commands FROM %I',browser_role);
        EXECUTE format('REVOKE ALL ON FUNCTION public.guard_product_stock_balance(),public.apply_product_stock_movement(),public.reject_product_stock_history_change(),public.require_completed_product_command() FROM %I',browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.products) OR EXISTS(SELECT 1 FROM public.product_commands) THEN
        RAISE EXCEPTION 'Product rollback requires no product/command records';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.product_commands;
DROP TABLE public.product_stock_movements;
DROP TABLE public.product_stock;
DROP TABLE public.products;
DROP FUNCTION public.require_completed_product_command();
DROP FUNCTION public.reject_product_stock_history_change();
DROP FUNCTION public.apply_product_stock_movement();
DROP FUNCTION public.guard_product_stock_balance();
