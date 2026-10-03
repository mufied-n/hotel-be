# SRS — Hosted payment, hold dan pemulihan pembayaran

Dokumen teknis: [TECH-F05](../tech/05-payment-hold-and-recovery-architecture-2026-10-03.md).

ID: SRS-F05-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE payments + provider integration + FE. Pasangan: [PRD-F05](../prd/05-payment-hold-and-recovery-2026-10-03.md). Gap: BE-G09–G12, G20.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Observed: pay-now copy. Payment vendor tidak diobservasi. Audit mencatat fake gateway dan deadline/recovery gap.

[F01 Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-2026-10-03.md); [F02 Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-2026-10-03.md)

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | payment pending → processing → paid/failed; hold active → consumed/expired; ambiguous → needs_assistance view. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F05-FR-01 — Trace PRD F05-AC-01

Create booking idempoten mencatat quote/hold/outbox secara atomic; call provider di luar transaksi DB dengan durable recovery intent.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

### F05-FR-02 — Trace PRD F05-AC-02

Payment attempt memiliki ID/provider reference/amount/currency/exponent/status/deadline; attempt baru hanya setelah status lama terminal dan policy mengizinkan.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

### F05-FR-03 — Trace PRD F05-AC-03

Retry provider timeout mencari attempt existing; jangan menganggap timeout sebagai gagal atau charge ulang.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

### F05-FR-04 — Trace PRD F05-AC-04

Return URL hanya meminta check status; query success tidak mengubah domain. Webhook memverifikasi signature/timestamp/replay memakai kontrak provider.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

### F05-FR-05 — Trace PRD F05-AC-05

Paid confirmation mencocokkan amount/currency/booking/provider reference; duplicate/out-of-order event aman. Payment dan hold expiry diserialisasi.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

### F05-FR-06 — Trace PRD F05-AC-06

Server time/expiry menjadi authority; timer FE menggunakan elapsed nyata, visibility refresh, cleanup, bounded poll/backoff dan aksi retry status.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

### F05-FR-07 — Trace PRD F05-AC-07

Late payment dialihkan assistance/reconciliation sesuai policy; tidak otomatis acquire stok yang sudah dilepas. Fake-pay dinonaktifkan di production.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F05-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `POST /api/v1/guest/bookings/{booking_id}/payment-attempts` | guest | 201 | Mutation/request durable sesuai FR dan policy |
| `GET /api/v1/guest/bookings/{booking_id}/payment-status` | guest | 200 | Read tanpa efek mutation |
| `POST /api/v1/payment-events` | provider | 202 | Mutation/request durable sesuai FR dan policy |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F05. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F05-API-1: POST /api/v1/guest/bookings/{booking_id}/payment-attempts

Request `application/json`:

```json
{
  "return_path": "/guest/bookings/booking-example"
}
```

Response HTTP 201:

```json
{
  "attempt_id": "attempt-example",
  "status": "pending",
  "hosted_url": "https://payments.example.test/attempt-example",
  "expires_at": "2026-10-03T03:30:00Z"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F05-API-2: GET /api/v1/guest/bookings/{booking_id}/payment-status

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response HTTP 200:

```json
{
  "booking_status": "pending",
  "payment_status": "processing",
  "server_time": "2026-10-03T03:05:00Z",
  "hold_expires_at": "2026-10-03T03:30:00Z"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F05-API-3: POST /api/v1/payment-events

Request `application/json`:

```json
{
  "event_id": "provider-event-example",
  "provider_reference": "provider-reference-example",
  "state": "paid",
  "amount": 1131500,
  "currency": "IDR"
}
```

Response HTTP 202:

```json
{
  "event_id": "provider-event-example",
  "state": "accepted"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Payload adalah normalized adapter proposal; provider-specific signing/raw payload belum final dan wajib dicocokkan dokumentasi provider terpilih.

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

Record durable memuat ID, property/owner bila relevan, state, version, created_at/updated_at, actor/reason untuk mutation staff, dan snapshot/references domain. Unique constraint idempotency/event key ditetapkan per namespace; transactional outbox untuk external work. Retention dan erase/anonymize policy disetujui owner.
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

- F05-VT-1: Duplicate submit/webhook hanya satu charge intent/booking confirmation. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F05-VT-2: Provider timeout dan return tampered tidak auto-confirm. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F05-VT-3: Expiry versus paid race membuktikan stok dan status final benar. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F05-VT-4: Offline/unmount/hidden tab tidak menyebabkan poll leak atau charge baru. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Provider, signing protocol, hold TTL, attempt retry window, late payment dan refund handling.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.

## Alignment dengan Batch D existing

[SRS Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
