-- +goose Up
-- Existing members retain the full access they had before roles were introduced.
ALTER TABLE shop_memberships ADD COLUMN role VARCHAR(10) NOT NULL DEFAULT 'owner';
ALTER TABLE shop_memberships ADD COLUMN professional_id UUID;
ALTER TABLE shop_memberships ADD CONSTRAINT membership_role CHECK (role IN ('owner','barber'));
ALTER TABLE shop_memberships ADD CONSTRAINT membership_barber_link CHECK (
    role <> 'barber' OR (professional_id IS NOT NULL AND professional_id <> '00000000-0000-0000-0000-000000000000'::uuid)
);
ALTER TABLE shop_memberships ADD CONSTRAINT membership_professional_fk
    FOREIGN KEY (shop_id,professional_id) REFERENCES professionals(shop_id,id);
CREATE UNIQUE INDEX membership_professional_user ON shop_memberships(shop_id,professional_id) WHERE professional_id IS NOT NULL;
-- Omitting role in new provisioning must not grant full access.
ALTER TABLE shop_memberships ALTER COLUMN role DROP DEFAULT;

-- +goose Down
DROP INDEX membership_professional_user;
ALTER TABLE shop_memberships DROP CONSTRAINT membership_professional_fk;
ALTER TABLE shop_memberships DROP CONSTRAINT membership_barber_link;
ALTER TABLE shop_memberships DROP CONSTRAINT membership_role;
ALTER TABLE shop_memberships DROP COLUMN professional_id;
ALTER TABLE shop_memberships DROP COLUMN role;
