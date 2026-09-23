-- +goose Up
-- Required by the planned tenant/professional time-range exclusion constraint.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- +goose Down
-- Extensions may be shared. Deliberately retain btree_gist on rollback.
SELECT 1;
