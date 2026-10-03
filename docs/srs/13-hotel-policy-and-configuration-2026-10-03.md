# SRS — Konfigurasi hotel dan publikasi kebijakan

Dokumen teknis: [TECH-F13](../tech/13-hotel-policy-and-configuration-architecture-2026-10-03.md).

ID: SRS-F13-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Hotel GM/ops + BE config. Pasangan: [PRD-F13](../prd/13-hotel-policy-and-configuration-2026-10-03.md). Gap: BE-G08, G19, G21, G22.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Official observed check-in15/check-out12 dan policy snapshot; Batch C memiliki cutoff fleksibel14WIB. Aturan final harus owner-reviewed, tanpa legal tax inference.

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md)

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | draft → reviewed → scheduled → active → superseded; historical snapshot retained. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F13-FR-01 — Trace PRD F13-AC-01

Metadata address/contact/check-in/out/timezone dari owner; proposal contact tidak dipublikasikan sebagai verified tanpa provenance.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

### F13-FR-02 — Trace PRD F13-AC-02

Policy object menyimpan terms/privacy/cancellation/no-show/early-checkout/rate association dan effective window/version.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

### F13-FR-03 — Trace PRD F13-AC-03

Draft publish membutuhkan approval authority, expected version dan audit; scheduled overlap/invalid timezone/limits ditolak.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

### F13-FR-04 — Trace PRD F13-AC-04

Quote dan booking menyimpan policy/version immutable; enforcement existing booking pakai versi saat consent, bukan current config.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

### F13-FR-05 — Trace PRD F13-AC-05

Runtime limits/body/search/hold/quote TTL memiliki validation dan owner; quote TTL tidak sama dengan hold/payment deadline.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

### F13-FR-06 — Trace PRD F13-AC-06

Cancel/refund exceptions mencatat reason/approver dan permission spesifik; emergency override tidak membuka route publik.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

### F13-FR-07 — Trace PRD F13-AC-07

Roll back publication lewat versi baru effective-forward; source flags fake gateway/dev scenario tidak boleh enabled di production.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F13-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `GET /api/v1/hotel` | public | 200 | Read tanpa efek mutation |
| `POST /api/v1/staff/policy-versions` | staff | 201 | Mutation/request durable sesuai FR dan policy |
| `POST /api/v1/staff/policy-versions/{policy_version_id}/publications` | staff | 201 | Mutation/request durable sesuai FR dan policy |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F13. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F13-API-1: GET /api/v1/hotel

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response HTTP 200:

```json
{
  "name": "PULANG ke UTTARA",
  "timezone": "Asia/Jakarta",
  "check_in_time": "15:00",
  "check_out_time": "12:00",
  "policy_version": "policy-v1"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Endpoint publik tetap bounded/rate-limited dan tidak membuka detail PII.

### F13-API-2: POST /api/v1/staff/policy-versions

Request `application/json`:

```json
{
  "kind": "booking_terms",
  "content": "Teks contoh untuk review owner.",
  "effective_at": "2026-10-10T00:00:00+07:00"
}
```

Response HTTP 201:

```json
{
  "policy_version_id": "policy-v2",
  "state": "draft",
  "version": 1
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F13-API-3: POST /api/v1/staff/policy-versions/{policy_version_id}/publications

Request `application/json`:

```json
{
  "expected_version": 1,
  "reason": "Hotel approved publication"
}
```

Response HTTP 201:

```json
{
  "publication_id": "publication-example",
  "state": "scheduled"
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

- F13-VT-1: Concurrent publish version conflict tanpa lost update. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F13-VT-2: Future publication dan Asia/Jakarta cutoff boundary. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F13-VT-3: Update policy tidak mengubah booking lama. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F13-VT-4: Invalid contact/window/TTL dan dev flag production ditolak. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Owner policies, cutoff konflik, exceptions, legal privacy/cancellation copy dan publish workflow.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.
