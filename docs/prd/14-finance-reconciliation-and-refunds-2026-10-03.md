# PRD — Rekonsiliasi pembayaran, late payment dan refund

Dokumen teknis: [TECH-F14](../tech/14-finance-reconciliation-and-refunds-architecture-2026-10-03.md).

ID: PRD-F14-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: Finance + GM + BE payments. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F14](../srs/14-finance-reconciliation-and-refunds-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Audit payment persistence/late-payment gap; finance workflow merupakan proposal. Tidak ada bukti vendor refund behavior.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G10–G12, G14, G20**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Pembayaran yang paid di provider tetapi unresolved di booking membutuhkan keputusan tanpa kehilangan dana atau duplicate refund.

Outcome: Payment ledger/provider reconciliation, amount mismatches, late payment, approved refund, durable retry dan audit. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

finance: investigate/propose refund; gm_admin: approve exception sesuai policy; guest: read result sendiri; reception: limited read. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Import provider settlement → match ledger → mismatch case → investigate → decision → refund/reconcile → resolved.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Payment ledger/provider reconciliation, amount mismatches, late payment, approved refund, durable retry dan audit.

Di luar scope: Accounting general ledger, fiscal invoice, bank payout automation dan automatic late-confirm bukan scope.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F14-AC-01

Kebutuhan: Ledger menyimpan attempts/verified events/amount currency exponent/provider IDs; paid/refund states tidak disimpulkan dari FE.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-01.

### F14-AC-02

Kebutuhan: Reconciliation run matching mengklasifikasi matched/provider_paid_local_pending/amount_mismatch/duplicate/late; input file/source provenance recorded.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-02.

### F14-AC-03

Kebutuhan: Late paid setelah stock release membentuk case; confirmation baru membutuhkan inventory/policy transaction, atau refund/manual resolution approved.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-03.

### F14-AC-04

Kebutuhan: Refund memerlukan eligible policy/case, remaining refundable amount, reason, approver sesuai matrix dan expected version.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-04.

### F14-AC-05

Kebutuhan: Refund intent persisted before external call; idempotency provider key dan lookup ambiguous response mencegah duplicate money movement.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-05.

### F14-AC-06

Kebutuhan: Sum successful + pending refunds <= captured amount; parallel full/partial refunds serialized. Provider fee/penalty tidak ditebak.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-06.

### F14-AC-07

Kebutuhan: Case resolution tidak menutupi mismatch tanpa evidence; report reconciliation/audit permissioned dan PII minimal; notification truthfully F11.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F14-FR-07.

## Skenario review wajib

- F14-SC-1: Paid provider local pending dan wrong currency bukan auto-confirm.
- F14-SC-2: Refund timeout replay memanggil intent sama.
- F14-SC-3: Parallel partial/full refund tidak over-refund.
- F14-SC-4: Late paid booking expired tidak consume stock tanpa policy/inventory recheck.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F05 Hosted payment, hold dan pemulihan pembayaran](05-payment-hold-and-recovery-2026-10-03.md); [F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F13 Konfigurasi hotel dan publikasi kebijakan](13-hotel-policy-and-configuration-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Refund eligibility/penalty/fees, approval threshold, settlement format/provider, SLA late cases dan refund timing.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.


## Alignment dengan Batch D existing

[PRD Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
