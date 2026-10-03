# SRS — Katalog, stok harian dan kamar maintenance

Dokumen teknis: [TECH-F08](../tech/08-catalog-inventory-and-maintenance-architecture-2026-10-03.md).

ID: SRS-F08-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Revenue/operations + BE catalog/inventory. Pasangan: [PRD-F08](../prd/08-catalog-inventory-and-maintenance-2026-10-03.md). Gap: BE-G01–G03, G17–G20.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Observed: 5 keluarga/7 varian dan official copy 95 kamar, bukan alokasi sellable. Existing katalog CRUD menjadi owner, daftar seed codes tidak diasumsikan cocok dengan vendor.

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md)

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | catalog draft → published/inactive; block planned → active → released; stock ledger berbeda dari room status. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F08-FR-01 — Trace PRD F08-AC-01

Catalog family/variant/bed/media/capacity di-owner review; SKU/inventory mapping immutable untuk historic booking.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

### F08-FR-02 — Trace PRD F08-AC-02

Search memeriksa seluruh nights [check_in,check_out), quantity dan occupancy/children entitlement; missing inventory error terpisah sold-out.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

### F08-FR-03 — Trace PRD F08-AC-03

Inventory records membedakan physical capacity, sellable allocation, holds, confirmed reservations, maintenance dan channel allocation; hindari double subtraction.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

### F08-FR-04 — Trace PRD F08-AC-04

Maintenance block punya room/date/reason/actor/version; konflik booking existing ditampilkan dan butuh resolution, bukan silent removal.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

### F08-FR-05 — Trace PRD F08-AC-05

Publish horizon/tariff coverage tervalidasi; tanggal di luar horizon memiliki guidance. Nonaktif variant tidak menghapus history.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

### F08-FR-06 — Trace PRD F08-AC-06

Hold decrement/release atomic, non-negative; repair inventory lewat adjustment ledger audited dengan expected version.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

### F08-FR-07 — Trace PRD F08-AC-07

Physical room continuous stay dan alternate assignment diuji PostgreSQL nyata; foto memakai asset approved/local dengan alt.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F08-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `GET /api/v1/inventory-availability` | public | 200 | Read tanpa efek mutation |
| `POST /api/v1/staff/maintenance-blocks` | staff | 201 | Mutation/request durable sesuai FR dan policy |
| `POST /api/v1/staff/inventory-adjustments` | staff | 201 | Mutation/request durable sesuai FR dan policy |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F08. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F08-API-1: GET /api/v1/inventory-availability

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response HTTP 200:

```json
{
  "data": [
    {
      "variant_id": "variant-example",
      "available_rooms": 1
    }
  ],
  "pagination": {
    "next_cursor": null,
    "has_more": false
  }
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Endpoint publik tetap bounded/rate-limited dan tidak membuka detail PII.

### F08-API-2: POST /api/v1/staff/maintenance-blocks

Request `application/json`:

```json
{
  "room_id": "physical-room-example",
  "start_date": "2026-10-10",
  "end_date": "2026-10-12",
  "reason": "Perawatan"
}
```

Response HTTP 201:

```json
{
  "block_id": "block-example",
  "state": "planned",
  "version": 1
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F08-API-3: POST /api/v1/staff/inventory-adjustments

Request `application/json`:

```json
{
  "variant_id": "variant-example",
  "date": "2026-10-10",
  "delta": 1,
  "expected_version": 1,
  "reason": "Owner allocation correction"
}
```

Response HTTP 201:

```json
{
  "adjustment_id": "adjustment-example",
  "version": 2
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

- F08-VT-1: Missing satu night tidak menampilkan available dan tidak decrement parsial. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F08-VT-2: N concurrent request last room <= allocation successes. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F08-VT-3: Maintenance overlap confirmed memerlukan resolution. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F08-VT-4: Mapping seed vs 7 official variants direview owner. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Stock per variant, physical mapping, maintenance workflow, child capacities, horizon, readiness authority.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.
