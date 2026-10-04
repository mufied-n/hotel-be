# Registry TECH per fitur — booking PULANG

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Status: PROPOSED / REQUIRES ARCHITECTURE REVIEW.
15 dokumen teknis melengkapi 15 pasangan PRD/SRS. Scope UI hanya booking webapp; tidak ada landing/marketing pages. Existing source/backend sedang berubah dan tidak diaudit ulang oleh paket desain ini.

| ID | Tech owner | PRD | SRS | Status |
|---|---|---|---|---|
| F01 | [Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-architecture-2026-10-03.md) | [PRD](../prd/01-booking-journey-completion-2026-10-03.md) | [SRS](../srs/01-booking-journey-completion-2026-10-03.md) | PROPOSED |
| F02 | [Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-architecture-2026-10-03.md) | [PRD](../prd/02-guest-access-and-session-2026-10-03.md) | [SRS](../srs/02-guest-access-and-session-2026-10-03.md) | PROPOSED |
| F03 | [Booking Saya dan detail reservasi privat](03-my-bookings-architecture-2026-10-03.md) | [PRD](../prd/03-my-bookings-2026-10-03.md) | [SRS](../srs/03-my-bookings-2026-10-03.md) | PROPOSED |
| F04 | [Konfirmasi, bukti booking dan kalender](04-booking-confirmation-artifacts-architecture-2026-10-03.md) | [PRD](../prd/04-booking-confirmation-artifacts-2026-10-03.md) | [SRS](../srs/04-booking-confirmation-artifacts-2026-10-03.md) | PROPOSED |
| F05 | [Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-architecture-2026-10-03.md) | [PRD](../prd/05-payment-hold-and-recovery-2026-10-03.md) | [SRS](../srs/05-payment-hold-and-recovery-2026-10-03.md) | PROPOSED |
| F06 | [Bantuan tamu, perubahan dan pembatalan](06-guest-assistance-and-booking-requests-architecture-2026-10-03.md) | [PRD](../prd/06-guest-assistance-and-booking-requests-2026-10-03.md) | [SRS](../srs/06-guest-assistance-and-booking-requests-2026-10-03.md) | PROPOSED |
| F07 | [Resepsionis, assignment kamar dan lifecycle menginap](07-front-desk-stay-operations-architecture-2026-10-03.md) | [PRD](../prd/07-front-desk-stay-operations-2026-10-03.md) | [SRS](../srs/07-front-desk-stay-operations-2026-10-03.md) | PROPOSED |
| F08 | [Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-architecture-2026-10-03.md) | [PRD](../prd/08-catalog-inventory-and-maintenance-2026-10-03.md) | [SRS](../srs/08-catalog-inventory-and-maintenance-2026-10-03.md) | PROPOSED |
| F09 | [Manajemen tarif, paket dan promo](09-rate-plan-and-promo-management-architecture-2026-10-03.md) | [PRD](../prd/09-rate-plan-and-promo-management-2026-10-03.md) | [SRS](../srs/09-rate-plan-and-promo-management-2026-10-03.md) | PROPOSED |
| F10 | [Sinkronisasi kanal dan pencegahan overselling](10-channel-inventory-synchronization-architecture-2026-10-03.md) | [PRD](../prd/10-channel-inventory-synchronization-2026-10-03.md) | [SRS](../srs/10-channel-inventory-synchronization-2026-10-03.md) | PROPOSED |
| F11 | [Notifikasi booking, payment dan operasional](11-notification-delivery-architecture-2026-10-03.md) | [PRD](../prd/11-notification-delivery-2026-10-03.md) | [SRS](../srs/11-notification-delivery-2026-10-03.md) | PROPOSED |
| F12 | [Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-architecture-2026-10-03.md) | [PRD](../prd/12-staff-identity-permissions-and-audit-2026-10-03.md) | [SRS](../srs/12-staff-identity-permissions-and-audit-2026-10-03.md) | PROPOSED |
| F13 | [Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-architecture-2026-10-03.md) | [PRD](../prd/13-hotel-policy-and-configuration-2026-10-03.md) | [SRS](../srs/13-hotel-policy-and-configuration-2026-10-03.md) | PROPOSED |
| F14 | [Rekonsiliasi pembayaran, late payment dan refund](14-finance-reconciliation-and-refunds-architecture-2026-10-03.md) | [PRD](../prd/14-finance-reconciliation-and-refunds-2026-10-03.md) | [SRS](../srs/14-finance-reconciliation-and-refunds-2026-10-03.md) | PROPOSED |
| F15 | [Verifikasi operasional, release gate dan pilot hotel](15-operational-verification-and-hotel-pilot-architecture-2026-10-03.md) | [PRD](../prd/15-operational-verification-and-hotel-pilot-2026-10-03.md) | [SRS](../srs/15-operational-verification-and-hotel-pilot-2026-10-03.md) | PROPOSED |

