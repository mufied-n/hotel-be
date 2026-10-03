# PRD — Sinkronisasi kanal dan pencegahan overselling

Dokumen teknis: [TECH-F10](../tech/10-channel-inventory-synchronization-architecture-2026-10-03.md).

ID: PRD-F10-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE integrations + hotel channel owner. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F10](../srs/10-channel-inventory-synchronization-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Gap audit G18: stock authority kanal lain belum diputuskan. Provider channel dan vendor payload belum diteliti; kontrak canonical di sini proposal.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G18, G20, G21**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Booking website dan OTA dapat menjual stok yang sama tanpa koordinasi atau reconciliation.

Outcome: Authority model, provider mappings, external reservation events, retries, lag/drift and allocation safeguards. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

integration service: signed events; revenue/ops: mappings/reconciliation; guest: read resulting sellability. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Mapping channel → import event → deduplicate → transactional stock ledger → outbox sync → reconcile drift.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Authority model, provider mappings, external reservation events, retries, lag/drift and allocation safeguards.

Di luar scope: Jangan mengklaim zero overselling lintas vendor yang eventual; OTA/property connectors belum dipilih.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F10-AC-01

Kebutuhan: Owner memilih source authority/channel manager atau partitioned allotments; jangan menjalankan dua independent writers pada stock sama.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-01.

### F10-AC-02

Kebutuhan: Mapping provider property/variant/rate/external booking ke internal IDs versioned; unmapped event dikarantina dan tidak menebak room type.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-02.

### F10-AC-03

Kebutuhan: Inbound authenticated/signed events durable dan dedup per provider/event/revision; out-of-order state tidak rollback reservation terbaru.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-03.

### F10-AC-04

Kebutuhan: Stock movement external reserve/change/cancel idempoten transactional dengan nights/quantity; cancellation tidak release dua kali.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-04.

### F10-AC-05

Kebutuhan: Outbound updates outbox/retry/backoff dan per-resource ordering; provider outage memicu lag alert dan policy stop-sell/safety allocation approved.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-05.

### F10-AC-06

Kebutuhan: Scheduled reconciliation membandingkan authority snapshot/local ledger; drift punya report/version/reason dan review sebelum repair.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-06.

### F10-AC-07

Kebutuhan: Metrics lag/failure/conflict/unmapped tanpa PII; connector replay di sandbox membuktikan concurrent website/OTA behaviour.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F10-FR-07.

## Skenario review wajib

- F10-SC-1: Duplicate/out-of-order OTA reservation/cancellation.
- F10-SC-2: Channel down menyebabkan approved fallback bukan stock bebas.
- F10-SC-3: Mapping missing dikarantina tanpa stock mutation.
- F10-SC-4: Reconciliation drift repair audited dan idempotent.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-2026-10-03.md); [F09 Manajemen tarif, paket dan promo](09-rate-plan-and-promo-management-2026-10-03.md); [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Provider, stock authority, polling/webhook capability, stop-sell threshold, mapping dan conflicts runbook.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

