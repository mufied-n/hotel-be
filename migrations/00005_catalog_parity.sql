-- +goose Up
-- Paritas Katalog Hotel Pulang ke Uttara: 5 Room Families, 7 Sellable Variants, 95 Kamar Fisik (BE-G01)

-- 1. Tambah metadata detail pada tabel room_types
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS code VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS family_name TEXT NOT NULL DEFAULT '';
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS bed_type TEXT NOT NULL DEFAULT '';
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS room_size_sqm INT NOT NULL DEFAULT 0;
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS max_adults INT NOT NULL DEFAULT 2;
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS max_children INT NOT NULL DEFAULT 1;
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS base_price_minor BIGINT NOT NULL DEFAULT 550000;
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS photos JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE room_types ADD COLUMN IF NOT EXISTS amenities JSONB NOT NULL DEFAULT '[]'::jsonb;

-- 2. Update 5 varian kamar awal dan insert 2 varian suite
INSERT INTO room_types (id, code, name, family_name, bed_type, room_size_sqm, max_capacity, max_adults, max_children, base_price_minor, description, amenities, photos)
VALUES
    ('01900000-0000-7000-8000-000000000001', 'sup-king', 'Superior King Room', 'Superior', '1 King Bed', 28, 3, 2, 1, 550000,
     'Kamar Superior modern dengan 1 King Bed berkualitas tinggi dan pemandangan kota Yogyakarta.',
     '["Free High-Speed Wi-Fi", "Air Conditioning", "43-inch Smart TV", "Coffee/Tea Maker", "Safe Deposit Box", "Rain Shower", "Hair Dryer"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/superior-king-1.jpg", "alt": "Superior King Bedroom"}]'::jsonb),
    ('01900000-0000-7000-8000-000000000002', 'sup-twin', 'Superior Twin Room', 'Superior', '2 Twin Beds', 28, 3, 2, 1, 550000,
     'Kamar Superior nyaman dengan 2 Twin Beds ideal untuk perjalanan bisnis atau liburan bersama teman.',
     '["Free High-Speed Wi-Fi", "Air Conditioning", "43-inch Smart TV", "Coffee/Tea Maker", "Safe Deposit Box", "Rain Shower", "Hair Dryer"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/superior-twin-1.jpg", "alt": "Superior Twin Bedroom"}]'::jsonb),
    ('01900000-0000-7000-8000-000000000003', 'dlx-king', 'Deluxe King Room', 'Deluxe', '1 King Bed', 34, 3, 2, 1, 750000,
     'Kamar Deluxe elegan berukuran 34 sqm dengan area duduk santai dan balkon pemandangan Gunung Merapi.',
     '["Free High-Speed Wi-Fi", "Balcony with View", "Air Conditioning", "50-inch Smart TV", "Coffee Machine", "Minibar", "Bathrobe & Slippers", "Rain Shower"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/deluxe-king-1.jpg", "alt": "Deluxe King Bedroom"}]'::jsonb),
    ('01900000-0000-7000-8000-000000000004', 'dlx-twin', 'Deluxe Twin Room', 'Deluxe', '2 Twin Beds', 34, 3, 2, 1, 750000,
     'Kamar Deluxe lapang dengan 2 Twin Beds dan fasilitas premium untuk kenyamanan maksimal.',
     '["Free High-Speed Wi-Fi", "Balcony with View", "Air Conditioning", "50-inch Smart TV", "Coffee Machine", "Minibar", "Bathrobe & Slippers", "Rain Shower"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/deluxe-twin-1.jpg", "alt": "Deluxe Twin Bedroom"}]'::jsonb),
    ('01900000-0000-7000-8000-000000000005', 'exc-king', 'Executive King Room', 'Executive', '1 King Bed', 42, 3, 2, 1, 1100000,
     'Kamar Executive luas berukuran 42 sqm dengan ruang kerja ergonomis dan akses eksklusif lounge.',
     '["Free High-Speed Wi-Fi", "Executive Desk", "Espresso Machine", "Bathtub & Rain Shower", "55-inch Smart TV", "Lounge Access", "Premium Toiletries"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/executive-king-1.jpg", "alt": "Executive King Bedroom"}]'::jsonb),
    ('01900000-0000-7000-8000-000000000006', 'jste-suite', 'Junior Suite', 'Junior Suite', '1 Super King Bed', 56, 4, 2, 2, 1650000,
     'Suite mewah dengan ruang tamu terpisah, walk-in closet, dan kamar mandi marmer dengan bathtub.',
     '["Separate Living Area", "Walk-in Closet", "Marble Bathroom with Bathtub", "65-inch OLED TV", "Espresso Machine", "Minibar Included", "Butler Service on Demand"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/junior-suite-1.jpg", "alt": "Junior Suite Living & Bedroom"}]'::jsonb),
    ('01900000-0000-7000-8000-000000000007', 'pste-suite', 'Presidential Suite', 'Presidential Suite', '2 King Beds', 110, 6, 4, 2, 3500000,
     'Puncak kemewahan Pulang ke Uttara dengan 2 master bedroom, dining room, kitchenette, dan panorama 360 derajat kota Jogja.',
     '["2 Master Bedrooms", "Private Dining Room", "Kitchenette", "Jacuzzi with Skyline View", "75-inch Home Theater", "Dedicated Butler", "Private Check-in"]'::jsonb,
     '[{"url": "https://pulangkeuttara.com/images/rooms/presidential-suite-1.jpg", "alt": "Presidential Suite Panorama"}]'::jsonb)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    family_name = EXCLUDED.family_name,
    bed_type = EXCLUDED.bed_type,
    room_size_sqm = EXCLUDED.room_size_sqm,
    max_capacity = EXCLUDED.max_capacity,
    max_adults = EXCLUDED.max_adults,
    max_children = EXCLUDED.max_children,
    base_price_minor = EXCLUDED.base_price_minor,
    description = EXCLUDED.description,
    amenities = EXCLUDED.amenities,
    photos = EXCLUDED.photos;

