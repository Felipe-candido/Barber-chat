-- Run after common migrations, in the same database used by the Go API.
-- Edit these values first. This is explicit manual provisioning, not an API seed.
-- Existing professionals/configuration are preserved; divergent hours abort.
BEGIN;
DO $$
DECLARE
    target_slug TEXT := 'ALTERE-O-SLUG';
    professional_name TEXT := 'ALTERE-O-NOME';
    slot_minutes INTEGER := 60;
    extra_buffer_minutes INTEGER := 0;
    weekdays INTEGER[] := ARRAY[1,2,3,4,5,6]; -- Sunday=0, Saturday=6.
    opening_minute INTEGER := 570; -- 09:30.
    closing_minute INTEGER := 1110; -- 18:30, same civil day.
    service_ids UUID[] := ARRAY[]::UUID[]; -- Empty: all CURRENT active services.
    target_shop UUID;
    target_professional UUID;
    matches INTEGER;
    day INTEGER;
BEGIN
    IF target_slug = 'ALTERE-O-SLUG' OR professional_name = 'ALTERE-O-NOME' OR btrim(professional_name) = '' THEN
        RAISE EXCEPTION 'Set target_slug and professional_name before executing';
    END IF;
    SELECT id INTO STRICT target_shop FROM shops WHERE slug = target_slug AND active = TRUE FOR SHARE;
    SELECT count(*) INTO matches FROM professionals WHERE shop_id = target_shop AND name = professional_name;
    IF matches > 1 THEN RAISE EXCEPTION 'Ambiguous professional name; choose an existing ID manually'; END IF;
    IF matches = 0 THEN
        target_professional := gen_random_uuid();
        INSERT INTO professionals (id,shop_id,name) VALUES (target_professional,target_shop,professional_name);
    ELSE
        SELECT id INTO STRICT target_professional FROM professionals
            WHERE shop_id = target_shop AND name = professional_name AND active = TRUE;
    END IF;
    PERFORM 1 FROM professionals WHERE shop_id = target_shop AND id = target_professional FOR UPDATE;
    IF EXISTS (SELECT 1 FROM schedule_settings s WHERE s.shop_id = target_shop AND s.professional_id = target_professional
        AND (s.interval_minutes <> slot_minutes OR s.buffer_minutes <> extra_buffer_minutes)) THEN
        RAISE EXCEPTION 'Existing scheduling policy differs; review it instead of overwriting';
    END IF;
    INSERT INTO schedule_settings (shop_id,professional_id,interval_minutes,buffer_minutes)
        VALUES (target_shop,target_professional,slot_minutes,extra_buffer_minutes) ON CONFLICT DO NOTHING;
    IF cardinality(weekdays) = 0 OR EXISTS (SELECT 1 FROM unnest(weekdays) d WHERE d NOT BETWEEN 0 AND 6 OR d IS NULL) THEN
        RAISE EXCEPTION 'Use at least one weekday between 0 and 6';
    END IF;
    IF EXISTS (SELECT 1 FROM working_hours WHERE shop_id = target_shop AND professional_id = target_professional
        AND (weekday <> ALL(weekdays) OR opens_at_minute <> opening_minute OR closes_at_minute <> closing_minute)) THEN
        RAISE EXCEPTION 'Existing working hours differ; review them instead of overwriting';
    END IF;
    FOREACH day IN ARRAY weekdays LOOP
        IF NOT EXISTS (SELECT 1 FROM working_hours WHERE shop_id = target_shop AND professional_id = target_professional AND weekday = day) THEN
            INSERT INTO working_hours (id,shop_id,professional_id,weekday,opens_at_minute,closes_at_minute)
                VALUES (gen_random_uuid(),target_shop,target_professional,day,opening_minute,closing_minute);
        END IF;
    END LOOP;
    IF EXISTS (SELECT 1 FROM unnest(service_ids) requested WHERE NOT EXISTS (
        SELECT 1 FROM services s WHERE s.shop_id = target_shop AND s.id = requested AND s.active = TRUE
            AND s.duration_minutes BETWEEN 1 AND 1440 AND s.price_cents <= 9007199254740991)) THEN
        RAISE EXCEPTION 'A requested service is inactive, unsupported or belongs to another shop';
    END IF;
    INSERT INTO professional_services (shop_id,professional_id,service_id)
        SELECT target_shop,target_professional,id FROM services
        WHERE shop_id = target_shop AND active = TRUE AND duration_minutes BETWEEN 1 AND 1440 AND price_cents <= 9007199254740991
            AND (cardinality(service_ids) = 0 OR id = ANY(service_ids)) ON CONFLICT DO NOTHING;
    IF NOT EXISTS (SELECT 1 FROM professional_services WHERE shop_id = target_shop AND professional_id = target_professional) THEN
        RAISE EXCEPTION 'Create an active bookable service first';
    END IF;
    RAISE NOTICE 'Calendar ready. Professional ID: %. Public path: /b/%', target_professional,target_slug;
END;
$$;
COMMIT;
