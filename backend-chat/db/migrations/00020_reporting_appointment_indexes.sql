-- +goose Up
-- The overlap GiST excludes cancelled rows and does not order historical reads
-- by starts_at. These cover unit-wide and personal date-range reporting/agenda.
CREATE INDEX appointments_shop_period_page
    ON public.appointments(shop_id,starts_at DESC,id DESC);
CREATE INDEX appointments_professional_period_page
    ON public.appointments(shop_id,professional_id,starts_at DESC,id DESC);

-- +goose Down
DROP INDEX public.appointments_professional_period_page;
DROP INDEX public.appointments_shop_period_page;
