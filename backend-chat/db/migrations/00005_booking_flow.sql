-- +goose Up
ALTER TABLE services ADD CONSTRAINT services_shop_id_unique UNIQUE (shop_id, id);

CREATE TABLE professionals (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL REFERENCES shops(id),
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (shop_id, id)
);
CREATE TABLE professional_services (
    shop_id UUID NOT NULL,
    professional_id UUID NOT NULL,
    service_id UUID NOT NULL,
    PRIMARY KEY (shop_id, professional_id, service_id),
    FOREIGN KEY (shop_id, professional_id) REFERENCES professionals(shop_id, id),
    FOREIGN KEY (shop_id, service_id) REFERENCES services(shop_id, id)
);
CREATE TABLE schedule_settings (
    shop_id UUID NOT NULL,
    professional_id UUID NOT NULL,
    interval_minutes INTEGER NOT NULL CHECK (interval_minutes BETWEEN 1 AND 1440),
    buffer_minutes INTEGER NOT NULL DEFAULT 0 CHECK (buffer_minutes BETWEEN 0 AND 1440),
    PRIMARY KEY (shop_id, professional_id),
    FOREIGN KEY (shop_id, professional_id) REFERENCES professionals(shop_id, id)
);
CREATE TABLE working_hours (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL,
    professional_id UUID NOT NULL,
    weekday INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    opens_at_minute INTEGER NOT NULL CHECK (opens_at_minute BETWEEN 0 AND 1439),
    closes_at_minute INTEGER NOT NULL CHECK (closes_at_minute BETWEEN 1 AND 1440),
    CHECK (closes_at_minute > opens_at_minute),
    FOREIGN KEY (shop_id, professional_id) REFERENCES professionals(shop_id, id),
    EXCLUDE USING gist (shop_id WITH =, professional_id WITH =, weekday WITH =,
        int4range(opens_at_minute, closes_at_minute, '[)') WITH &&)
);
CREATE TABLE schedule_blocks (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL,
    professional_id UUID NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL CHECK (ends_at > starts_at),
    FOREIGN KEY (shop_id, professional_id) REFERENCES professionals(shop_id, id)
);
CREATE INDEX schedule_blocks_lookup ON schedule_blocks (shop_id, professional_id, starts_at);

CREATE TABLE customers (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL REFERENCES shops(id),
    first_name VARCHAR(100) NOT NULL CHECK (length(btrim(first_name)) > 0),
    last_name VARCHAR(100) NOT NULL CHECK (length(btrim(last_name)) > 0),
    phone_e164 VARCHAR(16) NOT NULL CHECK (phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (shop_id, id),
    UNIQUE (shop_id, phone_e164)
);
CREATE TABLE appointments (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL REFERENCES shops(id),
    customer_id UUID NOT NULL,
    service_id UUID NOT NULL,
    professional_id UUID NOT NULL,
    service_name VARCHAR(100) NOT NULL CHECK (length(btrim(service_name)) > 0),
    duration_minutes INTEGER NOT NULL CHECK (duration_minutes BETWEEN 1 AND 1440),
    price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
    currency VARCHAR(3) NOT NULL CHECK (currency = 'BRL'),
    professional_name VARCHAR(100) NOT NULL CHECK (length(btrim(professional_name)) > 0),
    shop_timezone VARCHAR(100) NOT NULL CHECK (length(btrim(shop_timezone)) > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL CHECK (ends_at > starts_at),
    status VARCHAR(20) NOT NULL CHECK (status IN ('scheduled','confirmed','cancelled','completed','no_show')),
    version BIGINT NOT NULL CHECK (version > 0),
    code_hash BYTEA NOT NULL CHECK (octet_length(code_hash) = 32),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (shop_id, customer_id) REFERENCES customers(shop_id, id),
    FOREIGN KEY (shop_id, service_id) REFERENCES services(shop_id, id),
    FOREIGN KEY (shop_id, professional_id) REFERENCES professionals(shop_id, id),
    CHECK (ends_at - starts_at >= duration_minutes * INTERVAL '1 minute'),
    UNIQUE (shop_id, code_hash),
    CONSTRAINT appointments_no_overlap EXCLUDE USING gist (
        shop_id WITH =, professional_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status <> 'cancelled')
);
CREATE TABLE booking_requests (
    shop_id UUID NOT NULL REFERENCES shops(id),
    key_hash BYTEA NOT NULL CHECK (octet_length(key_hash) = 32),
    fingerprint BYTEA NOT NULL CHECK (octet_length(fingerprint) = 32),
    encrypted_receipt BYTEA,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (shop_id, key_hash)
);

-- Manual calendar writes follow the same lock as API bookings. The BEFORE
-- triggers also reject inserting blocks over existing non-cancelled bookings.
-- +goose StatementBegin
CREATE FUNCTION lock_booking_calendar() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_shop UUID; target_professional UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_shop := OLD.shop_id; target_professional := OLD.professional_id;
    ELSE
        target_shop := NEW.shop_id; target_professional := NEW.professional_id;
        IF TG_OP = 'UPDATE' AND (OLD.shop_id <> NEW.shop_id OR OLD.professional_id <> NEW.professional_id) THEN
            RAISE EXCEPTION 'calendar ownership is immutable' USING ERRCODE = '23514';
        END IF;
    END IF;
    PERFORM 1 FROM professionals WHERE shop_id = target_shop AND id = target_professional FOR UPDATE;
    IF TG_TABLE_NAME = 'schedule_blocks' AND TG_OP <> 'DELETE' THEN
        IF EXISTS (SELECT 1 FROM appointments WHERE shop_id = target_shop AND professional_id = target_professional
            AND status <> 'cancelled' AND starts_at < NEW.ends_at AND ends_at > NEW.starts_at) THEN
            RAISE EXCEPTION 'block overlaps an appointment' USING ERRCODE = '23P01';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER schedule_settings_lock BEFORE INSERT OR UPDATE OR DELETE ON schedule_settings
    FOR EACH ROW EXECUTE FUNCTION lock_booking_calendar();
CREATE TRIGGER working_hours_lock BEFORE INSERT OR UPDATE OR DELETE ON working_hours
    FOR EACH ROW EXECUTE FUNCTION lock_booking_calendar();
CREATE TRIGGER schedule_blocks_lock BEFORE INSERT OR UPDATE OR DELETE ON schedule_blocks
    FOR EACH ROW EXECUTE FUNCTION lock_booking_calendar();

-- All business access goes through Go, not browser Supabase Data API calls.
-- This also closes the earlier missing protection on shops/services.
-- +goose StatementBegin
DO $$
DECLARE table_name TEXT; browser_role TEXT;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['shops','services','professionals','professional_services',
        'schedule_settings','working_hours','schedule_blocks','customers','appointments','booking_requests']
    LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', table_name);
        EXECUTE format('REVOKE ALL ON public.%I FROM PUBLIC', table_name);
        FOR browser_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon','authenticated')
        LOOP
            EXECUTE format('REVOKE ALL ON public.%I FROM %I', table_name, browser_role);
        END LOOP;
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE booking_requests;
DROP TABLE schedule_blocks;
DROP TABLE working_hours;
DROP TABLE schedule_settings;
DROP FUNCTION lock_booking_calendar();
DROP TABLE appointments;
DROP TABLE customers;
DROP TABLE professional_services;
DROP TABLE professionals;
ALTER TABLE services DROP CONSTRAINT services_shop_id_unique;
-- Deliberately preserve RLS/revocations on shops/services after rollback.
