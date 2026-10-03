# SRS — Identitas staff, izin dan audit perubahan

Dokumen teknis: [TECH-F12](../tech/12-staff-identity-permissions-and-audit-architecture-2026-10-03.md).

ID: SRS-F12-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE identity/security + GM; existing Casbin owner. Pasangan: [PRD-F12](../prd/12-staff-identity-permissions-and-audit-2026-10-03.md). Gap: BE-G14, G15, G20.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Existing RBAC doc dan G14 audit: header identity belum trusted/fail-open. Ini persyaratan lanjutan, bukan klaim source saat ini belum diperbaiki.

Tidak ada fitur prerequisite; keputusan identity/hotel dapat membatasi integrasi.

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | staff active/inactive; session issued → revoked/expired; role assignment audit versioned. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F12-FR-01 — Trace PRD F12-AC-01

Identity diverifikasi server dari selected IdP/session; X-User-ID/X-User-Role dan literal Bearer role publik tidak memberi authority.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

### F12-FR-02 — Trace PRD F12-AC-02

Enforcer init/load failure fail-closed; readiness tidak hijau untuk protected ops ketika auth dependency unusable.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

### F12-FR-03 — Trace PRD F12-AC-03

Subject active/role/property scope dicek; inactive/revoked staff ditolak walau token valid structurally.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

### F12-FR-04 — Trace PRD F12-AC-04

Authorization deny by default per action/resource; finance refund, revenue tariff, reception stay dan GM exception punya permission spesifik.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

### F12-FR-05 — Trace PRD F12-AC-05

Role changes tidak self-escalate; approval/step-up untuk aksi sensitif ditetapkan owner. Audit gagal ditulis membuat sensitive mutation gagal atomically.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

### F12-FR-06 — Trace PRD F12-AC-06

Audit menyimpan actor verified, action, resource, before/after redacted, reason/time/request_id/version; immutable kepada caller biasa.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

### F12-FR-07 — Trace PRD F12-AC-07

Staff logout/expiry/revoke dan provider event credentials terpisah; secrets/token audit/log di-redact dan semua private response no-store.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F12-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `GET /api/v1/staff/session` | staff | 200 | Read tanpa efek mutation |
| `PATCH /api/v1/staff/users/{staff_id}/role-assignments` | staff | 200 | Mutation/request durable sesuai FR dan policy |
| `GET /api/v1/staff/audit-events` | staff | 200 | Read tanpa efek mutation |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F12. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F12-API-1: GET /api/v1/staff/session

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response HTTP 200:

```json
{
  "staff_id": "staff-example",
  "active": true,
  "permissions": [
    "reservations.read"
  ],
  "property_id": "property-example"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F12-API-2: PATCH /api/v1/staff/users/{staff_id}/role-assignments

Request `application/json`:

```json
{
  "expected_version": 1,
  "roles": [
    "receptionist"
  ],
  "reason": "Approved duty assignment"
}
```

Response HTTP 200:

```json
{
  "staff_id": "staff-example",
  "roles": [
    "receptionist"
  ],
  "version": 2
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Sesi sesuai audience dan ownership/permission server wajib; sensitive mutation audit + CSRF bila cookie.

### F12-API-3: GET /api/v1/staff/audit-events

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response HTTP 200:

```json
{
  "data": [
    {
      "event_id": "audit-example",
      "actor_id": "staff-example",
      "action": "reservation.check_in",
      "resource_id": "booking-example",
      "occurred_at": "2026-10-03T03:00:00Z"
    }
  ],
  "pagination": {
    "next_cursor": null,
    "has_more": false
  }
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

- F12-VT-1: Forged headers/Bearer gm_admin ditolak. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F12-VT-2: Enforcer nil/init failure protected route closed. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F12-VT-3: Role matrix + property/guest cross-resource deny. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F12-VT-4: Inactive staff dan audit write failure tidak mutate. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

Staff IdP/login method, MFA/step-up, role provisioning approvals, audit retention/export dan property scope.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.
