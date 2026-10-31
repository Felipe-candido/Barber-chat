-- +goose Up
-- Reuse shops.phone; do not normalize or overwrite legacy values implicitly.
ALTER TABLE public.shops
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD COLUMN settings_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT shop_description_length CHECK (char_length(description) <= 1000),
    ADD CONSTRAINT shop_settings_version_range CHECK (settings_version BETWEEN 1 AND 9007199254740991);

-- +goose Down
-- Refuse to discard descriptions or optimistic-lock history after use.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM public.shops WHERE description <> '' OR settings_version <> 1) THEN
        RAISE EXCEPTION 'Settings rollback requires no descriptions or settings changes';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.shops DROP CONSTRAINT shop_description_length,
    DROP CONSTRAINT shop_settings_version_range,
    DROP COLUMN description, DROP COLUMN settings_version;
