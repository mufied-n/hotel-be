-- +goose Up
-- Menambahkan feature flags untuk kapabilitas perhotelan tingkat tinggi:
-- OTA Channel Sync, Real-Time SSE, Last-Room Safeguards, Dynamic Rates, Official PDF Voucher & Modular WhatsApp.

INSERT INTO feature_flags (key, name, description, enabled, allowed_roles) VALUES
('ff_dynamic_rates_calendar', 'Dynamic Rates & Stop-Sell Calendar', 'Kalender tarif musiman, batas stop-sell, dan alokasi stok kamar harian', TRUE, ARRAY['receptionist', 'revenue_mgr', 'gm_admin']),
('ff_official_pdf_voucher', 'Official PDF Voucher & PBJT Invoice', 'Penerbitan berkas PDF e-voucher resmi A4 ber-QR code dan faktur pajak daerah PBJT 10% Sleman', TRUE, '{}'),
('ff_realtime_event_hub', 'Real-Time Live SSE Stream', 'Streaming live Server-Sent Events (SSE) status pemesanan tamu dan meja depan', TRUE, '{}'),
('ff_channel_sync_integration', 'Multi-Channel OTA Integration', 'Penerimaan webhook masuk kanal OTA dan monitoring isu sinkronisasi', TRUE, '{}'),
('ff_last_room_safeguards', 'Last-Room Safeguards & Upgrade', 'Proteksi kuota kamar terakhir LRDA, dynamic hold 15 menit, dan resolusi upgrade 1-click meja depan', TRUE, ARRAY['receptionist', 'revenue_mgr', 'gm_admin']),
('ff_whatsapp_notifier', 'Modular WhatsApp Dispatcher', 'Pengiriman otomatis notifikasi konfirmasi pemesanan dan tautan e-voucher via WhatsApp', TRUE, '{}')
ON CONFLICT (key) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    allowed_roles = EXCLUDED.allowed_roles;

-- +goose Down
DELETE FROM feature_flags WHERE key IN (
    'ff_dynamic_rates_calendar',
    'ff_official_pdf_voucher',
    'ff_realtime_event_hub',
    'ff_channel_sync_integration',
    'ff_last_room_safeguards',
    'ff_whatsapp_notifier'
);
