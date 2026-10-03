# PRD — Konfirmasi, bukti booking dan kalender

Dokumen teknis: [TECH-F04](../tech/04-booking-confirmation-artifacts-architecture-2026-10-03.md).

ID: PRD-F04-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: FE + BE artifacts; hotel review copy. Prioritas: P1 untuk alur tamu; gate keamanan/payment mengikuti prioritas gap.
Pasangan: [SRS-F04](../srs/04-booking-confirmation-artifacts-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Planned FE04: receipt/ICS optional. Konfirmasi vendor dan email belum diobservasi; fitur ini proposal.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G05, G08, G13, G16, G19**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Tamu butuh bukti yang bisa dibaca ulang dengan tanggal, reference dan status pembayaran yang benar.

Outcome: Konfirmasi HTML, PDF booking acknowledgement, ICS arrival/departure dan status notifikasi. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

owner guest: download sendiri; staff berizin: baca sesuai operasional; publik: tidak dapat mengunduh lewat UUID. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Confirmed verified → detail konfirmasi → unduh bukti/kalender → kontak hotel.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Konfirmasi HTML, PDF booking acknowledgement, ICS arrival/departure dan status notifikasi.

Di luar scope: Bukti booking bukan invoice fiskal, bukan bukti paid jika payment pending, dan bukan klaim email delivered.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F04-AC-01

Kebutuhan: Bukti dibuat dari booking/policy/Money snapshot yang persisted; hanya confirmed backend mendapat label confirmed hotel.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-01.

### F04-AC-02

Kebutuhan: HTML/PDF mencantumkan reference, stay, tiap kamar/bed/rate/breakfast, guest yang perlu, total, payment state, check-in/out timezone dan contact owner-approved.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-02.

### F04-AC-03

Kebutuhan: Download authorize owner setiap request atau signed URL terbatas/revocable; URL publik tanpa expiry dilarang; response no-store.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-03.

### F04-AC-04

Kebutuhan: ICS memakai UID stabil dan timezone Asia/Jakarta untuk arrival/departure; teks escapable dan akhir kalender tidak mengubah jumlah malam.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-04.

### F04-AC-05

Kebutuhan: Artefak memuat versi/generation time; perubahan atau pembatalan menandai artefak terbaru dan tidak menghapus histori audit.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-05.

### F04-AC-06

Kebutuhan: Dokumen demo selalu watermarked demo/no-reservation; PDF/ICS tidak dibuat dari status yang dipilih user sebagai bukti pembayaran nyata.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-06.

### F04-AC-07

Kebutuhan: Failure generation dapat retry idempoten; status email disajikan terpisah queued/sent/delivered/failed menurut evidence delivery.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F04-FR-07.

## Skenario review wajib

- F04-SC-1: PDF total sama dengan persisted quote dan bukan double included tax.
- F04-SC-2: PDF panjang/multi-room/font/Unicode dapat dibaca.
- F04-SC-3: ICS timezone dan UID stabil; cancelled tidak tetap active tanpa penjelasan.
- F04-SC-4: Unauthorized download serta retry generation aman.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F03 Booking Saya dan detail reservasi privat](03-my-bookings-2026-10-03.md); [F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Template owner, data pribadi minimum, language, receipt versus tax invoice, logo/font dan contact.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

