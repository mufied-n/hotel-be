# PRD — Konfigurasi hotel dan publikasi kebijakan

Dokumen teknis: [TECH-F13](../tech/13-hotel-policy-and-configuration-architecture-2026-10-03.md).

ID: PRD-F13-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Hotel GM/ops + BE config. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F13](../srs/13-hotel-policy-and-configuration-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Official observed check-in15/check-out12 dan policy snapshot; Batch C memiliki cutoff fleksibel14WIB. Aturan final harus owner-reviewed, tanpa legal tax inference.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G08, G19, G21, G22**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Tarif, checkout dan staff dapat memakai aturan berbeda bila konfigurasi/historical policy tidak versioned.

Outcome: Hotel metadata/contact/timezone, operating windows, stay/occupancy limits, policy version, exception permissions dan integration flags. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

gm_admin: approve/publish; revenue/ops: propose sesuai permission; guest/staff: read relevant published snapshot. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Draft config/policy → validation/review → schedule publish → quote snapshot → historical enforcement.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Hotel metadata/contact/timezone, operating windows, stay/occupancy limits, policy version, exception permissions dan integration flags.

Di luar scope: Tidak mengubah histori booking dengan konfigurasi terbaru; fiscal rates butuh sumber/owner finance, bukan asumsi legal.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F13-AC-01

Kebutuhan: Metadata address/contact/check-in/out/timezone dari owner; proposal contact tidak dipublikasikan sebagai verified tanpa provenance.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-01.

### F13-AC-02

Kebutuhan: Policy object menyimpan terms/privacy/cancellation/no-show/early-checkout/rate association dan effective window/version.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-02.

### F13-AC-03

Kebutuhan: Draft publish membutuhkan approval authority, expected version dan audit; scheduled overlap/invalid timezone/limits ditolak.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-03.

### F13-AC-04

Kebutuhan: Quote dan booking menyimpan policy/version immutable; enforcement existing booking pakai versi saat consent, bukan current config.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-04.

### F13-AC-05

Kebutuhan: Runtime limits/body/search/hold/quote TTL memiliki validation dan owner; quote TTL tidak sama dengan hold/payment deadline.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-05.

### F13-AC-06

Kebutuhan: Cancel/refund exceptions mencatat reason/approver dan permission spesifik; emergency override tidak membuka route publik.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-06.

### F13-AC-07

Kebutuhan: Roll back publication lewat versi baru effective-forward; source flags fake gateway/dev scenario tidak boleh enabled di production.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F13-FR-07.

## Skenario review wajib

- F13-SC-1: Concurrent publish version conflict tanpa lost update.
- F13-SC-2: Future publication dan Asia/Jakarta cutoff boundary.
- F13-SC-3: Update policy tidak mengubah booking lama.
- F13-SC-4: Invalid contact/window/TTL dan dev flag production ditolak.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Owner policies, cutoff konflik, exceptions, legal privacy/cancellation copy dan publish workflow.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

