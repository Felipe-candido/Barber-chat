-- Read projections only. Each branch is tenant-scoped before grouping/keyset/limit.
-- Keep the two activity CTEs aligned; both are exercised against real PostgreSQL.

-- name: ListReportActivity :many
WITH activity AS (
    SELECT a.shop_id, 'services'::text AS source, a.id AS record_id,
        '00000000-0000-0000-0000-000000000000'::uuid AS item_id,
        a.service_id, '00000000-0000-0000-0000-000000000000'::uuid AS product_id,
        a.professional_id, a.service_name::text AS label, a.professional_name::text AS professional_name,
        a.currency::text AS currency, a.status::text AS appointment_status, a.status_source::text AS status_source,
        ''::text AS sale_status, (c.first_name || ' ' || c.last_name)::text AS customer_name,
        c.phone_e164::text AS customer_phone, a.starts_at AS occurred_at,
        1::bigint AS quantity, a.price_cents::bigint AS unit_price_cents,
        a.price_cents::bigint AS value_cents,
        to_char(a.starts_at AT TIME ZONE sqlc.arg(timezone)::text, 'YYYY-MM-DD')::text AS local_date
    FROM public.appointments a
    JOIN public.customers c ON c.shop_id=a.shop_id AND c.id=a.customer_id
    JOIN public.shops sh ON sh.id=a.shop_id AND sh.active
    WHERE a.shop_id=sqlc.arg(shop_id)
      AND sqlc.arg(source)::text IN ('all','services')
      AND a.starts_at>=sqlc.arg(from_at)::timestamptz AND a.starts_at<sqlc.arg(until_at)::timestamptz
      AND (sqlc.arg(professional_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR a.professional_id=sqlc.arg(professional_id))
      AND (sqlc.arg(service_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR a.service_id=sqlc.arg(service_id))
      AND (sqlc.arg(name_pattern)::text='' OR (c.first_name || ' ' || c.last_name) ILIKE sqlc.arg(name_pattern) ESCAPE E'\\')
      AND (sqlc.arg(phone)::text='' OR c.phone_e164=sqlc.arg(phone))
      AND (sqlc.arg(appointment_status)::text='' OR a.status=sqlc.arg(appointment_status))
    UNION ALL
    SELECT s.shop_id, 'products'::text, s.id, i.id,
        '00000000-0000-0000-0000-000000000000'::uuid, i.product_id,
        COALESCE(s.professional_id,'00000000-0000-0000-0000-000000000000'::uuid),
        i.product_name_snapshot::text, s.professional_name_snapshot::text, i.currency::text,
        ''::text, ''::text, s.status::text,
        COALESCE(c.first_name || ' ' || c.last_name,'')::text, COALESCE(c.phone_e164,'')::text,
        s.sold_at, i.quantity::bigint, i.unit_price_cents, i.line_total_cents,
        to_char(s.sold_at AT TIME ZONE sqlc.arg(timezone)::text, 'YYYY-MM-DD')::text
    FROM public.sales s
    JOIN public.sale_items i ON i.shop_id=s.shop_id AND i.sale_id=s.id
    JOIN public.shops sh ON sh.id=s.shop_id AND sh.active
    LEFT JOIN public.appointments a ON a.shop_id=s.shop_id AND a.id=s.appointment_id
    LEFT JOIN public.customers c ON c.shop_id=a.shop_id AND c.id=a.customer_id
    WHERE s.shop_id=sqlc.arg(shop_id)
      AND sqlc.arg(source)::text IN ('all','products')
      AND s.sold_at>=sqlc.arg(from_at)::timestamptz AND s.sold_at<sqlc.arg(until_at)::timestamptz
      AND (sqlc.arg(professional_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR s.professional_id=sqlc.arg(professional_id))
      AND (sqlc.arg(product_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR i.product_id=sqlc.arg(product_id))
      AND (sqlc.arg(name_pattern)::text='' OR (c.first_name || ' ' || c.last_name) ILIKE sqlc.arg(name_pattern) ESCAPE E'\\')
      AND (sqlc.arg(phone)::text='' OR c.phone_e164=sqlc.arg(phone))
      AND (sqlc.arg(sale_status)::text='' OR s.status=sqlc.arg(sale_status))
)
SELECT shop_id,source,record_id,item_id,service_id,product_id,professional_id,label,
    professional_name,currency,appointment_status,status_source,sale_status,
    customer_name,customer_phone,occurred_at,quantity,unit_price_cents,value_cents
FROM activity
WHERE NOT sqlc.arg(has_cursor)::boolean OR
    (occurred_at,source COLLATE "C",record_id,item_id) <
    (sqlc.arg(after_at)::timestamptz,sqlc.arg(after_source)::text COLLATE "C",sqlc.arg(after_id)::uuid,sqlc.arg(after_item_id)::uuid)
ORDER BY occurred_at DESC,source COLLATE "C" DESC,record_id DESC,item_id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: ReadReportActivityBuckets :many
WITH activity AS (
    SELECT a.shop_id, 'services'::text AS source, a.id AS record_id,
        '00000000-0000-0000-0000-000000000000'::uuid AS item_id,
        a.service_id, '00000000-0000-0000-0000-000000000000'::uuid AS product_id,
        a.professional_id, a.service_name::text AS label, a.professional_name::text AS professional_name,
        a.currency::text AS currency, a.status::text AS appointment_status, a.status_source::text AS status_source,
        ''::text AS sale_status, (c.first_name || ' ' || c.last_name)::text AS customer_name,
        c.phone_e164::text AS customer_phone, a.starts_at AS occurred_at,
        1::bigint AS quantity, a.price_cents::bigint AS unit_price_cents,
        a.price_cents::bigint AS value_cents,
        to_char(a.starts_at AT TIME ZONE sqlc.arg(timezone)::text, 'YYYY-MM-DD')::text AS local_date
    FROM public.appointments a
    JOIN public.customers c ON c.shop_id=a.shop_id AND c.id=a.customer_id
    JOIN public.shops sh ON sh.id=a.shop_id AND sh.active
    WHERE a.shop_id=sqlc.arg(shop_id)
      AND sqlc.arg(source)::text IN ('all','services')
      AND a.starts_at>=sqlc.arg(from_at)::timestamptz AND a.starts_at<sqlc.arg(until_at)::timestamptz
      AND (sqlc.arg(professional_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR a.professional_id=sqlc.arg(professional_id))
      AND (sqlc.arg(service_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR a.service_id=sqlc.arg(service_id))
      AND (sqlc.arg(name_pattern)::text='' OR (c.first_name || ' ' || c.last_name) ILIKE sqlc.arg(name_pattern) ESCAPE E'\\')
      AND (sqlc.arg(phone)::text='' OR c.phone_e164=sqlc.arg(phone))
      AND (sqlc.arg(appointment_status)::text='' OR a.status=sqlc.arg(appointment_status))
    UNION ALL
    SELECT s.shop_id, 'products'::text, s.id, i.id,
        '00000000-0000-0000-0000-000000000000'::uuid, i.product_id,
        COALESCE(s.professional_id,'00000000-0000-0000-0000-000000000000'::uuid),
        i.product_name_snapshot::text, s.professional_name_snapshot::text, i.currency::text,
        ''::text, ''::text, s.status::text,
        COALESCE(c.first_name || ' ' || c.last_name,'')::text, COALESCE(c.phone_e164,'')::text,
        s.sold_at, i.quantity::bigint, i.unit_price_cents, i.line_total_cents,
        to_char(s.sold_at AT TIME ZONE sqlc.arg(timezone)::text, 'YYYY-MM-DD')::text
    FROM public.sales s
    JOIN public.sale_items i ON i.shop_id=s.shop_id AND i.sale_id=s.id
    JOIN public.shops sh ON sh.id=s.shop_id AND sh.active
    LEFT JOIN public.appointments a ON a.shop_id=s.shop_id AND a.id=s.appointment_id
    LEFT JOIN public.customers c ON c.shop_id=a.shop_id AND c.id=a.customer_id
    WHERE s.shop_id=sqlc.arg(shop_id)
      AND sqlc.arg(source)::text IN ('all','products')
      AND s.sold_at>=sqlc.arg(from_at)::timestamptz AND s.sold_at<sqlc.arg(until_at)::timestamptz
      AND (sqlc.arg(professional_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR s.professional_id=sqlc.arg(professional_id))
      AND (sqlc.arg(product_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR i.product_id=sqlc.arg(product_id))
      AND (sqlc.arg(name_pattern)::text='' OR (c.first_name || ' ' || c.last_name) ILIKE sqlc.arg(name_pattern) ESCAPE E'\\')
      AND (sqlc.arg(phone)::text='' OR c.phone_e164=sqlc.arg(phone))
      AND (sqlc.arg(sale_status)::text='' OR s.status=sqlc.arg(sale_status))
),
buckets AS (
SELECT (CASE WHEN grouping(local_date)=0 THEN 'day'
    WHEN grouping(professional_id)=0 THEN 'professional'
    WHEN grouping(service_id)=0 THEN 'service'
    WHEN grouping(product_id)=0 THEN 'product' ELSE 'total' END)::text AS bucket,
    (CASE WHEN grouping(local_date)=0 THEN local_date ELSE '' END)::text AS group_key,
    (CASE WHEN grouping(professional_id)=0 THEN professional_id
    WHEN grouping(service_id)=0 THEN service_id WHEN grouping(product_id)=0 THEN product_id
    ELSE '00000000-0000-0000-0000-000000000000'::uuid END)::uuid AS group_id,
    currency::text,
    (CASE WHEN grouping(professional_id)=0 THEN
        (array_agg(professional_name ORDER BY occurred_at DESC,source COLLATE "C" DESC,record_id DESC,item_id DESC))[1]
    WHEN grouping(service_id)=0 OR grouping(product_id)=0 THEN
        (array_agg(label ORDER BY occurred_at DESC,source COLLATE "C" DESC,record_id DESC,item_id DESC))[1]
    ELSE '' END)::text AS label,
    count(*) FILTER (WHERE appointment_status='scheduled')::bigint AS scheduled,
    count(*) FILTER (WHERE appointment_status='confirmed')::bigint AS confirmed,
    count(*) FILTER (WHERE appointment_status='completed')::bigint AS completed,
    count(*) FILTER (WHERE appointment_status='completed' AND status_source='automatic')::bigint AS completed_automatically,
    count(*) FILTER (WHERE appointment_status='cancelled')::bigint AS cancelled,
    count(*) FILTER (WHERE appointment_status='no_show')::bigint AS no_show,
    COALESCE(sum(value_cents) FILTER (WHERE appointment_status IN('scheduled','confirmed')),0)::bigint AS scheduled_value,
    COALESCE(sum(value_cents) FILTER (WHERE appointment_status='completed'),0)::bigint AS service_value,
    COALESCE(sum(value_cents) FILTER (WHERE sale_status='recorded'),0)::bigint AS product_value,
    count(DISTINCT record_id) FILTER (WHERE sale_status='recorded')::bigint AS sale_count,
    COALESCE(sum(quantity) FILTER (WHERE sale_status='recorded'),0)::bigint AS product_units
FROM activity
GROUP BY GROUPING SETS ((currency),(currency,local_date),(currency,professional_id),(currency,service_id),(currency,product_id))
HAVING (grouping(service_id)=1 OR service_id<>'00000000-0000-0000-0000-000000000000'::uuid)
   AND (grouping(product_id)=1 OR product_id<>'00000000-0000-0000-0000-000000000000'::uuid)
)
SELECT * FROM buckets
ORDER BY bucket COLLATE "C",group_key COLLATE "C",group_id,currency COLLATE "C";

-- name: ReadReportSnapshotTime :one
SELECT transaction_timestamp()::timestamptz AS observed_at;

