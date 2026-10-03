# Registry PRD — roadmap fitur booking PULANG

Pasangan teknis per fitur: [TECH registry](../tech/README.md).

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Paket baru: **15 fitur / 15 pasangan PRD–SRS**, semuanya proposal untuk review. Scope frontend hanya webapp booking; tidak ada landing page/marketing pages.

## Cara membaca

PRD menjelaskan outcome, persona, authority, scope, acceptance dan keputusan bisnis. SRS merinci FR traceable, resource/state, HTTP/JSON, error, consistency dan verification. Dokumen existing berstatus approved tetap pemilik kontrak; paket baru bukan approval baru atau re-audit source backend. Riset vendor berhenti sebelum submit; akses tamu/receipt/operations/refund/pilot adalah proposal.

| ID | Fitur | Prerequisite | Gap trace | Status |
|---|---|---|---|---|
| F01 | [Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-2026-10-03.md) | — | BE-G02–G09, G12, G15 | PROPOSED |
| F02 | [Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-2026-10-03.md) | F12, F11 | BE-G13, G15 | PROPOSED |
| F03 | [Booking Saya dan detail reservasi privat](03-my-bookings-2026-10-03.md) | F02, F01 | BE-G13, G19 | PROPOSED |
| F04 | [Konfirmasi, bukti booking dan kalender](04-booking-confirmation-artifacts-2026-10-03.md) | F03, F05 | BE-G05, G08, G13, G16, G19 | PROPOSED |
| F05 | [Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md) | F01, F02 | BE-G09–G12, G20 | PROPOSED |
| F06 | [Bantuan tamu, perubahan dan pembatalan](06-guest-assistance-and-booking-requests-2026-10-03.md) | F03, F13, F14 | BE-G08, G13–G16, G22 | PROPOSED |
| F07 | [Resepsionis, assignment kamar dan lifecycle menginap](07-front-desk-stay-operations-2026-10-03.md) | F12, F08, F05, F13 | BE-G14, G17, G20, G22 | PROPOSED |
| F08 | [Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-2026-10-03.md) | F12, F13 | BE-G01–G03, G17–G20 | PROPOSED |
| F09 | [Manajemen tarif, paket dan promo](09-rate-plan-and-promo-management-2026-10-03.md) | F08, F12, F13 | BE-G04–G06, G08, G19 | PROPOSED |
| F10 | [Sinkronisasi kanal dan pencegahan overselling](10-channel-inventory-synchronization-2026-10-03.md) | F08, F09, F12 | BE-G18, G20, G21 | PROPOSED |
| F11 | [Notifikasi booking, payment dan operasional](11-notification-delivery-2026-10-03.md) | F12; event payment setelah F05 | BE-G16, G20, G21 | PROPOSED |
| F12 | [Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md) | — | BE-G14, G15, G20 | PROPOSED |
| F13 | [Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md) | F12 | BE-G08, G19, G21, G22 | PROPOSED |
| F14 | [Rekonsiliasi pembayaran, late payment dan refund](14-finance-reconciliation-and-refunds-2026-10-03.md) | F05, F12, F13 | BE-G10–G12, G14, G20 | PROPOSED |
| F15 | [Verifikasi operasional, release gate dan pilot hotel](15-operational-verification-and-hotel-pilot-2026-10-03.md) | F01, F02, F03, F04, F05, F06, F07, F08, F09, F10, F11, F12, F13, F14 | BE-G20, G21; seluruh gate live G01–G22 sesuai scope | PROPOSED |

## Pemilik kontrak yang sudah ada

- [catalog-crud-management](catalog-crud-management-2026-10-03.md) — dipertahankan; status dibaca pada dokumen owner, tidak dianggap bukti deployment.
- [pricing-quote-policies-batch-c](pricing-quote-policies-batch-c-2026-10-03.md) — dipertahankan; status dibaca pada dokumen owner, tidak dianggap bukti deployment.
- [checkout-idempotency-payment-batch-d](checkout-idempotency-payment-batch-d-2026-10-03.md) — owner checkout/idempotency/payment sedang berubah selama penulisan; dipertahankan dan harus diverifikasi source pada implementasi.
- [hotel-rbac-casbin](hotel-rbac-casbin-2026-10-03.md) — dipertahankan; status dibaca pada dokumen owner, tidak dianggap bukti deployment.
- [hotel-booking-parity-and-gap-remediation](hotel-booking-parity-and-gap-remediation-2026-10-03.md) — dipertahankan; status dibaca pada dokumen owner, tidak dianggap bukti deployment.

Referensi pasangan: [SRS registry](../srs/README.md).
[Shared contract dan register konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md); [OpenAPI target proposal](../srs/contracts/booking-roadmap-proposal.openapi.json).

## Urutan delivery

1. Trust boundary F12 dan owner decisions F13; perbaikan fixture F01 dapat berjalan segera.
2. Catalog/inventory F08 dan rate/promo F09 mengikuti existing owner documents; access F02 melalui notification capability F11.
3. Payment/hold F05, Booking Saya F03, artifacts F04 dan bantuan F06.
4. Front desk F07, finance F14, channel F10 dan delivery reliability F11 sesuai scope hotel.
5. Verification F15 mulai sejak setiap fitur (bukan menunggu akhir); go-live hanya setelah gate fitur yang dipakai selesai.

Dependencies menunjukkan kebutuhan untuk live; UI fixture dapat direview sebelum seluruh BE selesai. Bootstrap provider email untuk F02 tidak menunggu semua payment/notification scenarios F11. Tidak ada dependency cycle yang mengharuskan seluruh fitur selesai untuk trial lokal.

## Scope bertahap

Guest trial: perbaikan flow → akses demo → Booking Saya → detail/bukti demo.
Live booking: verified access, quote/policy, inventory, durable payment, provider delivery dan QA gate.
Ops: verified staff, assignments, maintenance, channel authority, reconciliation/refund.
Hotel pilot: allowlist, stock scope, support, rollback/restore, measured acceptance dan go/no-go tercatat.

Aksi fitur belum diimplementasikan oleh dokumentasi ini. Update status implemented/verified hanya dengan walkthrough + source + actual evidence. NOT RUN tidak berubah menjadi PASS karena approval dokumen atau coverage mock.
