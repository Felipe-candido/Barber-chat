-- +goose Up
ALTER TABLE public.appointment_notification_plans ADD COLUMN payload_purged_at TIMESTAMPTZ;
ALTER TABLE public.appointment_notification_plans DROP CONSTRAINT appointment_notification_plans_check;
ALTER TABLE public.appointment_notification_plans ADD CONSTRAINT notification_plan_payload CHECK(
 (skip_reason IS NULL AND payload_version IS NOT NULL AND payload_version=1
  AND payload_key_id IS NOT NULL AND payload_key_id ~ '^[A-Za-z0-9_-]{1,64}$'
  AND payload_expires_at IS NOT NULL AND isfinite(payload_expires_at) AND payload_expires_at>created_at
  AND((payload_purged_at IS NULL AND encrypted_payload IS NOT NULL AND octet_length(encrypted_payload) BETWEEN 29 AND 16412)
   OR(payload_purged_at IS NOT NULL AND isfinite(payload_purged_at) AND payload_purged_at>=payload_expires_at AND encrypted_payload IS NULL)))
 OR(skip_reason IS NOT NULL AND payload_version IS NULL AND payload_key_id IS NULL AND encrypted_payload IS NULL
  AND payload_expires_at IS NULL AND reminder_skip_reason IS NULL AND payload_purged_at IS NULL)
);
CREATE INDEX notification_payload_cleanup ON public.appointment_notification_plans(payload_expires_at) WHERE encrypted_payload IS NOT NULL;
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.appointment_notification_plans WHERE payload_purged_at IS NOT NULL) THEN
  RAISE EXCEPTION 'Purged private payloads cannot be restored by a down migration';
 END IF;
END $$;
-- +goose StatementEnd
DROP INDEX public.notification_payload_cleanup;
ALTER TABLE public.appointment_notification_plans DROP CONSTRAINT notification_plan_payload,DROP COLUMN payload_purged_at;
ALTER TABLE public.appointment_notification_plans ADD CONSTRAINT appointment_notification_plans_check CHECK(
 (skip_reason IS NULL AND payload_version IS NOT NULL AND payload_version=1 AND payload_key_id IS NOT NULL
  AND payload_key_id ~ '^[A-Za-z0-9_-]{1,64}$' AND encrypted_payload IS NOT NULL AND octet_length(encrypted_payload) BETWEEN 29 AND 16412
  AND payload_expires_at IS NOT NULL AND isfinite(payload_expires_at) AND payload_expires_at>created_at)
 OR(skip_reason IS NOT NULL AND payload_version IS NULL AND payload_key_id IS NULL AND encrypted_payload IS NULL
  AND payload_expires_at IS NULL AND reminder_skip_reason IS NULL)
);
