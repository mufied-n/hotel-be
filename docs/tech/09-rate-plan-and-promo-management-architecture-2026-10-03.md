# TECH — Manajemen tarif, paket dan promo

ID: TECH-F09-2026-10-03. Status: PROPOSED / REQUIRES ARCHITECTURE REVIEW.
Tanggal: 3 Oktober 2026 (Asia/Jakarta). Owner: Revenue manager + BE rates; Batch C owner.
[PRD-F09](../prd/09-rate-plan-and-promo-management-2026-10-03.md) · [SRS-F09](../srs/09-rate-plan-and-promo-management-2026-10-03.md) · [Tech registry](README.md).

## 1. Outcome, scope dan evidence

Harga/promosi harus dapat diatur hotel tanpa kode hardcoded atau biaya sarapan/tax ditambahkan dua kali.

Target teknis: Per-variant plan availability, benefit/breakfast entitlement, seasonal rates, promo eligibility/window/quota, owner Money contract.
Di luar scope: Dynamic AI revenue optimisation dan multi-currency conversion tidak termasuk; pajak hukum tidak ditetapkan dokumen ini.

Observed: deluxe RO/Breakfast, suite Breakfast, snapshot OCTOBREAK 27% dan included charges. Existing Batch C approved berbeda: 15%, exponent0, 15min. Tidak ditimpa paket baru.
Dokumen ini adalah target desain, bukan bukti implementasi atau re-audit source terbaru. Backend sedang berubah; cek source, SQL, version/runtime dan existing owner sebelum mengedit.
[Tech owner existing](pricing-quote-policies-architecture-2026-10-03.md) tetap dipertahankan. Extension/interoperability di sini membutuhkan amendment ketika berbeda dengan owner approved.
Lihat [kontrak/keputusan C01–C14](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Status approved dokumen existing tidak dipindahkan ke dokumen ini.

## 2. Prerequisite dan authority

- [F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-architecture-2026-10-03.md).
- [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-architecture-2026-10-03.md).
- [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-architecture-2026-10-03.md).


Authority domain: Go/PostgreSQL untuk booking/quote/inventory/payment; verified identity untuk guest/staff; provider hanya melalui verified adapter. FE memegang view state, bukan otoritas financial/stock/policy. Retention, provider, stock allocation, TTL dan thresholds yang belum approved tetap keputusan terbuka.

## 3. Arsitektur dan batas komponen

Module/use case: **rates/quote + revenue management transport**.

```mermaid
flowchart LR
    Actor["Actor F09 / authorized workflow"] --> Boundary["Transport atau harness: validate + auth"]
    Boundary --> UseCase["F09: rates/quote + revenue management transport"]
    UseCase --> Policy["Owner policy dan invariants"]
    UseCase --> Store[("Existing PostgreSQL / evidence store")]
    UseCase --> Adapter["External adapter atau read projection"]
    Adapter --> Recovery["Retry / reconciliation sesuai fitur"]
```

Diagram menunjukkan logical ownership, bukan deployable microservices. Untuk pure read F03, external adapter adalah DTO projection; untuk F15, harness/evidence bukan HTTP publik.
Manual constructor injection dan ports yang dibutuhkan use case cukup; jangan tambah DI framework/state library/message broker tanpa kebutuhan terukur. Nuxt/BFF bila dipilih hanya session/proxy boundary, tidak menghitung ulang harga atau stock.

Ports/adapters yang diperlukan: Rates repository, promo evaluator/usage store, quote engine/snapshot store, policy version reader.
Interface signature Go final ditetapkan setelah current source/version/owner contract diperiksa; tidak menghasilkan skeletal interfaces yang hanya meniru semua repository.

## 4. Model data dan constraints proposal

| Record / sumber | Field / hubungan | Constraints / consistency |
|---|---|---|
| rate_plans/calendar/promotions | Reuse Batch C owner types and reviewed extensions for per-variant eligibility/version | Unique plan/code scope; validity windows, no overlapping ambiguous calendar priority |
| quotes + policy/pricing snapshots | Immutable input/items/pricing/exponent/rounding/expiry | Correct owner TTL; valid quote not silently reprice |
| promotion usage/reservations (proposal) | promo+booking commitment, quota consumed/released records | Unique commitment per booking/promo; atomic quota concurrency |

Nama tabel bertanda proposal adalah logical model, belum migration yang dijalankan. Reuse existing owner tables jika sudah tersedia; jangan membuat dua ledger/session/quote store untuk domain yang sama.
Opaque IDs memakai strategi existing owner; UUIDv7 database function/version hanya dipakai jika actual runtime/migration capability verified. Foreign keys/history retention harus mencegah cleanup UI menghapus financial evidence.

Query/index review:
- Equality tenant/property/owner sebelum tanggal/status/pagination untuk records privat.
- Cursor order stabil created_at/id; compose index sesuai actual query EXPLAIN, bukan menambah index setiap field.
- Unique keys untuk replay/event/intent scopes; conflicts dibedakan validation versus concurrent mutation.
- Harga integer currency/exponent sesuai owner Batch C; datetime UTC RFC3339 dan DateOnly hotel Asia/Jakarta.
- Version guard untuk edit staff; immutable snapshot untuk quote/policy yang sudah disetujui tamu.
- Retention/delete/anonymize membutuhkan owner approval dan pemeriksaan references; no destructive cascade default pada money/history.

## 5. Alur transaksi dan recovery

1. Revenue drafts plan/benefit/breakfast/calendar; policy/time overlap and Money validation run before publish.
2. Quote reads active owner-approved rate and entitlement per variant/date/occupancy; resolves promo conditions.
3. Integer calculation order and rounding golden cases produce original/discount/charges/final; no universal deluxe surcharge suite.
4. Snapshot persisted with policy/consent version and expiry; published change affects future quote only per lock policy.
5. Create booking commits promo quota with booking/hold inside transaction; replay not double-consume.
6. Expired/price change returns re-review guidance; FE adapter handles approved exponent0 versus fixture2 explicitly.

Batas transaksi lokal harus merangkum effects yang saling bergantung; external I/O tidak ditahan dalam DB row locks. Lock order eksplisit setelah entity table schema inspected; retry bounded hanya untuk conflict/deadlock yang aman dan idempotent.
Read/status refresh tidak membuat intent charge/refund/booking. State “unknown” tidak dianggap failed/success; user diarahkan ke status/reconciliation. Cancellation/reset/log-out adalah operasi berbeda, bukan alias delete financial history.

Domain/view state: rate/promo draft → scheduled/active → expired/inactive; quote immutable sampai expiry.
Domain enum existing dipertahankan; serializer/adapter memetakan view tanpa mengganti enum approved secara sepihak.

## 6. HTTP/FE integration boundary

Endpoint/request/response/errors dimiliki [SRS-F09](../srs/09-rate-plan-and-promo-management-2026-10-03.md); contoh machine-readable ada di [OpenAPI proposal](../srs/contracts/booking-roadmap-proposal.openapi.json).
Transport validates bounded body/unknown fields/DateOnly/opaque IDs; normalize payload hanya setelah hashing strategy owner ditentukan. Existing Batch D raw-body hash/key1–64 dan nested guest DTO proposal perlu C14 alignment.
Guest read/mutation selalu resolve owner; staff role/property/action trusted; provider signature/raw bytes mengikuti provider, bukan static demo header.
FE adapter tidak membawa PII/access token di URL/localStorage/analytics; private views no-store dan dibersihkan setelah expired/revoked session. Fixture tetap terpisah dan labelled demo.

## 7. Keamanan dan privacy

revenue_mgr: draft/publish tarif; gm_admin: approval sesuai policy; guest: quote read.
- Authorization server dan application resource policy melindungi HTTP maupun job/internal callers; role UI bukan izin domain.
- Cookie auth memerlukan CSRF, HttpOnly/Secure/SameSite/session expiry/revoke proposal review; token raw tidak dicetak/log.
- Generic cross-owner not-found dan auth challenge acceptance; rate limit scope berdasarkan risiko endpoint.
- Sensitive mutation audit mencatat trusted actor/reason/request_id/version; jika evidence mandatory gagal, transaction jangan silent succeed.
- Secrets/provider credentials hanya konfigurasi secret store/deployment owner. Sandbox berbeda live.
- Export/download/evidence PII dibatasi, masked/redacted dan retention disetujui. Tidak ada klaim compliance certification dari dokumen ini.

Risiko khusus: C01–C06 existing approved conflicts; arithmetic/design approval required before operational tariff publish.

## 8. ADR-F09-01 — Versioned rates and quote snapshot as checkout price authority

Status: PROPOSED. Context: Harga/promosi harus dapat diatur hotel tanpa kode hardcoded atau biaya sarapan/tax ditambahkan dua kali.
Keputusan yang diusulkan: Versioned rates and quote snapshot as checkout price authority dalam modular monolith dan ports/adapters existing.
Alternatif/trade-off: FE computes discount fast but divergent backend totals; mutable current rate join rewrites historic pricing; optimization engine not warranted.
Konsekuensi: invariant lokal mudah diuji dan ownership jelas; schema/retry/version semantics menambah tanggung jawab implementasi. Existing approved interfaces/state butuh review kompatibilitas; stakeholder sign-off sebelum accepted.
Approval record: belum ada. Owner mencatat pilihan/alasan/approver/tanggal/effective scope; tidak menganggap proposal ini accepted.

## 9. Observability, benchmark dan kapasitas

quote p95/allocation, invalid promo eligibility, expired quote rejection, quota contention, pricing invariants; benchmark no arbitrary5ms pass.

Log terstruktur mencatat operation/result/request_id/resource reference redacted; metric labels tidak memakai email/booking ID/provider reference/high-cardinality IDs. Error stack internal tidak menjadi response publik.
Ukur p50/p95/p99, DB lock waits/query rows/pool saturation, worker backlog/retry dan allocations bila use case Go baru/refined. Pisahkan DB waktu, provider latency dan render/browser timing.
Workload baseline harus mencatat jumlah nights/rooms/occupancy/data/parallelism/rate dan mesin; status test fixture tidak disamakan beban produksi. SLO/timeout/retry budgets/RPO/RTO angka di-owner review setelah measurement; tidak mengklaim quote5ms dari dokumen owner sebagai hasil benchmark.

## 10. Rencana file/module dan migrasi

| Area | Tugas rencana |
|---|---|
| Nuxt FE | Typed DTO adapter, view states dan keyboard/error/recovery pada scope fitur |
| BE transport | Extend existing routes/middleware; compatibility/auth/errors mengikuti SRS owner |
| BE application/domain | Use cases pada rates/quote + revenue management transport; invariants, policy checks, stable error mapping |
| BE persistence | Reuse owner repositories; reviewed schema/index/unique/version changes |
| Workers/provider | Implement only ports needed; sandbox/retry/dedup/reconciliation bila ada external effects |
| Tests/evidence | Table-driven unit/handler, isolated integration, E2E + cleanup proof, physical-device bila UI |

Target direktori mengikuti current source organisasi cmd/internal/migrations/testing; nama/path file final dipetakan pada walkthrough, bukan diasumsikan semua package baru sudah ada.

Rollout/migration sequence:
1. Reconcile approved schema/contracts and OPEN decisions; freeze proposal revision yang diterapkan.
2. Expand additive columns/tables/indexes only after volume/locking/schema version review. Jangan reuse nomor migration yang sedang dikerjakan Batch D atau fitur lain.
3. Backfill dengan checkpoint/reconciliation, bounded batches, validated constraints; data fixture bukan data live seed.
4. Deploy compatibility readers/writers dan test old/new paths; feature flag/allowlist default sesuai approved rollout.
5. Verify real data effects/provider sandbox and owner acceptance; enable scope terbatas.
6. Contract/deprecate legacy hanya setelah consumers migrated; rollback aplikasi membiarkan schema additive/data histories aman.

Down migration yang menghapus ledger/booking/session evidence bukan default rollback produksi. Database restore/rollback tidak membatalkan provider charges/refunds; reconciliation F14/F15 harus diperhitungkan. Jika tidak perlu schema change (misal pure FE reset), dokumentasikan no-migration secara eksplisit saat actual implementation.

## 11. Traceability dan verification

| SRS requirement | Design owner | Verification |
|---|---|---|
| F09-FR-01 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-01; positive/negative/boundary, actual effects |
| F09-FR-02 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-02; positive/negative/boundary, actual effects |
| F09-FR-03 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-03; positive/negative/boundary, actual effects |
| F09-FR-04 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-04; positive/negative/boundary, actual effects |
| F09-FR-05 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-05; positive/negative/boundary, actual effects |
| F09-FR-06 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-06; positive/negative/boundary, actual effects |
| F09-FR-07 | Use case F09, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F09-AC-07; positive/negative/boundary, actual effects |

Named scenarios:
- F09-TECH-VT-1: Suite quote benar; RO tidak tersedia ditolak. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.
- F09-TECH-VT-2: Promo boundary timezone/expired/quota replay serentak. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.
- F09-TECH-VT-3: Included fees tidak ditambah lagi dan integer rounding golden cases. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.
- F09-TECH-VT-4: Tariff update tidak mengubah persisted booking total. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.

Wajib unit/handler contract dan permissions matrix; Go package baru/refinement target >=80% table-driven coverage, go vet dan race sesuai AGENTS. SQL locks/GiST/unique/rollback diuji DB isolated; provider signatures/replay/sandbox dan actual email/channel delivery terpisah.
Browser refresh/back/expiry/keyboard/200%/reduced-motion bila UI; Chrome Android/Safari iOS actual dicatat sebagai actual, emulation tidak menggantikannya. Lint OpenAPI/compiled docs tidak menutup integration gates.

## 12. Handoff dan keputusan terbuka

Conflict register C01–C06 wajib resolve sebelum adapter live; tarif, benefit, breakfast dan tax owner approval.
Tech-specific unresolved risk: C01–C06 existing approved conflicts; arithmetic/design approval required before operational tariff publish.

FE/BE/QA/hotel/ops mengisi approved contract revision, migration impact, rollout switch, rollback owner dan acceptance evidence. Status IMPLEMENTED_UNVERIFIED boleh setelah source tersedia; VERIFIED hanya setelah required tests actual. Paket dokumen ini tidak mengubah source/database/provider atau menjalankan deployment.

