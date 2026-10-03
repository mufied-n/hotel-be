# PRD — Kelengkapan alur booking dan reset sesi

Dokumen teknis: [TECH-F01](../tech/01-booking-journey-completion-architecture-2026-10-03.md).

ID: PRD-F01-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: FE + BE booking; keputusan katalog/rate oleh hotel. Prioritas: P1 untuk alur tamu; gate keamanan/payment mengikuti prioritas gap.
Pasangan: [SRS-F01](../srs/01-booking-journey-completion-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Observed: tanggal, occupancy, room/rate, harga included dan review vendor. Source FE: promo belum mengubah harga, tombol lanjut simulasi belum beraksi, reset draft belum terhubung. Audit backend bukan verifikasi source terbaru.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G02–G09, G12, G15**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Pengguna dapat menuntaskan happy path tetapi beberapa aksi belum punya efek dan draft lama bisa ikut booking baru.

Outcome: Search codec, occupancy per kamar, promo/error, quote snapshot, invalidation, consent, submit replay, reset dan guard refresh. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

guest: search/review/create miliknya; staff: tanpa akses mutation dari shell guest. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Cari → pilih setiap kamar/rate → detail tamu → review/consent → submit → status → mulai booking baru.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Search codec, occupancy per kamar, promo/error, quote snapshot, invalidation, consent, submit replay, reset dan guard refresh.

Di luar scope: Login tamu dimiliki F02; provider payment F05; tanpa landing atau marketing pages.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F01-AC-01

Kebutuhan: Search memvalidasi DateOnly, checkout > check-in, panjang stay/horizon dan room/child limits dari konfigurasi; hari checkout tidak menambah malam.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-01.

### F01-AC-02

Kebutuhan: Setiap room_index memiliki variant/rate/occupancy; semua pilihan lengkap sebelum quote agregat. Rate yang tidak tersedia untuk variant tidak ditampilkan atau diterima.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-02.

### F01-AC-03

Kebutuhan: Promo diterapkan server quote dengan valid/invalid/expired/ineligible states; total seluruh stay/rooms menjadi harga utama sebelum select.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-03.

### F01-AC-04

Kebutuhan: Edit tanggal, occupancy, variant, rate, promo/currency membatalkan quote/consent/attempt lama; edit guest tidak mengubah harga. Response async lama tidak mengganti search terbaru.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-04.

### F01-AC-05

Kebutuhan: Review memakai snapshot harga/policy, guest, special request Unicode <=500 code points; terms/privacy terpisah unchecked dan tersimpan version/timestamp pada submit.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-05.

### F01-AC-06

Kebutuhan: Create memakai key stabil dan fingerprint payload; retry ambigu mempertahankan attempt. Key sama payload berbeda menghasilkan 409.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-06.

### F01-AC-07

Kebutuhan: Mulai booking baru membersihkan draft/guest/consent/attempt; histori terotorisasi F03 tetap ada. Refresh tanpa draft menawarkan recovery, bukan PII URL/storage.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F01-FR-07.

## Skenario review wajib

- F01-SC-1: Single/multi-room dengan children konsisten dari results sampai status.
- F01-SC-2: Promo invalid tidak memberi diskon; suite hanya rate owner yang sah.
- F01-SC-3: Back/edit menghapus stale quote; reset menghapus semua PII draft.
- F01-SC-4: Double submit/network ambiguity menghasilkan satu booking dan satu hold.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

Tidak ada fitur prerequisite; keputusan identity/hotel dapat membatasi integrasi.

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Mapping varian/backend IDs, batas anak/stay, total per item, dan perbedaan Money/quote TTL harus diselaraskan dengan katalog/Batch C.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.


## Alignment dengan Batch D existing

[PRD Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
