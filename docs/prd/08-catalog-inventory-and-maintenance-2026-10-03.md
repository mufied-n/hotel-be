# PRD — Katalog, stok harian dan kamar maintenance

Dokumen teknis: [TECH-F08](../tech/08-catalog-inventory-and-maintenance-architecture-2026-10-03.md).

ID: PRD-F08-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Revenue/operations + BE catalog/inventory. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F08](../srs/08-catalog-inventory-and-maintenance-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Observed: 5 keluarga/7 varian dan official copy 95 kamar, bukan alokasi sellable. Existing katalog CRUD menjadi owner, daftar seed codes tidak diasumsikan cocok dengan vendor.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G01–G03, G17–G20**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Stok seed dan kode varian belum cukup menjadi sumber sellability hotel lintas tanggal/occupancy.

Outcome: Official variant mapping, photos/capacity provenance, date horizon/allotment, physical rooms, maintenance and readiness. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

guest: read sellable catalog; revenue_mgr: katalog/allotment; gm_admin: review; housekeeping: readiness sesuai izin. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Owner mapping → publish catalog → allocate date inventory → maintenance block → search/hold → reconcile.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Official variant mapping, photos/capacity provenance, date horizon/allotment, physical rooms, maintenance and readiness.

Di luar scope: Tidak mengganti catalog CRUD approved; channel connectivity dimiliki F10; total 95 bukan available count otomatis.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F08-AC-01

Kebutuhan: Catalog family/variant/bed/media/capacity di-owner review; SKU/inventory mapping immutable untuk historic booking.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-01.

### F08-AC-02

Kebutuhan: Search memeriksa seluruh nights [check_in,check_out), quantity dan occupancy/children entitlement; missing inventory error terpisah sold-out.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-02.

### F08-AC-03

Kebutuhan: Inventory records membedakan physical capacity, sellable allocation, holds, confirmed reservations, maintenance dan channel allocation; hindari double subtraction.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-03.

### F08-AC-04

Kebutuhan: Maintenance block punya room/date/reason/actor/version; konflik booking existing ditampilkan dan butuh resolution, bukan silent removal.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-04.

### F08-AC-05

Kebutuhan: Publish horizon/tariff coverage tervalidasi; tanggal di luar horizon memiliki guidance. Nonaktif variant tidak menghapus history.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-05.

### F08-AC-06

Kebutuhan: Hold decrement/release atomic, non-negative; repair inventory lewat adjustment ledger audited dengan expected version.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-06.

### F08-AC-07

Kebutuhan: Physical room continuous stay dan alternate assignment diuji PostgreSQL nyata; foto memakai asset approved/local dengan alt.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F08-FR-07.

## Skenario review wajib

- F08-SC-1: Missing satu night tidak menampilkan available dan tidak decrement parsial.
- F08-SC-2: N concurrent request last room <= allocation successes.
- F08-SC-3: Maintenance overlap confirmed memerlukan resolution.
- F08-SC-4: Mapping seed vs 7 official variants direview owner.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Stock per variant, physical mapping, maintenance workflow, child capacities, horizon, readiness authority.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

