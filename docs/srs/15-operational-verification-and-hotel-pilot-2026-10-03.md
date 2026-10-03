# SRS — Verifikasi operasional, release gate dan pilot hotel

Dokumen teknis: [TECH-F15](../tech/15-operational-verification-and-hotel-pilot-architecture-2026-10-03.md).

ID: SRS-F15-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: QA + ops + hotel pilot owner + BE/FE. Pasangan: [PRD-F15](../prd/15-operational-verification-and-hotel-pilot-2026-10-03.md). Gap: BE-G20, G21; seluruh gate live G01–G22 sesuai scope.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Audit G20: mocked coverage/build bukan bukti DB/provider/delivery. QA FE partial Chromium dan device NOT RUN; pilot belum dilaksanakan.

[F01 Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-2026-10-03.md); [F02 Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-2026-10-03.md); [F03 Booking Saya dan detail reservasi privat](03-my-bookings-2026-10-03.md); [F04 Konfirmasi, bukti booking dan kalender](04-booking-confirmation-artifacts-2026-10-03.md); [F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md); [F06 Bantuan tamu, perubahan dan pembatalan](06-guest-assistance-and-booking-requests-2026-10-03.md); [F07 Resepsionis, assignment kamar dan lifecycle menginap](07-front-desk-stay-operations-2026-10-03.md); [F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-2026-10-03.md); [F09 Manajemen tarif, paket dan promo](09-rate-plan-and-promo-management-2026-10-03.md); [F10 Sinkronisasi kanal dan pencegahan overselling](10-channel-inventory-synchronization-2026-10-03.md); [F11 Notifikasi booking, payment dan operasional](11-notification-delivery-2026-10-03.md); [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md); [F14 Rekonsiliasi pembayaran, late payment dan refund](14-finance-reconciliation-and-refunds-2026-10-03.md)

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | gate planned → executed → passed/failed/not_run; pilot proposed → approved → running → evaluated → go/no_go. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F15-FR-01 — Trace PRD F15-AC-01

Matrix requirement/gap → named scenario → commands/environment/source hash → DB/provider assertions → result/cleanup; NOT RUN tidak pass.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

### F15-FR-02 — Trace PRD F15-AC-02

New/refined Go packages mencapai target table-driven coverage >=80% menurut AGENTS dan vet/race; real PG/Valkey tests tetap wajib terpisah.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

### F15-FR-03 — Trace PRD F15-AC-03

Concurrency last-room, missing-night rollback, assignment conflicts, expiry/paid race, duplicate event dan queue down/sweep diuji DB isolated.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

### F15-FR-04 — Trace PRD F15-AC-04

Provider payment/email/channel sandbox membuktikan idempotency/signatures/replay/outage; live credentials bukan test fixtures.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

### F15-FR-05 — Trace PRD F15-AC-05

Observe queue/outbox lag, payment unknown, stock drift, auth rejects, sweep health dan readiness; threshold/SLO ditetapkan setelah measurement.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

### F15-FR-06 — Trace PRD F15-AC-06

Backup/restore drill dan rollback deploy/migration/payment intent menyimpan evidence; rollback tidak menghapus successful charges/history.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

### F15-FR-07 — Trace PRD F15-AC-07

Pilot allowlist/scope/stock/day/support owner ditetapkan; actual Chrome Android/Safari iOS keyboard/accessibility QA; go/no-go hotel tercatat dengan outstanding risks.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F15-SC terkait di PRD.

## HTTP proposal dan akses

Fitur release/pilot tidak membutuhkan endpoint bisnis baru. Menggunakan kontrak F01–F14, existing health/readiness internal dan evidence tools. Jangan membuat public endpoint untuk backup/restore, fault injection, atau go-live otomatis.





## Validasi, error dan failure semantics

| Kondisi | HTTP / kode | Guidance / efek |
|---|---|---|
| Malformed/unknown field/range | 400 VALIDATION_ERROR | Field errors tanpa echo secret; tidak mutate |
| Sesi tidak valid/expired | 401 AUTHENTICATION_REQUIRED | Verifikasi ulang; hapus state privat |
| Role/action tidak diizinkan | 403 FORBIDDEN | Tidak retry mutation dengan role buatan |
| Resource asing/hilang | 404 RESOURCE_NOT_FOUND | Body generik, tidak mengungkap owner |
| Key sama payload berbeda/state conflict | 409 IDEMPOTENCY_CONFLICT / STATE_CONFLICT | Read latest/review; tidak efek kedua |
| Quote expired bila terkait booking | 410 QUOTE_EXPIRED | Quote/review ulang; mengikuti owner Batch C |
| Stale expected_version / If-Match | 412 VERSION_CONFLICT | Reload dan review ulang |
| Body terlalu besar | 413 PAYLOAD_TOO_LARGE | Kurangi payload |
| Limit request | 429 RATE_LIMITED | Retry-After; tidak restart financial intent |
| Dependency unavailable | 503 SERVICE_UNAVAILABLE | Status unknown, retry bounded/read authoritative |

Respons `application/problem+json` menggunakan schema bersama. Error khusus `CONSENT_REQUIRED`, `INVALID_PROMO_CODE`, `NON_REFUNDABLE_BOOKING`, `SOLD_OUT` dan payment outcomes mengikuti owner dokumen domain; tidak mengganti kode existing tanpa adapter migration. HTTP 500 tanpa stack/schema/provider secret, request_id disimpan untuk tracing.
Private mutations tidak mengizinkan client menentukan actor_id/role/server timestamp. GET tidak membuat charge, refund, hold atau memutasi state domain.

## Data, persistence dan konsistensi

Evidence durable memiliki source/version/environment, scenario, expected/actual, DB/provider assertions, cleanup dan reviewer; real secrets/PII tidak dipublikasikan.
Financial intent/stock effects tidak dihapus ketika UI reset atau user logout. Cache/derived view tidak menggantikan ledger/domain authority. Untuk read pagination, filter ownership/property sebelum limit; bound horizon/search diputuskan hotel.

## Integrasi, NFR dan compatibility

- API `/api/v1` proposal, schema versioned; perubahan required field/status/envelope mengamend owner atau versi baru dengan migration/deprecation plan.
- Sesi guest/staff terpisah; provider events terverifikasi. BFF same-origin adalah proposal, bukan keputusan existing.
- Money integer, currency/exponent explicit; server authority harga/deadline; adapter FE exponent2 demo tidak menentukan kontrak BE exponent0.
- UTC timestamps dengan offset/Z; kalender/hotel policy Asia/Jakarta. Jangan mengurangi dua browser-local midnight timestamps untuk nights.
- Body/query limits, scoped rate limit dan retries bounded configurable; response privat no-store. Tidak menyimpan guest PII di query/localStorage/analytics.
- Observable: request_id/event IDs, operation outcome, latency/failure/retry/backlog; redaction dan audit berbeda dari debug log.
- Performance/SLO angka belum approved. Capture workload dan ukur percentile/resource sebelum gate numerik.

## Verification matrix

- F15-VT-1: Backup restore drill proves balances/reservations/outbox continuity. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F15-VT-2: No overselling N clients and cleanup ledger proof. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F15-VT-3: Real devices/session recovery/staff operations walkthrough. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F15-VT-4: Go/no-go rejects absent provider evidence and rollback ownership. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Pilot dates/rooms/channel scope, numeric SLO/RPO/RTO/thresholds, support rota, deployment topology dan acceptable deferred risks.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.
