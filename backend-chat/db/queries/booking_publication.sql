-- name: ListPublicationWorkingWindows :many
SELECT ss.professional_id, ss.interval_minutes, ss.buffer_minutes,
    wh.opens_at_minute, wh.closes_at_minute
FROM public.schedule_settings ss
JOIN public.working_hours wh ON wh.shop_id=ss.shop_id AND wh.professional_id=ss.professional_id
WHERE ss.shop_id=$1;
