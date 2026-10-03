# PRD — Notifikasi booking, payment dan operasional

Dokumen teknis: [TECH-F11](../tech/11-notification-delivery-architecture-2026-10-03.md).

ID: PRD-F11-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE workers + communications provider + hotel. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F11](../srs/11-notification-delivery-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Audit menyebut LogNotifier/event log, bukan pengiriman nyata. Email vendor belum diobservasi.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G16, G20, G21**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Booking dapat committed tetapi tamu/staff tidak menerima informasi atau sistem mengklaim delivery hanya dari log.

Outcome: Owner-reviewed templates, event dedup, email default proposal, staff alerts, delivery history/redrive. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

guest: receive own event; reception: queue sesuai role; staff: authorized resend; worker: scoped provider credentials. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Domain commit/outbox → delivery job → send → accepted/delivered/failed → retry/escalate.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Owner-reviewed templates, event dedup, email default proposal, staff alerts, delivery history/redrive.

Di luar scope: WhatsApp/SMS belum disepakati; marketing newsletter/consent tidak digabung transaksi.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F11-AC-01

Kebutuhan: Booking/payment/operational event dan outbox row ditulis dalam transaction yang sama; rollback domain tidak mengirim notification.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-01.

### F11-AC-02

Kebutuhan: Delivery unik per event/recipient/channel/template version; retry mempertahankan key dan tidak menggandakan pesan logis.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-02.

### F11-AC-03

Kebutuhan: Template memakai persisted policy/quote/status; confirmed dikirim hanya saat domain confirmed, bukan saat redirect provider.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-03.

### F11-AC-04

Kebutuhan: Provider accepted/sent/delivered berbeda; callback verified/replay-safe memperbarui delivery status terpisah booking.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-04.

### F11-AC-05

Kebutuhan: Retry bounded/backoff, dead-letter/escalation dan authenticated resend dengan reason; error delivery tidak rollback paid booking.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-05.

### F11-AC-06

Kebutuhan: Recipient/contact/version diverifikasi; PII/token tidak di log; access link expiry/revocation mengikuti F02.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-06.

### F11-AC-07

Kebutuhan: Ops alerts tentang inventory/payment unknown tidak membocorkan guest full details; user sees truthful notification state.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F11-FR-07.

## Skenario review wajib

- F11-SC-1: Outbox commit failure rollback dan tidak send.
- F11-SC-2: Duplicate worker/provider callback menghasilkan satu logical delivery.
- F11-SC-3: Queue/provider down recovery/redrive bounded.
- F11-SC-4: Invalid recipient bounce tidak mengubah confirmed/payment.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md). Bootstrap delivery email/challenge dapat dibangun lebih dahulu; template payment/confirmed diaktifkan setelah event F05 verified.

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Provider/channel, template/legal review, SLA, retry retention dan escalation recipients.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.
