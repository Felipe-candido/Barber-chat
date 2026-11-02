-- +goose Up
ALTER TABLE public.notification_attempts
 ADD COLUMN receipt_state SMALLINT NOT NULL DEFAULT 0 CHECK(receipt_state BETWEEN 0 AND 3),
 ADD COLUMN receipt_at TIMESTAMPTZ,
 ADD CONSTRAINT notification_receipt_observation CHECK((receipt_state=0 AND receipt_at IS NULL)
  OR(receipt_state>0 AND receipt_at IS NOT NULL AND isfinite(receipt_at)));
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.notification_attempts WHERE receipt_state>0) THEN
  RAISE EXCEPTION 'Receipt rollback requires no recorded provider receipts';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.notification_attempts DROP CONSTRAINT notification_receipt_observation,DROP COLUMN receipt_state,DROP COLUMN receipt_at;
