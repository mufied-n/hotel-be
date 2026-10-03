# PRD — Hosted payment, hold dan pemulihan pembayaran

Dokumen teknis: [TECH-F05](../tech/05-payment-hold-and-recovery-architecture-2026-10-03.md).

ID: PRD-F05-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE payments + provider integration + FE. Prioritas: P1 untuk alur tamu; gate keamanan/payment mengikuti prioritas gap.
Pasangan: [SRS-F05](../srs/05-payment-hold-and-recovery-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Observed: pay-now copy. Payment vendor tidak diobservasi. Audit mencatat fake gateway dan deadline/recovery gap.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G09–G12, G20**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Timeout dan return gateway ambigu dapat membuat duplicate charge atau confirmed yang tidak didukung pembayaran.

Outcome: Persisted attempts, authoritative deadline, hosted payment, verified events, bounded status recovery. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

owner guest: attempt/status miliknya; provider: verified webhook; finance: recovery F14. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Submit → booking/hold → payment attempt → hosted gateway → processing → paid/failed/expired/assistance.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Persisted attempts, authoritative deadline, hosted payment, verified events, bounded status recovery.

Di luar scope: Tidak mengumpulkan PAN/CVV dalam aplikasi; provider dan refund policy belum dipilih.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F05-AC-01

Kebutuhan: Create booking idempoten mencatat quote/hold/outbox secara atomic; call provider di luar transaksi DB dengan durable recovery intent.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-01.

### F05-AC-02

Kebutuhan: Payment attempt memiliki ID/provider reference/amount/currency/exponent/status/deadline; attempt baru hanya setelah status lama terminal dan policy mengizinkan.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-02.

### F05-AC-03

Kebutuhan: Retry provider timeout mencari attempt existing; jangan menganggap timeout sebagai gagal atau charge ulang.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-03.

### F05-AC-04

Kebutuhan: Return URL hanya meminta check status; query success tidak mengubah domain. Webhook memverifikasi signature/timestamp/replay memakai kontrak provider.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-04.

### F05-AC-05

Kebutuhan: Paid confirmation mencocokkan amount/currency/booking/provider reference; duplicate/out-of-order event aman. Payment dan hold expiry diserialisasi.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-05.

### F05-AC-06

Kebutuhan: Server time/expiry menjadi authority; timer FE menggunakan elapsed nyata, visibility refresh, cleanup, bounded poll/backoff dan aksi retry status.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-06.

### F05-AC-07

Kebutuhan: Late payment dialihkan assistance/reconciliation sesuai policy; tidak otomatis acquire stok yang sudah dilepas. Fake-pay dinonaktifkan di production.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F05-FR-07.

## Skenario review wajib

- F05-SC-1: Duplicate submit/webhook hanya satu charge intent/booking confirmation.
- F05-SC-2: Provider timeout dan return tampered tidak auto-confirm.
- F05-SC-3: Expiry versus paid race membuktikan stok dan status final benar.
- F05-SC-4: Offline/unmount/hidden tab tidak menyebabkan poll leak atau charge baru.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F01 Kelengkapan alur booking dan reset sesi](01-booking-journey-completion-2026-10-03.md); [F02 Akses tamu, verifikasi kepemilikan dan sesi](02-guest-access-and-session-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Provider, signing protocol, hold TTL, attempt retry window, late payment dan refund handling.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.


## Alignment dengan Batch D existing

[PRD Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
