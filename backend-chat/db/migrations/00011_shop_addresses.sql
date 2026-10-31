-- +goose Up
-- Legacy shops remain valid without inventing addresses. New registrations must
-- write a complete address in the same transaction as the shop and its owner.
CREATE TABLE public.shop_addresses (
    shop_id UUID PRIMARY KEY REFERENCES public.shops(id) ON DELETE RESTRICT,
    country_code VARCHAR(2) NOT NULL DEFAULT 'BR' CHECK (country_code = 'BR'),
    postal_code VARCHAR(8) NOT NULL CHECK (postal_code ~ '^[0-9]{8}$'),
    street VARCHAR(150) NOT NULL CHECK (street = btrim(street) AND street <> '' AND street !~ '[[:cntrl:]]'),
    number VARCHAR(20) NOT NULL CHECK (number = btrim(number) AND number <> '' AND number !~ '[[:cntrl:]]'),
    complement VARCHAR(100) NOT NULL DEFAULT '' CHECK (complement = btrim(complement) AND complement !~ '[[:cntrl:]]'),
    district VARCHAR(100) NOT NULL CHECK (district = btrim(district) AND district <> '' AND district !~ '[[:cntrl:]]'),
    city VARCHAR(100) NOT NULL CHECK (city = btrim(city) AND city <> '' AND city !~ '[[:cntrl:]]'),
    state VARCHAR(2) NOT NULL CHECK (state IN ('AC','AL','AP','AM','BA','CE','DF','ES','GO','MA','MT','MS','MG','PA','PB','PR','PE','PI','RJ','RN','RS','RO','RR','SC','SP','SE','TO')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE public.shop_addresses ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON public.shop_addresses FROM PUBLIC;
-- +goose StatementBegin
DO $$ DECLARE browser_role TEXT; BEGIN
    FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon', 'authenticated') LOOP
        EXECUTE format('REVOKE ALL ON public.shop_addresses FROM %I', browser_role);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM public.shop_addresses) THEN
        RAISE EXCEPTION 'Address rollback requires no saved addresses';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.shop_addresses;
