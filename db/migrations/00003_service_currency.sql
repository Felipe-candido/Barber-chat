-- +goose Up
-- The initial catalog operates in BRL. Preserve currency with the price.
ALTER TABLE services ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT 'BRL';
ALTER TABLE services ADD CONSTRAINT services_currency_brl CHECK (currency = 'BRL');

-- +goose Down
ALTER TABLE services DROP CONSTRAINT services_currency_brl;
ALTER TABLE services DROP COLUMN currency;
