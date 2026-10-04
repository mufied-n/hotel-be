# SRS: Hospitality & Multi-Channel Feature Flags System
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Fitur ID:** `FEAT-HOSPITALITY-FEATURE-FLAGS`
- **Tanggal:** 4 Oktober 2026
- **Status:** APPROVED / IN IMPLEMENTATION

---

## 1. Kebutuhan Fungsional (Functional Requirements)

* **FR-FF-05 (Seed Migration for Hospitality & Channel Flags):**
  Sistem wajib menyediakan migrasi Goose SQL (`00023_hospitality_and_channels_feature_flags.sql`) yang menyematkan baris konfigurasi flag baru pada tabel `feature_flags`.
* **FR-FF-06 (Route Perimeter Feature Gating):**
  Seluruh endpoint HTTP fitur baru wajib dilapisi middleware `RequireFeature(d.FeatureFlag, key)`:
  - `POST /api/v1/channel-events` $\rightarrow$ `ff("ff_channel_sync_integration")`
  - `GET /api/v1/guest/bookings/:id/live-status` $\rightarrow$ `ff("ff_realtime_event_hub")`
  - `GET /api/v1/front-desk/live-stream` $\rightarrow$ `ff("ff_realtime_event_hub")`
  - `GET /api/v1/revenue/calendar` $\rightarrow$ `ff("ff_dynamic_rates_calendar")`
  - `PUT /api/v1/revenue/calendar/bulk` $\rightarrow$ `ff("ff_dynamic_rates_calendar")`
  - `GET /api/v1/revenue/promos` $\rightarrow$ `ff("ff_promotions_engine")`
  - `POST /api/v1/revenue/promos` $\rightarrow$ `ff("ff_promotions_engine")`
  - `PUT /api/v1/revenue/promos/:id` $\rightarrow$ `ff("ff_promotions_engine")`
  - `GET /api/v1/staff/channel-sync-issues` $\rightarrow$ `ff("ff_channel_sync_integration")`
  - `POST /api/v1/staff/channel-sync-issues/:id/resolve` $\rightarrow$ `ff("ff_last_room_safeguards")`
  - `GET /api/v1/staff/channel-partners/:code` $\rightarrow$ `ff("ff_channel_sync_integration")`
  - `GET /api/v1/bookings/:id/voucher.pdf` $\rightarrow$ `ff("ff_official_pdf_voucher")`
  - `GET /api/v1/bookings/:id/invoice.pdf` $\rightarrow$ `ff("ff_official_pdf_voucher")`
  - `GET /api/v1/guest/bookings/:id/voucher.pdf` $\rightarrow$ `ff("ff_official_pdf_voucher")`
  - `GET /api/v1/guest/bookings/:id/invoice.pdf` $\rightarrow$ `ff("ff_official_pdf_voucher")`
* **FR-FF-07 (HTTP 503 Feature Disabled Response Contract):**
  Jika endpoint diakses saat flag bernilai `false`, sistem merespons dengan format RFC 7807 Problem Details:
  ```json
  {
    "type": "about:blank",
    "title": "Service Unavailable",
    "status": 503,
    "detail": "Fitur '<flag_key>' sedang dinonaktifkan sementara.",
    "code": "FEATURE_DISABLED"
  }
  ```
* **FR-FF-08 (Live Administrative Toggling):**
  Staf dengan peran `gm_admin` dapat mematikan dan menyalakan flag sewaktu-waktu melalui:
  - `PUT /api/v1/admin/feature-flags/:key`
  - Payload: `{"enabled": false}` atau `{"enabled": true}`.
  - Respons: HTTP 200 OK beserta data flag yang telah diperbarui.
