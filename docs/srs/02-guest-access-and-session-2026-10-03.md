# SRS — Akses tamu, verifikasi kepemilikan dan sesi

Dokumen teknis: [TECH-F02](../tech/02-guest-access-and-session-architecture-2026-10-03.md).

ID: SRS-F02-2026-10-03. Status: PROPOSED / REQUIRES CONTRACT REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE identity + FE; hotel menentukan channel. Pasangan: [PRD-F02](../prd/02-guest-access-and-session-2026-10-03.md). Gap: BE-G13, G15.
Kontrak HTTP di bawah adalah proposal target, bukan daftar route yang sudah tersedia. [Kontrak bersama/konflik](00-shared-contracts-and-decisions-2026-10-03.md) menjadi rujukan boundary.

## Otoritas dan prerequisite

Proposed: login belum diobservasi pada vendor. Audit G13 mengharuskan ownership/session; frontend belum memiliki auth.

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F11 Notifikasi booking, payment dan operasional](11-notification-delivery-2026-10-03.md)

Dokumen katalog CRUD, Batch C dan RBAC existing tetap pemilik kontrak pada scope masing-masing. Jika proposal ini memakai nama endpoint existing atau memperluas payload, perubahan memerlukan amendment/versioning review; jangan implementasikan dua kontrak berbeda pada URI yang sama.

## Model resource dan state

| Resource / hubungan | Authority |
|---|---|
| Booking → quote/policy snapshot → occupancy/room selections | BE booking; snapshot immutable |
| Booking → verified guest identity / property | BE ownership; UUID/reference bukan credential |
| Actor → permission → audited mutation | Identity trusted + policy RBAC |
| Resource fitur ini | challenge_pending → verified → active_session → expired/revoked; booking ownership adalah relasi terpisah. |

Snapshot/domain payment, stock, booking, session dan notification tidak dilebur menjadi satu status UI. Schema domain/SQL dan migration rollout dibuat pada tahap tech architecture; spesifikasi ini tidak mengubah enum Go sepihak.

## Functional requirements

### F02-FR-01 — Trace PRD F02-AC-01

Request challenge memberi respons generik untuk email terdaftar/tidak terdaftar; cooldown/rate limit per subject/device/network memakai angka configurable.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

### F02-FR-02 — Trace PRD F02-AC-02

Hotel memilih satu metode email OTP atau magic link; token random kuat disimpan hash, memiliki expiry dan one-time use; contoh OTP di dokumen hanya fixture.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

### F02-FR-03 — Trace PRD F02-AC-03

Verification bersifat atomic; replay/expired/invalid challenge ditolak. Link verification dipisahkan dari GET agar mail scanner tidak mengonsumsi token.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

### F02-FR-04 — Trace PRD F02-AC-04

Sesi menggunakan cookie HttpOnly/Secure/scope/SameSite yang ditetapkan lewat review; rotasi setelah verifikasi, idle/absolute timeout, logout dan revocation.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

### F02-FR-05 — Trace PRD F02-AC-05

Booking disambungkan melalui bukti email/guest identity terverifikasi dan aturan claim historis; tahu reference tidak cukup. Verified booker tidak otomatis memberi akses semua occupant.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

### F02-FR-06 — Trace PRD F02-AC-06

Unauthorized, expiry, revoke menghapus state privat FE; logout tab lain menginvalidasi akses server. Data privat no-store dan tanpa PII/token analytics.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

### F02-FR-07 — Trace PRD F02-AC-07

Reset booking draft tidak logout; expiry login mempertahankan hanya search non-PII dan meminta verifikasi ulang. CSRF protection untuk cookie mutations.

Boundary: validasi/authorization server sebelum effects; transaction membungkus effects lokal yang saling bergantung. Setelah timeout, klien membaca hasil/request intent yang sama; tidak menafsirkan unknown sebagai sukses. Verifikasi memakai acceptance serta F02-SC terkait di PRD.

## HTTP proposal dan akses

| Operasi proposal | Akses | Sukses | Tujuan |
|---|---|---|---|
| `POST /api/v1/guest-access-challenges` | public | 202 | Mutation/request durable sesuai FR dan policy |
| `POST /api/v1/guest-sessions` | public | 201 | Mutation/request durable sesuai FR dan policy |
| `DELETE /api/v1/guest-sessions/current` | guest | 204 | Mutation/request durable sesuai FR dan policy |

Machine-readable: [OpenAPI 3.1 proposal](contracts/booking-roadmap-proposal.openapi.json), tag F02. Semua field menggunakan snake_case. Contoh berikut memakai data sintetik, bukan tarif/credential/property produksi.

### F02-API-1: POST /api/v1/guest-access-challenges

Request `application/json`:

```json
{
  "email": "guest@example.test"
}
```

Response HTTP 202:

```json
{
  "challenge_id": "challenge-example",
  "state": "accepted",
  "resend_after_seconds": 60
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Endpoint publik tetap bounded/rate-limited dan tidak membuka detail PII.

### F02-API-2: POST /api/v1/guest-sessions

Request `application/json`:

```json
{
  "challenge_id": "challenge-example",
  "verification_code": "123456"
}
```

Response HTTP 201:

```json
{
  "session_id": "session-example",
  "expires_at": "2026-10-03T04:00:00Z"
}
```

Validasi: field wajib/types/batas ada di OpenAPI dan aturan lintas-field pada FR; resource opaque ID authorize dahulu. Endpoint publik tetap bounded/rate-limited dan tidak membuka detail PII.

### F02-API-3: DELETE /api/v1/guest-sessions/current

Request: tanpa JSON body; path parameter non-empty, filter/cursor/limit mengikuti kontrak bersama.

Response: HTTP 204, tanpa body; cookie/session di-revoke server.

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

- F02-VT-1: Email A tidak bisa claim/read B walaupun mengetahui reference. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F02-VT-2: OTP/link replay dan verification serentak hanya satu berhasil. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F02-VT-3: Expired/revoked session ditolak dan cache privat tidak bocor. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.
- F02-VT-4: Enumeration/cooldown/resend/screen-reader error diuji. Bukti: input/subject/state awal, response/state akhir, DB/provider effects dan absence duplicate/partial effects; cleanup/replay tercatat.

Tambahkan unit/handler contract valid-invalid-boundary, auth/CSRF/unknown fields, retries dan regression fitur dependensi. Go target >=80% package baru/refinement sesuai AGENTS; race/vet dan real DB/provider sandbox membuktikan boundary yang tidak dicakup mock. Physical-device dan screen reader hasil actual terpisah. Fitur tanpa external side effects tetap memiliki E2E read/deny/refresh.

## Handoff dan keputusan terbuka

OTP versus magic link, channel/provider, TTL, claim historis, retention dan akses booker versus penghuni harus ditetapkan hotel/security.

FE: screen/state/error mapping + view model adapter; BE: transport/domain/auth/transaction; QA: named scenario evidence; hotel: business rules; ops/provider: secrets/sandbox/recovery. Semua owner mengisi keputusan sebelum integrasi yang bergantung padanya.
Status dokumen bukan status implemented/verified. Tidak ada endpoint baru atau pembayaran nyata dijalankan oleh paket ini.

## Alignment dengan Batch D existing

[SRS Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
