# SRS — Kelengkapan alur booking dan reset sesi

Dokumen teknis: [TECH-F01](../tech/01-booking-journey-completion-architecture-2026-10-03.md).

ID: SRS-F01-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: FE + BE booking; keputusan katalog/rate oleh hotel. Pasangan: [PRD-F01](../prd/01-booking-journey-completion-2026-10-03.md). Gap: BE-G02–G09, G12, G15.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Observed: tanggal, occupancy, room/rate, harga included dan review vendor. Source FE: promo belum mengubah harga, tombol lanjut simulasi belum beraksi, reset draft belum terhubung. Audit backend bukan verifikasi source terbaru.

Tidak ada fitur prerequisite; keputusan identity/hotel dapat membatasi integrasi.

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | draft → quote_selected → reviewed → submitted; new_booking menghapus draft, bukan reservasi server. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F01-FR-01 — Trace PRD F01-AC-01

Search memvalidasi DateOnly, checkout > check-in, panjang stay/horizon dan room/child limits dari konfigurasi; hari checkout tidak menambah malam.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

### F01-FR-02 — Trace PRD F01-AC-02

Setiap room_index memiliki variant/rate/occupancy; semua pilihan lengkap sebelum quote agregat. Rate yang tidak tersedia untuk variant tidak ditampilkan atau diterima.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

### F01-FR-03 — Trace PRD F01-AC-03

Promo diterapkan server quote dengan valid/invalid/expired/ineligible states; total seluruh stay/rooms menjadi harga utama sebelum select.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

### F01-FR-04 — Trace PRD F01-AC-04

Edit tanggal, occupancy, variant, rate, promo/currency membatalkan quote/consent/attempt lama; edit guest tidak mengubah harga. Response async lama tidak mengganti search terbaru.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

### F01-FR-05 — Trace PRD F01-AC-05

Review memakai snapshot harga/policy, guest, special request Unicode <=500 code points; terms/privacy terpisah unchecked dan tersimpan version/timestamp pada submit.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

### F01-FR-06 — Trace PRD F01-AC-06

Create memakai key stabil dan fingerprint payload; retry ambigu mempertahankan attempt. Key sama payload berbeda menghasilkan 409.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

### F01-FR-07 — Trace PRD F01-AC-07

Mulai booking baru membersihkan draft/guest/consent/attempt; histori terotorisasi F03 tetap ada. Refresh tanpa draft menawarkan recovery, bukan PII URL/storage.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F01-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `POST /api/v1/booking-searches` | public | 200 | Mutation/request durable sesuai FR dan policy |
| `POST /api/v1/bookings` | guest | 201 | Mutation/request durable sesuai FR dan policy |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F01. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F01-API-1: POST /api/v1/booking-searches

Request `application/json`:

```json
{
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "rooms": [
    {
      "room_index": 0,
      "adults": 2,
      "children_ages": []
    }
  ],
  "currency": "IDR"
}
```

Response HTTP 200:

```json
{
  "search_id": "search-example",
  "data": [
    {
      "variant_id": "variant-example",
      "available": true
    }
  ],
  "pagination": {
    "next_cursor": null,
    "has_more": false
  }
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Endpoint publik tetap bounded/rate-limited dan tidak membuka detail PII.

### F01-API-2: POST /api/v1/bookings

Request `application/json`:

```json
{
  "quote_id": "quote-example",
  "guest": {
    "name": "Tamu Contoh",
    "email": "guest@example.test"
  },
  "special_request": "",
  "terms_accepted": true,
  "privacy_accepted": true,
  "policy_version": "policy-v1"
}
```

Response HTTP 201:

```json
{
  "booking_id": "booking-example",
  "status": "pending",
  "payment_status": "pending",
  "hold_expires_at": "2026-10-03T03:30:00Z"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

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

- F01-VT-1: Single/multi-room dengan children konsisten dari results sampai status. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F01-VT-2: Promo invalid tidak memberi diskon; suite hanya rate owner yang sah. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F01-VT-3: Back/edit menghapus stale quote; reset menghapus semua PII draft. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F01-VT-4: Double submit/network ambiguity menghasilkan satu booking dan satu hold. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Mapping varian/backend IDs, batas anak/stay, total per item, dan perbedaan Money/quote TTL harus diselaraskan dengan katalog/Batch C.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.

## Alignment dengan Batch D existing

[SRS Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
