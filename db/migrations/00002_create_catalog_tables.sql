-- +goose Up

CREATE TABLE shops (
    id UUID PRIMARY KEY,
    name VARCHAR(150) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    timezone VARCHAR(100) NOT NULL DEFAULT 'America/Sao_Paulo',
    phone VARCHAR(30),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT shops_name_not_empty
        CHECK (CHAR_LENGTH(TRIM(name)) > 0),

    CONSTRAINT shops_slug_not_empty
        CHECK (CHAR_LENGTH(TRIM(slug)) > 0)
);

CREATE TABLE services (
    id UUID PRIMARY KEY,
    shop_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    duration_minutes INTEGER NOT NULL,
    price_cents BIGINT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT services_shop_fk
        FOREIGN KEY (shop_id)
        REFERENCES shops(id)
        ON DELETE RESTRICT,

    CONSTRAINT services_name_not_empty
        CHECK (CHAR_LENGTH(TRIM(name)) > 0),

    CONSTRAINT services_duration_positive
        CHECK (duration_minutes > 0),

    CONSTRAINT services_price_non_negative
        CHECK (price_cents >= 0)
);

CREATE INDEX services_shop_active_idx
    ON services (shop_id, active);

-- +goose Down

DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS shops;