# PRD — Manajemen tarif, paket dan promo

Dokumen teknis: [TECH-F09](../tech/09-rate-plan-and-promo-management-architecture-2026-10-03.md).

ID: PRD-F09-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Revenue manager + BE rates; Batch C owner. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F09](../srs/09-rate-plan-and-promo-management-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Observed: deluxe RO/Breakfast, suite Breakfast, snapshot OCTOBREAK 27% dan included charges. Existing Batch C approved berbeda: 15%, exponent0, 15min. Tidak ditimpa paket baru.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G04–G06, G08, G19**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Harga/promosi harus dapat diatur hotel tanpa kode hardcoded atau biaya sarapan/tax ditambahkan dua kali.

Outcome: Per-variant plan availability, benefit/breakfast entitlement, seasonal rates, promo eligibility/window/quota, owner Money contract. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

revenue_mgr: draft/publish tarif; gm_admin: approval sesuai policy; guest: quote read. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Draft plan/rate calendar → validate → publish → search quote → immutable booking snapshot.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Per-variant plan availability, benefit/breakfast entitlement, seasonal rates, promo eligibility/window/quota, owner Money contract.

Di luar scope: Dynamic AI revenue optimisation dan multi-currency conversion tidak termasuk; pajak hukum tidak ditetapkan dokumen ini.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F09-AC-01

Kebutuhan: Availability rate plan ditentukan per variant/date/occupancy; suite snapshot tidak menerima RO atau surcharge deluxe universal.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-01.

### F09-AC-02

Kebutuhan: Rate calendar menggunakan Money integer currency/exponent, valid dates/version; quote engine mengikuti owner Batch C sampai amendment approved.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-02.

### F09-AC-03

Kebutuhan: Promo memakai window Asia/Jakarta, eligible variant/rate/min stay/occupancy/quota dan rejection codes yang jelas.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-03.

### F09-AC-04

Kebutuhan: Discount/tax/service ordering, included versus added, rounding tiap stage dan breakfast count disetujui owner; sum breakdown tepat sama payable.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-04.

### F09-AC-05

Kebutuhan: Quote memiliki ID/version/expiry/input/policy snapshot; price change butuh review baru, tidak silent surcharge pada create.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-05.

### F09-AC-06

Kebutuhan: Update tarif hanya future quote; existing valid quote behavior mengikuti lock policy. Promo quota committed atomically saat booking accepted, release rules eksplisit.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-06.

### F09-AC-07

Kebutuhan: Publishing dan concurrent edits membutuhkan actor/version/reason; snapshot vendor diberi provenance/date dan bukan tariff permanen.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F09-FR-07.

## Skenario review wajib

- F09-SC-1: Suite quote benar; RO tidak tersedia ditolak.
- F09-SC-2: Promo boundary timezone/expired/quota replay serentak.
- F09-SC-3: Included fees tidak ditambah lagi dan integer rounding golden cases.
- F09-SC-4: Tariff update tidak mengubah persisted booking total.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F08 Katalog, stok harian dan kamar maintenance](08-catalog-inventory-and-maintenance-2026-10-03.md); [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Conflict register C01–C06 wajib resolve sebelum adapter live; tarif, benefit, breakfast dan tax owner approval.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

