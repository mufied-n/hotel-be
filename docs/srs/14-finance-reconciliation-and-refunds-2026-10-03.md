# SRS — Rekonsiliasi pembayaran, late payment dan refund

Dokumen teknis: [TECH-F14](../tech/14-finance-reconciliation-and-refunds-architecture-2026-10-03.md).

ID: SRS-F14-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Finance + GM + BE payments. Pasangan: [PRD-F14](../prd/14-finance-reconciliation-and-refunds-2026-10-03.md). Gap: BE-G10–G12, G14, G20.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Audit payment persistence/late-payment gap; finance workflow merupakan proposal. Tidak ada bukti vendor refund behavior.

[F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md); [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md)

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | case open → investigating → action_pending → resolved; refund requested → approved → submitted → succeeded/failed/unknown. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F14-FR-01 — Trace PRD F14-AC-01

Ledger menyimpan attempts/verified events/amount currency exponent/provider IDs; paid/refund states tidak disimpulkan dari FE.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

### F14-FR-02 — Trace PRD F14-AC-02

Reconciliation run matching mengklasifikasi matched/provider_paid_local_pending/amount_mismatch/duplicate/late; input file/source provenance recorded.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

### F14-FR-03 — Trace PRD F14-AC-03

Late paid setelah stock release membentuk case; confirmation baru membutuhkan inventory/policy transaction, atau refund/manual resolution approved.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

### F14-FR-04 — Trace PRD F14-AC-04

Refund memerlukan eligible policy/case, remaining refundable amount, reason, approver sesuai matrix dan expected version.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

### F14-FR-05 — Trace PRD F14-AC-05

Refund intent persisted before external call; idempotency provider key dan lookup ambiguous response mencegah duplicate money movement.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

### F14-FR-06 — Trace PRD F14-AC-06

Sum successful + pending refunds <= captured amount; parallel full/partial refunds serialized. Provider fee/penalty tidak ditebak.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

### F14-FR-07 — Trace PRD F14-AC-07

Case resolution tidak menutupi mismatch tanpa evidence; report reconciliation/audit permissioned dan PII minimal; notification truthfully F11.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F14-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `POST /api/v1/staff/payment-reconciliations` | staff | 202 | Mutation/request durable sesuai FR dan policy |
| `GET /api/v1/staff/payment-cases` | staff | 200 | Read tanpa efek mutation |
| `POST /api/v1/staff/payment-cases/{case_id}/refunds` | staff | 202 | Mutation/request durable sesuai FR dan policy |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F14. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F14-API-1: POST /api/v1/staff/payment-reconciliations

Request `application/json`:

```json
{
  "provider": "provider-example",
  "date_from": "2026-10-01",
  "date_to": "2026-10-03"
}
```

Response HTTP 202:

```json
{
  "run_id": "reconciliation-example",
  "state": "queued"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F14-API-2: GET /api/v1/staff/payment-cases

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response HTTP 200:

```json
{
  "data": [
    {
      "case_id": "case-example",
      "kind": "late_payment",
      "state": "open"
    }
  ],
  "pagination": {
    "next_cursor": null,
    "has_more": false
  }
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F14-API-3: POST /api/v1/staff/payment-cases/{case_id}/refunds

Request `application/json`:

```json
{
  "expected_version": 1,
  "amount": {
    "amount": 1131500,
    "currency": "IDR",
    "exponent": 0
  },
  "reason": "Approved late payment resolution"
}
```

Response HTTP 202:

```json
{
  "refund_id": "refund-example",
  "state": "requested"
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

- F14-VT-1: Paid provider local pending dan wrong currency bukan auto-confirm. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F14-VT-2: Refund timeout replay memanggil intent sama. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F14-VT-3: Parallel partial/full refund tidak over-refund. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F14-VT-4: Late paid booking expired tidak consume stock tanpa policy/inventory recheck. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Refund eligibility/penalty/fees, approval threshold, settlement format/provider, SLA late cases dan refund timing.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.

## Alignment dengan Batch D existing

[SRS Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
