# Kontrak bersama dan register keputusan lintas fitur

Status: PROPOSED / NEEDS ALIGNMENT. Tanggal: 3 Oktober 2026 (Asia/Jakarta). Owner: BE/FE lead + hotel + QA. [Registry SRS](README.md).

## Otoritas sumber

| Sumber | Apa yang dibuktikan | Batas |
|---|---|---|
| [Riset vendor](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md) | Search/result/review publik, 5 keluarga/7 varian, snapshot harga/benefit/policy | 3–4 Oktober, satu occupancy; login/payment/receipt/dashboard belum diobservasi |
| [Desain resmi](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/official-design-system.md) | Brand visual dan observed check-in15/check-out12 | Bukan approval semua business rules/assets |
| [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md) | Partial rendered Chromium/unit/build evidence | Tidak membuktikan real stock/payment/device readiness atau complete acceptance |
| [Gap register](../gap/README.md) | Audit snapshot G01–G22 | Source berubah; remediasi terkini perlu source verification sebelum implementation |
| [Batch C owner](pricing-quote-policies-batch-c-2026-10-03.md) | Dokumen berstatus Approved for Implementation | Approval dibaca dari dokumen existing, bukan ditetapkan paket ini; konflik harus direview |
| [Batch D owner](checkout-idempotency-payment-batch-d-2026-10-03.md) | Dokumen berstatus approved checkout/idempotency/payment | Source sedang berubah; bukan bukti SQL/provider verification |
| [Catalog owner](catalog-crud-management-2026-10-03.md) | CRUD owner/contract existing | Seed IDs/codes tidak otomatis mapping brand resmi |
| [RBAC owner](hotel-rbac-casbin-2026-10-03.md) | Permission owner existing | Matriks permission tidak membuktikan trusted authentication/fail-closed |

## Conflict / decision register

| ID | Perbedaan / kebutuhan | Resolusi yang diperlukan | Owner / gate |
|---|---|---|---|
| C01 | Fixture FE IDR exponent2; Batch C IDR exponent0 | Pertahankan backend owner sampai amendment; pilih rounding display/transport/provider, konversi eksplisit golden cases | Finance + BE/FE, sebelum live Money adapter |
| C02 | Vendor snapshot OCTOBREAK27%; Batch C15% | Bedakan observed snapshot dan target approved; jangan publish promo contoh sebagai tarif hotel | Revenue + hotel, F09 |
| C03 | Vendor deluxe breakfast selisih109500; Batch C100000/dewasa/night; suite snapshot breakfast included | Entitlement per rate/occupancy dan quote examples; suite tidak ditambah deluxe surcharge otomatis | Revenue + hotel, F08/F09 |
| C04 | Vendor prices sudah termasuk tax/service; Batch C rumus added tax | Tentukan taxable base/charges/service/rounding dan owner-reviewed legal source; SRS baru tidak menetapkan hukum pajak | Finance + hotel, F09/F13 |
| C05 | FE demo quote/hold30m; Batch C quote15m | Quote TTL dan hold/payment deadline adalah objek berbeda; server authority, expected boundaries | BE + hotel, F01/F05 |
| C06 | Official check-in15/check-out12; Batch C cancellation cutoff14WIB dan frasa48h | Tentukan cutoff cancellation yang tepat sebagai policy, tidak samakan dengan check-in; immutable historic version | Hotel + ops, F06/F13 |
| C07 | Catalog seed codes/names berbeda dari 7 public variants | Mapping family/variant/sellable/physical IDs owner-approved; no destructive remap history | Revenue + BE, F08/F10 |
| C08 | Guest login method belum diamati/disepakati | Pilih email OTP atau magic link, channel/TTL/anti-enumeration, claim historical booking, booker vs occupant | Hotel + security, F02 |
| C09 | Stock per variant/allotment/channel authority belum verified | Source authority dan outage/stop-sell/maintenance/continuous-room policy | Ops + revenue, F08/F10 |
| C10 | Provider payment/refund, late-payment policy belum dipilih | Sandbox signing/idempotency, amount exponent, finance approval/refund eligibility | Finance + BE, F05/F14 |
| C11 | Staff identity provider dan session/MFA belum dipilih | Trusted auth/fail-closed + approved matrix/exception/audit retention | GM + security, F12 |
| C12 | Template/contact/assets/notif channel/retention belum owner-approved | Inventory assets/provenance; truthful delivery and privacy retention | Hotel + FE/BE, F04/F11/F13 |
| C13 | Pilot timing/workload/SLO/RPO/RTO/support belum ditetapkan | Measured targets dan named owner, acceptance/rollback/drill evidence | QA/ops + hotel, F15 |
| C14 | Batch D key1–64/raw-body hash, special_requests/guest_phone/arrival fields, X-Guest-Token dan initiated/success/failed/received_after_expiry berbeda dengan guest-session/normalized DTO proposal | Map adapter dan amendment per owner; query/session access model dan normalized payment view tidak mengubah domain enum approved. Review retry raw-body fingerprint dan retained cached PII | BE + security/FE, F01/F02/F05 |

