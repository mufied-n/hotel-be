-- +goose Up
-- Tambahkan feature flags untuk modul Operasional Front Desk (Proposed 02) dan Stay Modification (Proposed 03)
INSERT INTO feature_flags (key, name, description, enabled, allowed_roles) VALUES
('ff_front_desk_operations', 'Front Desk Operations & Handover', 'Papan operasional harian dan catatan serah terima shift front desk', TRUE, '{}'),
('ff_stay_modification', 'Stay Modification & Room Move', 'Fitur pemindahan kamar mid-stay dan perpanjangan durasi menginap', TRUE, '{}')
ON CONFLICT (key) DO UPDATE SET allowed_roles = EXCLUDED.allowed_roles;

-- +goose Down
DELETE FROM feature_flags WHERE key IN ('ff_front_desk_operations', 'ff_stay_modification');