CREATE UNIQUE INDEX IF NOT EXISTS idx_room_types_code ON room_types (code);

-- 3. Rekonstruksi 95 Kamar Fisik Sesuai Inventaris Resmi Pulang ke Uttara
DELETE FROM rooms;

-- Superior King: 25 unit (201 - 225)
INSERT INTO rooms (room_number, room_type_id)
SELECT '2' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000001'
FROM generate_series(1, 25) AS g;

-- Superior Twin: 20 unit (226 - 245)
INSERT INTO rooms (room_number, room_type_id)
SELECT '2' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000002'
FROM generate_series(26, 45) AS g;

-- Deluxe King: 20 unit (301 - 320)
INSERT INTO rooms (room_number, room_type_id)
SELECT '3' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000003'
FROM generate_series(1, 20) AS g;

-- Deluxe Twin: 15 unit (321 - 335)
INSERT INTO rooms (room_number, room_type_id)
SELECT '3' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000004'
FROM generate_series(21, 35) AS g;

-- Executive King: 10 unit (401 - 410)
INSERT INTO rooms (room_number, room_type_id)
SELECT '4' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000005'
FROM generate_series(1, 10) AS g;

-- Junior Suite: 3 unit (501 - 503)
INSERT INTO rooms (room_number, room_type_id)
SELECT '5' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000006'
FROM generate_series(1, 3) AS g;

-- Presidential Suite: 2 unit (504 - 505)
INSERT INTO rooms (room_number, room_type_id)
SELECT '5' || lpad(g::text, 2, '0'), '01900000-0000-7000-8000-000000000007'
FROM generate_series(4, 5) AS g;

-- 4. Inisialisasi Ketersediaan Kamar 365 Hari ke Depan untuk 7 Varian (BE-G18)
INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms)
SELECT
    rt.id,
    d::date,
    CASE rt.code
        WHEN 'sup-king'   THEN 25
        WHEN 'sup-twin'   THEN 20
        WHEN 'dlx-king'   THEN 20
        WHEN 'dlx-twin'   THEN 15
        WHEN 'exc-king'   THEN 10
        WHEN 'jste-suite' THEN 3
        WHEN 'pste-suite' THEN 2
        ELSE 10
    END,
    CASE rt.code
        WHEN 'sup-king'   THEN 25
        WHEN 'sup-twin'   THEN 20
        WHEN 'dlx-king'   THEN 20
        WHEN 'dlx-twin'   THEN 15
        WHEN 'exc-king'   THEN 10
        WHEN 'jste-suite' THEN 3
        WHEN 'pste-suite' THEN 2
        ELSE 10
    END
FROM room_types rt
CROSS JOIN generate_series(
    date_trunc('day', now())::date,
    date_trunc('day', now())::date + 364,
    interval '1 day'
) AS d
ON CONFLICT (room_type_id, date) DO UPDATE SET
    total_rooms = EXCLUDED.total_rooms,
    available_rooms = LEAST(inventory.available_rooms, EXCLUDED.total_rooms);

-- 5. Kebijakan Akses Casbin untuk Katalog & Pencarian Publik serta Manajemen CRUD
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES
    ('p', 'guest', '/api/v1/catalog/rooms', 'GET'),
    ('p', 'guest', '/api/v1/catalog/rooms/:id', 'GET'),
    ('p', 'guest', '/api/v1/search', 'GET'),
    ('p', 'revenue_mgr', '/api/v1/catalog/rooms', 'POST'),
    ('p', 'revenue_mgr', '/api/v1/catalog/rooms/:id', 'PUT'),
    ('g', 'revenue_mgr', 'guest', '')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

-- +goose Down
DELETE FROM casbin_rule WHERE (ptype = 'p' AND v1 IN ('/api/v1/catalog/rooms', '/api/v1/catalog/rooms/:id', '/api/v1/search')) OR (ptype = 'g' AND v0 = 'revenue_mgr' AND v1 = 'guest');
DELETE FROM inventory WHERE room_type_id IN ('01900000-0000-7000-8000-000000000006', '01900000-0000-7000-8000-000000000007');
DELETE FROM rooms WHERE room_type_id IN ('01900000-0000-7000-8000-000000000006', '01900000-0000-7000-8000-000000000007');
DELETE FROM room_types WHERE id IN ('01900000-0000-7000-8000-000000000006', '01900000-0000-7000-8000-000000000007');
DROP INDEX IF EXISTS idx_room_types_code;
ALTER TABLE room_types DROP COLUMN IF EXISTS amenities;
ALTER TABLE room_types DROP COLUMN IF EXISTS photos;
ALTER TABLE room_types DROP COLUMN IF EXISTS base_price_minor;
ALTER TABLE room_types DROP COLUMN IF EXISTS description;
ALTER TABLE room_types DROP COLUMN IF EXISTS max_children;
ALTER TABLE room_types DROP COLUMN IF EXISTS max_adults;
ALTER TABLE room_types DROP COLUMN IF EXISTS room_size_sqm;
ALTER TABLE room_types DROP COLUMN IF EXISTS bed_type;
ALTER TABLE room_types DROP COLUMN IF EXISTS family_name;
ALTER TABLE room_types DROP COLUMN IF EXISTS code;