## Authority dan perubahan

- [Katalog owner](catalog-crud-architecture-2026-10-03.md).
- [Pricing/quote/policy owner](pricing-quote-policies-architecture-2026-10-03.md).
- [Checkout/payment Batch D owner](checkout-idempotency-payment-architecture-2026-10-03.md).
- [Casbin/RBAC owner](hotel-rbac-casbin-architecture-2026-10-03.md).
- [Migration runner owner](database-migrations-and-runner-2026-10-03.md).
- [Router Refactor & Transport Modularization](router-refactor-architecture-2026-10-04.md).
- [Dynamic Rates, Room Allotment & Stop-Sell Architecture](dynamic-rates-and-stop-sell-architecture-2026-10-04.md).
- [Official PDF Confirmation Voucher & PBJT Tax Invoice Architecture](official-pdf-voucher-and-tax-invoice-architecture-2026-10-04.md).
- [Real-Time Hospitality Event Hub: NATS JetStream, Live Front Desk SSE, Multi-Channel Webhooks & Notifier Architecture](realtime-hospitality-event-hub-and-channel-sync-architecture-2026-10-04.md).
- [Last-Room Hospitality Safeguards: LRDA Safety Buffer, Dynamic Hold & 1-Click Upgrade Architecture](last-room-safeguards-and-complimentary-upgrade-architecture-2026-10-04.md).
- [Modular WhatsApp Notifier Architecture & Multi-Provider Engine](modular-whatsapp-notifier-provider-architecture-2026-10-04.md).
- [Hospitality & Multi-Channel Feature Flags System Architecture](hospitality-and-channels-feature-flags-architecture-2026-10-04.md).
- [Register konflik C01–C14](../srs/00-shared-contracts-and-decisions-2026-10-03.md).
- [PRD registry](../prd/README.md) dan [SRS registry](../srs/README.md).

Existing dokumen dipertahankan; approval existing tidak diubah. ADR pada setiap TECH baru masih Proposed. Provider/session/schema extensions bukan implementasi existing. Review sebelum accepted, jangan mengganti domain enum/currency/TTL/migration numbering tanpa owner amendment.

## Urutan dan ownership

Trust staff F12 + policy F13 → stock/catalog F08 + rate F09; flow fixture F01 dapat berjalan lebih awal.
Bootstrap delivery F11 → guest access F02 → Booking Saya F03; payment F05 → confirmation F04 / finance F14 → assistance F06; front desk F07 dan channel F10 sesuai gate dependensi.
Verification F15 dilakukan per change sejak awal; pilot live setelah seluruh gate scope yang dipakai lulus. Notification confirmed/payment templates diaktifkan setelah F05 verified, tanpa siklus bootstrap login.

Setiap dokumen mencakup diagram, logical schema/constraints, transaction/recovery, ports/adapters, auth/privacy, ADR/trade-offs, metrics/benchmark, migration/rollback, traceability 7 FR serta verification.
Tidak ada DDL/backend code/deployment yang dijalankan pada pekerjaan dokumentasi. [Validation report](feature-tech-validation-2026-10-03.md) mencatat pemeriksaan struktur/tautan; bukan bukti implementasi.

