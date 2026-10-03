# TECH — Verifikasi operasional, release gate dan pilot hotel

ID: TECH-F15-2026-10-03. Status: PROPOSED / REQUIRES ARCHITECTURE REVIEW.
Tanggal: 3 Oktober 2026 (Asia/Jakarta). Owner: QA + ops + hotel pilot owner + BE/FE.
[PRD-F15](../prd/15-operational-verification-and-hotel-pilot-2026-10-03.md) · [SRS-F15](../srs/15-operational-verification-and-hotel-pilot-2026-10-03.md) · [Tech registry](README.md).

## 1. Outcome, scope dan evidence

Fitur yang terlihat lengkap belum membuktikan stock/payment integrity, perangkat nyata, recovery atau operasional staff.

Target teknis: Coverage/test/evidence, concurrency/failure/replay, observability, backup/restore, rollout/rollback, limited pilot and support.
Di luar scope: Tidak menetapkan tanggal live, throughput/SLO/RPO/RTO numerik tanpa workload/owner; docs/test green tidak dianggap approval produksi.

Audit G20: mocked coverage/build bukan bukti DB/provider/delivery. QA FE partial Chromium dan device NOT RUN; pilot belum dilaksanakan.
Dokumen ini adalah target desain, bukan bukti implementasi atau re-audit source terbaru. Backend sedang berubah; cek source, SQL, version/runtime dan existing owner sebelum mengedit.
Rancangan menambah use case di existing modular monolith; tidak membangun service baru atau mengganti pemilik kontrak fitur lain.
Lihat [kontrak/keputusan C01–C14](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Status approved dokumen existing tidak dipindahkan ke dokumen ini.

## 2. Prerequisite dan authority

- [F01 Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-architecture-2026-10-03.md).
- [F02 Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-architecture-2026-10-03.md).
- [F03 Booking Saya dan detail reservasi privat](03-my-bookings-architecture-2026-10-03.md).
- [F04 Konfirmasi, bukti booking dan kalender](04-booking-confirmation-artifacts-architecture-2026-10-03.md).
- [F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-architecture-2026-10-03.md).
- [F06 Bantuan tamu, perubahan dan pembatalan](06-guest-assistance-and-booking-requests-architecture-2026-10-03.md).
- [F07 Resepsionis, assignment kamar dan lifecycle menginap](07-front-desk-stay-operations-architecture-2026-10-03.md).
- [F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-architecture-2026-10-03.md).
- [F09 Manajemen tarif, paket dan promo](09-rate-plan-and-promo-management-architecture-2026-10-03.md).
- [F10 Sinkronisasi kanal dan pencegahan overselling](10-channel-inventory-synchronization-architecture-2026-10-03.md).
- [F11 Notifikasi booking, payment dan operasional](11-notification-delivery-architecture-2026-10-03.md).
- [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-architecture-2026-10-03.md).
- [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-architecture-2026-10-03.md).
- [F14 Rekonsiliasi pembayaran, late payment dan refund](14-finance-reconciliation-and-refunds-architecture-2026-10-03.md).


Authority domain: Go/PostgreSQL untuk booking/quote/inventory/payment; verified identity untuk guest/staff; provider hanya melalui verified adapter. FE memegang view state, bukan otoritas financial/stock/policy. Retention, provider, stock allocation, TTL dan thresholds yang belum approved tetap keputusan terbuka.

## 3. Arsitektur dan batas komponen

Module/use case: **QA/ops evidence tooling and deployment controls**.

```mermaid
flowchart LR
    Actor["Actor F15 / authorized workflow"] --> Boundary["Transport atau harness: validate + auth"]
    Boundary --> UseCase["F15: QA/ops evidence tooling and deployment controls"]
    UseCase --> Policy["Owner policy dan invariants"]
    UseCase --> Store[("Existing PostgreSQL / evidence store")]
    UseCase --> Adapter["External adapter atau read projection"]
    Adapter --> Recovery["Retry / reconciliation sesuai fitur"]
```

Diagram menunjukkan logical ownership, bukan deployable microservices. Untuk pure read F03, external adapter adalah DTO projection; untuk F15, harness/evidence bukan HTTP publik.
Manual constructor injection dan ports yang dibutuhkan use case cukup; jangan tambah DI framework/state library/message broker tanpa kebutuhan terukur. Nuxt/BFF bila dipilih hanya session/proxy boundary, tidak menghitung ulang harga atau stock.

Ports/adapters yang diperlukan: Test harness/environment manager, evidence collector, metrics reader, release approval/runbook; no public go-live/fault injection endpoints.
Interface signature Go final ditetapkan setelah current source/version/owner contract diperiksa; tidak menghasilkan skeletal interfaces yang hanya meniru semua repository.

## 4. Model data dan constraints proposal

| Record / sumber | Field / hubungan | Constraints / consistency |
|---|---|---|
| verification manifests/artifacts | Build/source hash,scenario/env/commands/assertions/cleanup/result/reviewer | No secrets/real PII in evidence; immutable versioned report refs |
| pilot approvals/runbooks | stock/channel allowlist/support owners/windows/go-no-go | Explicit owner accepted/deferred gates, scoped controls versioned |
| backup/recovery checkpoints | Restore timestamps/db counts/payment/outbox reconciliation | Backup existence alone not proof restore success; provider money never rolled back by DB restore |

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

1. Freeze requirements/contracts/source and name isolated test environment/workload.
2. Execute tests unit/handler/race/vet and DB/provider concurrency/failure/replay with pre/post invariants and cleanup.
3. Run backup restore into isolated DB; reconcile ledger/provider/outbox, pause unsafe sends before recovery.
4. Measured device/browser/accessibility and reception/finance walkthrough record steps/results/screenshots.
5. Pilot limited owner-approved stock/channels/guests with rollback and on-call; monitor unknown payments/stock drift.
6. Hotel/ops go-no-go references actual evidence; abort stops new bookings but preserves paid/unknown financial intents for reconciliation.

Batas transaksi lokal harus merangkum effects yang saling bergantung; external I/O tidak ditahan dalam DB row locks. Lock order eksplisit setelah entity table schema inspected; retry bounded hanya untuk conflict/deadlock yang aman dan idempotent.
Read/status refresh tidak membuat intent charge/refund/booking. State “unknown” tidak dianggap failed/success; user diarahkan ke status/reconciliation. Cancellation/reset/log-out adalah operasi berbeda, bukan alias delete financial history.

Domain/view state: gate planned → executed → passed/failed/not_run; pilot proposed → approved → running → evaluated → go/no_go.
Domain enum existing dipertahankan; serializer/adapter memetakan view tanpa mengganti enum approved secara sepihak.

## 6. HTTP/FE integration boundary

Endpoint/request/response/errors dimiliki [SRS-F15](../srs/15-operational-verification-and-hotel-pilot-2026-10-03.md); contoh machine-readable ada di [OpenAPI proposal](../srs/contracts/booking-roadmap-proposal.openapi.json).
Transport validates bounded body/unknown fields/DateOnly/opaque IDs; normalize payload hanya setelah hashing strategy owner ditentukan. Existing Batch D raw-body hash/key1–64 dan nested guest DTO proposal perlu C14 alignment.
Guest read/mutation selalu resolve owner; staff role/property/action trusted; provider signature/raw bytes mengikuti provider, bukan static demo header.
FE adapter tidak membawa PII/access token di URL/localStorage/analytics; private views no-store dan dibersihkan setelah expired/revoked session. Fixture tetap terpisah dan labelled demo.

## 7. Keamanan dan privacy

QA: execute isolated scenarios; ops: drill/release; hotel owner: accept pilot; GM: go/no-go; guest: pilot scope approved.
- Authorization server dan application resource policy melindungi HTTP maupun job/internal callers; role UI bukan izin domain.
- Cookie auth memerlukan CSRF, HttpOnly/Secure/SameSite/session expiry/revoke proposal review; token raw tidak dicetak/log.
- Generic cross-owner not-found dan auth challenge acceptance; rate limit scope berdasarkan risiko endpoint.
- Sensitive mutation audit mencatat trusted actor/reason/request_id/version; jika evidence mandatory gagal, transaction jangan silent succeed.
- Secrets/provider credentials hanya konfigurasi secret store/deployment owner. Sandbox berbeda live.
- Export/download/evidence PII dibatasi, masked/redacted dan retention disetujui. Tidak ada klaim compliance certification dari dokumen ini.

Risiko khusus: Production data/secret isolation, rollback schema and irreversible charges; no auto-delete/reseed on restore; SLO/RPO/RTO owner-measured.

## 8. ADR-F15-01 — Evidence-backed scoped pilot with explicit abort and reconciliation

Status: PROPOSED. Context: Fitur yang terlihat lengkap belum membuktikan stock/payment integrity, perangkat nyata, recovery atau operasional staff.
Keputusan yang diusulkan: Evidence-backed scoped pilot with explicit abort and reconciliation dalam modular monolith dan ports/adapters existing.
Alternatif/trade-off: Big-bang live switch fast but unproven concurrency; mock-only passing suites cannot verify SQL/provider; simulation useful preflight not final acceptance.
Konsekuensi: invariant lokal mudah diuji dan ownership jelas; schema/retry/version semantics menambah tanggung jawab implementasi. Existing approved interfaces/state butuh review kompatibilitas; stakeholder sign-off sebelum accepted.
Approval record: belum ada. Owner mencatat pilihan/alasan/approver/tanggal/effective scope; tidak menganggap proposal ini accepted.

## 9. Observability, benchmark dan kapasitas

named scenario pass/failed/not_run, baseline latency/resource, restore time/data loss, pilot drift/unknowns and incident response.

Log terstruktur mencatat operation/result/request_id/resource reference redacted; metric labels tidak memakai email/booking ID/provider reference/high-cardinality IDs. Error stack internal tidak menjadi response publik.
Ukur p50/p95/p99, DB lock waits/query rows/pool saturation, worker backlog/retry dan allocations bila use case Go baru/refined. Pisahkan DB waktu, provider latency dan render/browser timing.
Workload baseline harus mencatat jumlah nights/rooms/occupancy/data/parallelism/rate dan mesin; status test fixture tidak disamakan beban produksi. SLO/timeout/retry budgets/RPO/RTO angka di-owner review setelah measurement; tidak mengklaim quote5ms dari dokumen owner sebagai hasil benchmark.

## 10. Rencana file/module dan migrasi

| Area | Tugas rencana |
|---|---|
| Nuxt FE | Typed DTO adapter, view states dan keyboard/error/recovery pada scope fitur |
| BE transport | Extend existing routes/middleware; compatibility/auth/errors mengikuti SRS owner |
| BE application/domain | Use cases pada QA/ops evidence tooling and deployment controls; invariants, policy checks, stable error mapping |
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
| F15-FR-01 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-01; positive/negative/boundary, actual effects |
| F15-FR-02 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-02; positive/negative/boundary, actual effects |
| F15-FR-03 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-03; positive/negative/boundary, actual effects |
| F15-FR-04 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-04; positive/negative/boundary, actual effects |
| F15-FR-05 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-05; positive/negative/boundary, actual effects |
| F15-FR-06 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-06; positive/negative/boundary, actual effects |
| F15-FR-07 | Use case F15, policy/transport/persistence boundaries sesuai bagian 3–7 | PRD F15-AC-07; positive/negative/boundary, actual effects |

Named scenarios:
- F15-TECH-VT-1: Backup restore drill proves balances/reservations/outbox continuity. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.
- F15-TECH-VT-2: No overselling N clients and cleanup ledger proof. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.
- F15-TECH-VT-3: Real devices/session recovery/staff operations walkthrough. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.
- F15-TECH-VT-4: Go/no-go rejects absent provider evidence and rollback ownership. Catat pre/post records, duplicate/rollback proof, command/environment/source hash/result dan cleanup.

Wajib unit/handler contract dan permissions matrix; Go package baru/refinement target >=80% table-driven coverage, go vet dan race sesuai AGENTS. SQL locks/GiST/unique/rollback diuji DB isolated; provider signatures/replay/sandbox dan actual email/channel delivery terpisah.
Browser refresh/back/expiry/keyboard/200%/reduced-motion bila UI; Chrome Android/Safari iOS actual dicatat sebagai actual, emulation tidak menggantikannya. Lint OpenAPI/compiled docs tidak menutup integration gates.

## 12. Handoff dan keputusan terbuka

Pilot dates/rooms/channel scope, numeric SLO/RPO/RTO/thresholds, support rota, deployment topology dan acceptable deferred risks.
Tech-specific unresolved risk: Production data/secret isolation, rollback schema and irreversible charges; no auto-delete/reseed on restore; SLO/RPO/RTO owner-measured.

FE/BE/QA/hotel/ops mengisi approved contract revision, migration impact, rollout switch, rollback owner dan acceptance evidence. Status IMPLEMENTED_UNVERIFIED boleh setelah source tersedia; VERIFIED hanya setelah required tests actual. Paket dokumen ini tidak mengubah source/database/provider atau menjalankan deployment.

