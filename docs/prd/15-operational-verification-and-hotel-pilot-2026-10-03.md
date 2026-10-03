# PRD — Verifikasi operasional, release gate dan pilot hotel

Dokumen teknis: [TECH-F15](../tech/15-operational-verification-and-hotel-pilot-architecture-2026-10-03.md).

ID: PRD-F15-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: QA + ops + hotel pilot owner + BE/FE. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F15](../srs/15-operational-verification-and-hotel-pilot-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Audit G20: mocked coverage/build bukan bukti DB/provider/delivery. QA FE partial Chromium dan device NOT RUN; pilot belum dilaksanakan.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G20, G21; seluruh gate live G01–G22 sesuai scope**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Fitur yang terlihat lengkap belum membuktikan stock/payment integrity, perangkat nyata, recovery atau operasional staff.

Outcome: Coverage/test/evidence, concurrency/failure/replay, observability, backup/restore, rollout/rollback, limited pilot and support. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

QA: execute isolated scenarios; ops: drill/release; hotel owner: accept pilot; GM: go/no-go; guest: pilot scope approved. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Freeze contracts → test isolated DB/provider → failure/restore drills → device/staff walkthrough → pilot → evaluate → go/no-go.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Coverage/test/evidence, concurrency/failure/replay, observability, backup/restore, rollout/rollback, limited pilot and support.

Di luar scope: Tidak menetapkan tanggal live, throughput/SLO/RPO/RTO numerik tanpa workload/owner; docs/test green tidak dianggap approval produksi.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F15-AC-01

Kebutuhan: Matrix requirement/gap → named scenario → commands/environment/source hash → DB/provider assertions → result/cleanup; NOT RUN tidak pass.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-01.

### F15-AC-02

Kebutuhan: New/refined Go packages mencapai target table-driven coverage >=80% menurut AGENTS dan vet/race; real PG/Valkey tests tetap wajib terpisah.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-02.

### F15-AC-03

Kebutuhan: Concurrency last-room, missing-night rollback, assignment conflicts, expiry/paid race, duplicate event dan queue down/sweep diuji DB isolated.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-03.

### F15-AC-04

Kebutuhan: Provider payment/email/channel sandbox membuktikan idempotency/signatures/replay/outage; live credentials bukan test fixtures.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-04.

### F15-AC-05

Kebutuhan: Observe queue/outbox lag, payment unknown, stock drift, auth rejects, sweep health dan readiness; threshold/SLO ditetapkan setelah measurement.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-05.

### F15-AC-06

Kebutuhan: Backup/restore drill dan rollback deploy/migration/payment intent menyimpan evidence; rollback tidak menghapus successful charges/history.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-06.

### F15-AC-07

Kebutuhan: Pilot allowlist/scope/stock/day/support owner ditetapkan; actual Chrome Android/Safari iOS keyboard/accessibility QA; go/no-go hotel tercatat dengan outstanding risks.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F15-FR-07.

## Skenario review wajib

- F15-SC-1: Backup restore drill proves balances/reservations/outbox continuity.
- F15-SC-2: No overselling N clients and cleanup ledger proof.
- F15-SC-3: Real devices/session recovery/staff operations walkthrough.
- F15-SC-4: Go/no-go rejects absent provider evidence and rollback ownership.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F01 Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-2026-10-03.md); [F02 Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-2026-10-03.md); [F03 Booking Saya dan detail reservasi privat](03-my-bookings-2026-10-03.md); [F04 Konfirmasi, bukti booking dan kalender](04-booking-confirmation-artifacts-2026-10-03.md); [F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md); [F06 Bantuan tamu, perubahan dan pembatalan](06-guest-assistance-and-booking-requests-2026-10-03.md); [F07 Resepsionis, assignment kamar dan lifecycle menginap](07-front-desk-stay-operations-2026-10-03.md); [F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-2026-10-03.md); [F09 Manajemen tarif, paket dan promo](09-rate-plan-and-promo-management-2026-10-03.md); [F10 Sinkronisasi kanal dan pencegahan overselling](10-channel-inventory-synchronization-2026-10-03.md); [F11 Notifikasi booking, payment dan operasional](11-notification-delivery-2026-10-03.md); [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md); [F14 Rekonsiliasi pembayaran, late payment dan refund](14-finance-reconciliation-and-refunds-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Pilot dates/rooms/channel scope, numeric SLO/RPO/RTO/thresholds, support rota, deployment topology dan acceptable deferred risks.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

