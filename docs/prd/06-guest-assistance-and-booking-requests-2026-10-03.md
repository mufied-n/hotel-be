# PRD — Bantuan tamu, perubahan dan pembatalan

Dokumen teknis: [TECH-F06](../tech/06-guest-assistance-and-booking-requests-architecture-2026-10-03.md).

ID: PRD-F06-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: FE + BE support; reception/hotel policy. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F06](../srs/06-guest-assistance-and-booking-requests-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Observed snapshot: non-cancellable/non-modifiable/no-show 100%. Assistance workflow merupakan proposal, bukan self-service cancellation bebas.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G08, G13–G16, G22**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Tamu perlu menghubungi hotel terkait reservasi tanpa mengira request mengubah atau membatalkan booking otomatis.

Outcome: Special request/bantuan, request perubahan/pembatalan, response hotel dan approved exception workflow. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

owner guest: request; receptionist: triage; gm_admin: policy exception; finance: refund F14. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Detail → baca policy → kirim request → received → review → accepted/rejected/resolved.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Special request/bantuan, request perubahan/pembatalan, response hotel dan approved exception workflow.

Di luar scope: Room service/housekeeping ordering dan chatbot belum memiliki riset cukup; tidak dijadikan scope.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F06-AC-01

Kebutuhan: Request terkait booking authorize owner; jenis assistance/change/cancellation terikat snapshot policy dan allowed_actions.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-01.

### F06-AC-02

Kebutuhan: Non-refundable/non-modifiable tetap ditegakkan; bantuan exception boleh dikirim dengan copy bahwa belum approved.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-02.

### F06-AC-03

Kebutuhan: Requested dates/rates tidak langsung overwrite booking; perubahan membuat re-quote dan persetujuan delta harga/policy sebelum commit.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-03.

### F06-AC-04

Kebutuhan: Request/cancellation memakai idempotency; release stok hanya sekali setelah keputusan booking mutation valid.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-04.

### F06-AC-05

Kebutuhan: Staff response mempunyai actor/reason/timestamp; penerimaan ticket bukan confirmation/refund success.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-05.

### F06-AC-06

Kebutuhan: Free-text memiliki batas Unicode, sanitization dan minim PII; lampiran ditunda sampai storage/security kontrak tersedia.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-06.

### F06-AC-07

Kebutuhan: Status request dapat dibaca ulang dan delivery notice menggunakan F11; timeout/offline tidak mengirim request duplikat.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F06-FR-07.

## Skenario review wajib

- F06-SC-1: Non-refundable cancellation ditolak tanpa mengubah stock/payment.
- F06-SC-2: Request exception tidak auto-cancel.
- F06-SC-3: Approved modification rechecks inventory dan accepts price delta.
- F06-SC-4: Unauthorized/duplicate request dan staff response tercatat.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F03 Booking Saya dan detail reservasi privat](03-my-bookings-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md); [F14 Rekonsiliasi pembayaran, late payment dan refund](14-finance-reconciliation-and-refunds-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

SLA bantuan, exception authority, cancellation cutoff termasuk perbedaan 12/15 versus 14 WIB dalam dokumen lama, eligibility refund.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

