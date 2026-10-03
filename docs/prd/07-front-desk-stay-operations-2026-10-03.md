# PRD — Resepsionis, assignment kamar dan lifecycle menginap

Dokumen teknis: [TECH-F07](../tech/07-front-desk-stay-operations-architecture-2026-10-03.md).

ID: PRD-F07-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Reception + BE operations + QA. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F07](../srs/07-front-desk-stay-operations-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Target internal dan audit source: assignment/check-in/out/no-show tersedia sebagian; belum bukti real-DB concurrency.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G14, G17, G20, G22**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Staff membutuhkan kerja reservasi harian tanpa double assignment atau state check-in/no-show yang salah.

Outcome: Arrival/in-house/departure, continuous room stay, multi-room assignment, housekeeping readiness read dan reasoned exception. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

receptionist: assignment/check-in/out; housekeeping: read kesiapan yang disepakati; gm_admin: exceptions audited. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Daftar arrival → verified reservation → assign physical rooms → check-in → in-house → check-out/no-show sesuai policy.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Arrival/in-house/departure, continuous room stay, multi-room assignment, housekeeping readiness read dan reasoned exception.

Di luar scope: Self check-in, key-card issuance, housekeeping task scheduling dan POS bukan scope.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F07-AC-01

Kebutuhan: Daftar staff memakai verified identity/role dan filter tanggal/property/status sebelum pagination; private PII hanya sesuai duty.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-01.

### F07-AC-02

Kebutuhan: Assignment multi-room memastikan physical room tersedia seluruh nights, sellable variant sesuai dan maintenance/readiness sah.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-02.

### F07-AC-03

Kebutuhan: Assignment paralel memakai transactional conflict protection; retry memilih room alternatif tanpa partial assignments.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-03.

### F07-AC-04

Kebutuhan: Check-in hanya confirmed reservation pada window policy dan payment conditions; future check-in ditolak kecuali exception resmi.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-04.

### F07-AC-05

Kebutuhan: Check-out early/no-show hanya pada batas policy yang disetujui; inventory release dan charges ditentukan policy snapshot, tidak FE.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-05.

### F07-AC-06

Kebutuhan: Repeat command idempoten; optimistic version/lock mencegah check-in/cancel/refund race; actor/reason audit wajib.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-06.

### F07-AC-07

Kebutuhan: Operational status dan payment status terpisah; booking eligible ditentukan backend, bukan header role/public guest.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F07-FR-07.

## Skenario review wajib

- F07-SC-1: Dua receptionist assign bersamaan tidak overlap room nights.
- F07-SC-2: Multi-room gagal satu assignment rollback semua.
- F07-SC-3: Future check-in/no-show terlalu awal ditolak.
- F07-SC-4: Early checkout dan cancellation race stock konsisten.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-2026-10-03.md); [F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Continuous room obligation, split-stay permission, readiness owner, early-checkout/no-show windows, exceptions.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

