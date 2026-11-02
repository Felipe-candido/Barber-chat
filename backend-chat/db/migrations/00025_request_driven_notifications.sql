-- +goose Up
-- Existing policies keep automatic semantics; new settings default to request.
ALTER TABLE public.shop_notification_settings ADD COLUMN delivery_mode VARCHAR(12) NOT NULL DEFAULT 'automatic'
 CHECK(delivery_mode IN('request','automatic'));
ALTER TABLE public.shop_notification_settings ALTER COLUMN delivery_mode SET DEFAULT 'request';
-- Existing immutable plans/jobs are never widened or silently backfilled.
ALTER TABLE public.notification_jobs ADD COLUMN dispatch_mode VARCHAR(12) NOT NULL DEFAULT 'automatic'
 CHECK(dispatch_mode IN('request','automatic'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.notification_jobs WHERE dispatch_mode='request') THEN
  RAISE EXCEPTION 'Request-mode rollback requires no request-driven jobs';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.notification_jobs DROP COLUMN dispatch_mode;
ALTER TABLE public.shop_notification_settings DROP COLUMN delivery_mode;