Semua C01–C14 OPEN pada paket proposal ini. Existing owner requirement tidak dibatalkan oleh konflik. Tidak ada angka pricing/provider contoh dianggap operational hotel approval.

## HTTP conventions

- Base target `/api/v1`, field `snake_case`; OpenAPI versi `0.1.0-proposal`. Server spec localhost sengaja hanya mock, bukan endpoint produksi.
- Existing route/payload harus dipetakan dan direview owner. Khusus create bookings, occupancy/quote aggregation dan strict consent adalah target extension; jangan kirim fields tanpa memastikan BE menerima/menegakkan.
- Breaking required field/status/schema/auth change membutuhkan amendment atau major-version migration dengan periode compatibility yang disepakati; tidak menghapus route lama mendadak.
- POST create durable resource:201 + Location. Async accepted:202 berarti queued, bukan completed. DELETE session:204 tanpa body.
- Koleksi: `cursor` opaque + `limit` default20 min1 max100 (proposal); order created_at/id; owner/property/filter sebelum pagination; next_cursor nullable/has_more. Search/availability juga bounded date horizon/occupancy. Invalid cursor400; akhir daftar200 kosong.
- Query search tidak membawa guest PII atau access token. Server/proxy log redaction berlaku untuk email/challenge/provider event.
- Private response `Cache-Control:no-store`; public catalog metadata boleh conditional cache hanya setelah owner TTL, search tidak cache stock tanpa version/freshness.
- Idempotency-Key untuk create financial/stock/notification intents, scope actor/resource/property + payload fingerprint persisted. Replay key/payload sama return resource yang sama; key sama payload lain409. Key retention/replay window owner-configured.
- expected_version atau If-Match menjaga concurrent staff edits; stale412. Audit sensitive mutation ikut atomic transaction.
- Bounded payload/horizon/occupancy/rate limit configured: angka body64KiB adalah batas proposal OpenAPI examples, bukan kapasitas produksi approved. Rate-limit endpoint-specific harus dikalibrasi;429 Retry-After.
- Timestamp RFC3339 UTC/offset. Hotel policy Asia/Jakarta; DateOnly kalender Gregorian; checkout exclusive. Durasi quote, hold, payment, session dan token tidak saling mengganti.

## Identity and permission

Guest cookie session proposal diverifikasi server dengan CSRF proteksi untuk mutations; HttpOnly/Secure/SameSite/expiry/revocation final pada F02 review. Staff session namespace terpisah F12; source role trusted, bukan header user. Challenge acceptance generik mencegah enumeration.
Guest read setiap booking/artifact/ticket memeriksa ownership; cross-owner404 generic. Staff mutation memeriksa active role/property/action dan reason. Provider signature header pada spec hanya abstract placeholder; algorithm/timestamp/raw bytes mengikuti provider terpilih, jangan memproduksikan contoh header static sebagai verifier.

## Error envelope

```json
{
  "type": "/problems/validation-error",
  "title": "Input tidak valid",
  "status": 400,
  "detail": "Periksa tanggal check-out.",
  "code": "VALIDATION_ERROR",
  "request_id": "req-example",
  "retryable": false,
  "errors": [{"field": "check_out", "message": "Harus setelah check_in."}]
}
```

Content-Type `application/problem+json`. type adalah stable URI reference. 400 validation/unknown field,401 authentication,403 permission,404 unknown/cross-owner,409 domain/key conflict,410 quote expiry,412 stale version,413 oversized,429 limiter,500 unexpected safe error,503 dependency unknown. Codes approved Batch C dipertahankan lewat mapping eksplisit.
Retry read dengan bounded backoff; mutation ambigu read intent/key yang sama. Tidak mengubah status expired/paid/confirmed hanya dari FE timer atau return URL.

## Money, snapshots and ownership

Money `{amount:integer,currency:string,exponent:integer}`; contoh target BE memakai IDR/exponent0 mengikuti Batch C, fixture FE exponent2 tetap fixture. Tidak memakai floating point untuk kalkulasi domain. Amount cross-field currency/exponent mesti sama; rounding/display/provider rules perlu C01/C04 resolution.
Persisted quote/policy/consent dan per-room occupancy snapshot tidak berubah ketika rate/policy publish. Booking owner authenticated berbeda dari occupant count. Staff check-in tidak memberi occupant akses akun otomatis.
Domain enum existing tidak dianggap sama dengan view pending_payment/processing/needs_assistance; adapter memakai booking_status/payment_status dan allowed_actions. Stock authority Go/PostgreSQL/approved channel integration, bukan Nuxt atau timer.

## Verification paket dokumen

OpenAPI proposal mendokumentasikan target DTO/core endpoints dan auth abstract; tidak mengganti contract approved, provider payload terpilih atau schema migration.
Validation report tersimpan di [laporan validasi](contracts/validation-report.md). Lint/mock hanya memverifikasi struktur/response contoh; ownership/SQL/idempotency/payment signing tetap memerlukan implementation tests nyata.
Semua PRD/SRS baru PROPOSED. Tidak ada kode backend/schema/konfigurasi/provider live dimutasi; dokumen existing dipertahankan.
