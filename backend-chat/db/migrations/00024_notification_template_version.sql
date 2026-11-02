-- +goose Up
-- NULL means an older attempt without a recorded template, never assume v1.
ALTER TABLE public.notification_attempts ADD COLUMN template_version INTEGER
 CHECK(template_version BETWEEN 1 AND 32767);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.notification_attempts WHERE template_version IS NOT NULL) THEN
  RAISE EXCEPTION 'Template rollback requires no versioned attempts';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.notification_attempts DROP COLUMN template_version;
