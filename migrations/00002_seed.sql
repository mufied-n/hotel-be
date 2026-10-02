-- +goose Up
INSERT INTO room_types (id, name, max_capacity) VALUES
    ('01900000-0000-7000-8000-000000000001', 'Standard', 2),
    ('01900000-0000-7000-8000-000000000002', 'Superior', 2),
    ('01900000-0000-7000-8000-000000000003', 'Deluxe', 3),
    ('01900000-0000-7000-8000-000000000004', 'Family', 4),
    ('01900000-0000-7000-8000-000000000005', 'Suite', 2);

-- Inventory 365 hari ke depan; weekend (Sabtu/Minggu) dipatok lebih mahal via rates di aplikasi.
INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
SELECT
    rt.id,
    d::date,
    CASE rt.name
        WHEN 'Standard' THEN 30
        WHEN 'Superior' THEN 25
        WHEN 'Deluxe'   THEN 20
        WHEN 'Family'   THEN 15
        WHEN 'Suite'    THEN 10
    END,
    CASE rt.name
        WHEN 'Standard' THEN 30
        WHEN 'Superior' THEN 25
        WHEN 'Deluxe'   THEN 20
        WHEN 'Family'   THEN 15
        WHEN 'Suite'    THEN 10
    END
FROM room_types rt
CROSS JOIN generate_series(
    date_trunc('day', now())::date,
    date_trunc('day', now())::date + 364,
    interval '1 day'
) AS d;

-- Kamar fisik untuk assignment saat check-in (nomor per tipe)
INSERT INTO rooms (room_number, room_type_id)
SELECT
    CASE rt.name
        WHEN 'Standard' THEN '1' || lpad(g::text, 2, '0')
        WHEN 'Superior' THEN '2' || lpad(g::text, 2, '0')
        WHEN 'Deluxe'   THEN '3' || lpad(g::text, 2, '0')
        WHEN 'Family'   THEN '4' || lpad(g::text, 2, '0')
        WHEN 'Suite'    THEN '5' || lpad(g::text, 2, '0')
    END,
    rt.id
FROM room_types rt
CROSS JOIN LATERAL generate_series(1,
    CASE rt.name
        WHEN 'Standard' THEN 30
        WHEN 'Superior' THEN 25
        WHEN 'Deluxe'   THEN 20
        WHEN 'Family'   THEN 15
        WHEN 'Suite'    THEN 10
    END) AS g;

-- +goose Down
DELETE FROM rooms;
DELETE FROM inventory;
DELETE FROM room_types;
