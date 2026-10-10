-- +goose Up
-- Keep provisioning history while allowing exactly one current account per barber.
ALTER TABLE staff_accounts ADD COLUMN replaced_at TIMESTAMPTZ;
ALTER TABLE staff_accounts DROP CONSTRAINT staff_accounts_shop_id_professional_id_key;
CREATE UNIQUE INDEX staff_accounts_current_professional
    ON staff_accounts(shop_id,professional_id) WHERE replaced_at IS NULL;

-- +goose Down
-- Do not silently discard historical requests or invalidate duplicate old links.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM staff_accounts WHERE replaced_at IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot roll back account replacement while history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX staff_accounts_current_professional;
ALTER TABLE staff_accounts ADD CONSTRAINT staff_accounts_shop_id_professional_id_key UNIQUE(shop_id,professional_id);
ALTER TABLE staff_accounts DROP COLUMN replaced_at;
