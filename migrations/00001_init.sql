-- +goose Up
-- btree_gist: operator class btree (=) untuk text/uuid pada GiST,
-- dibutuhkan oleh EXCLUDE USING GIST di room_assignments (§13).
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE room_types (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    name         TEXT NOT NULL,
    max_capacity INT  NOT NULL CHECK (max_capacity > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===== SCHEMA A: inventory per tipe per malam (kamar fungible) =====
CREATE TABLE inventory (
    room_type_id    UUID NOT NULL REFERENCES room_types(id),
    date            DATE NOT NULL,
    total_rooms     INT  NOT NULL CHECK (total_rooms > 0),
    available_rooms INT  NOT NULL,
    version         INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (room_type_id, date),
    CHECK (available_rooms >= 0 AND available_rooms <= total_rooms)
);

-- Partial index: query availability tersering hanya peduli baris yang masih tersisa.
CREATE INDEX idx_inv_available ON inventory (room_type_id, date)
    WHERE available_rooms > 0;

CREATE TABLE bookings (
    id                UUID PRIMARY KEY DEFAULT uuidv7(),
    room_type_id      UUID NOT NULL REFERENCES room_types(id),
    check_in          DATE NOT NULL,
    check_out         DATE NOT NULL,
    num_rooms         INT  NOT NULL CHECK (num_rooms > 0),
    num_guests        INT  NOT NULL CHECK (num_guests > 0),
    status            TEXT NOT NULL DEFAULT 'pending',
    total_price_minor BIGINT NOT NULL CHECK (total_price_minor >= 0),
    currency          CHAR(3) NOT NULL DEFAULT 'IDR',
    guest_name        TEXT NOT NULL,
    guest_email       TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (check_out > check_in),
    -- State machine terpusat (desain §12.2): transisi legal saja yang boleh tersimpan.
    CHECK (
        status IN ('pending', 'confirmed', 'checked_in', 'checked_out',
                   'cancelled', 'expired', 'failed', 'no_show')
    )
);

CREATE INDEX idx_bookings_room_type_dates ON bookings (room_type_id, check_in, check_out) WHERE status NOT IN ('cancelled', 'expired', 'failed');
CREATE INDEX idx_bookings_status ON bookings (status) WHERE status = 'pending';

CREATE TABLE reservation_room_nights (
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    night_date DATE NOT NULL,
    rate_minor BIGINT NOT NULL CHECK (rate_minor >= 0),
    PRIMARY KEY (booking_id, night_date)
);

CREATE TABLE holds (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_holds_expiry ON holds (expires_at);

-- ===== SCHEMA B: assignment kamar fisik saat check-in (jaminan di level DB) =====
CREATE TABLE rooms (
    room_number  TEXT PRIMARY KEY,
    room_type_id UUID NOT NULL REFERENCES room_types(id)
);

CREATE TABLE room_assignments (
    assignment_id UUID PRIMARY KEY DEFAULT uuidv7(),
    booking_id    UUID NOT NULL REFERENCES bookings(id),
    room_number   TEXT NOT NULL REFERENCES rooms(room_number),
    stay_dates    DATERANGE NOT NULL,
    assigned_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- GiST exclusion constraint: database MENOLAK dua tamu di kamar yang sama
    -- untuk rentang tanggal yang tumpang tindih (desain §13).
    EXCLUDE USING GIST (room_number WITH =, stay_dates WITH &&)
);

-- ===== OUTBOX (desain §5.3) =====
CREATE TABLE outbox (
    id            BIGSERIAL PRIMARY KEY,
    topic         TEXT NOT NULL,
    payload       JSONB NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'failed')),
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbox_pending ON outbox (next_retry_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS room_assignments;
DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS holds;
DROP TABLE IF EXISTS reservation_room_nights;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS inventory;
DROP TABLE IF EXISTS room_types;
