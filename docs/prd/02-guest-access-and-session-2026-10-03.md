# PRD — Akses tamu, verifikasi kepemilikan dan sesi

Dokumen teknis: [TECH-F02](../tech/02-guest-access-and-session-architecture-2026-10-03.md).

ID: PRD-F02-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE identity + FE; hotel menentukan channel. Prioritas: P1 untuk alur tamu; gate keamanan/payment mengikuti prioritas gap.
Pasangan: [SRS-F02](../srs/02-guest-access-and-session-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Proposed: login belum diobservasi pada vendor. Audit G13 mengharuskan ownership/session; frontend belum memiliki auth.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G13, G15**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Tamu perlu membuka kembali booking tanpa memperlihatkan data hanya karena mengetahui UUID/reference.

Outcome: Email OTP atau magic link sebagai pilihan yang belum diputuskan, verifikasi booking ownership, expiry/revoke/session recovery. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

anonymous: request challenge; verified guest: sesi dan booking yang terbukti miliknya; staff: tidak memakai sesi guest. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Masuk → kirim challenge → verifikasi → sesi → Booking Saya → logout/expiry.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Email OTP atau magic link sebagai pilihan yang belum diputuskan, verifikasi booking ownership, expiry/revoke/session recovery.

Di luar scope: Social login, membership/loyalty, password account dan WhatsApp provider bukan asumsi default.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F02-AC-01

Kebutuhan: Request challenge memberi respons generik untuk email terdaftar/tidak terdaftar; cooldown/rate limit per subject/device/network memakai angka configurable.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-01.

### F02-AC-02

Kebutuhan: Hotel memilih satu metode email OTP atau magic link; token random kuat disimpan hash, memiliki expiry dan one-time use; contoh OTP di dokumen hanya fixture.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-02.

### F02-AC-03

Kebutuhan: Verification bersifat atomic; replay/expired/invalid challenge ditolak. Link verification dipisahkan dari GET agar mail scanner tidak mengonsumsi token.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-03.

### F02-AC-04

Kebutuhan: Sesi menggunakan cookie HttpOnly/Secure/scope/SameSite yang ditetapkan lewat review; rotasi setelah verifikasi, idle/absolute timeout, logout dan revocation.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-04.

### F02-AC-05

Kebutuhan: Booking disambungkan melalui bukti email/guest identity terverifikasi dan aturan claim historis; tahu reference tidak cukup. Verified booker tidak otomatis memberi akses semua occupant.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-05.

### F02-AC-06

Kebutuhan: Unauthorized, expiry, revoke menghapus state privat FE; logout tab lain menginvalidasi akses server. Data privat no-store dan tanpa PII/token analytics.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-06.

### F02-AC-07

Kebutuhan: Reset booking draft tidak logout; expiry login mempertahankan hanya search non-PII dan meminta verifikasi ulang. CSRF protection untuk cookie mutations.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F02-FR-07.

## Skenario review wajib

- F02-SC-1: Email A tidak bisa claim/read B walaupun mengetahui reference.
- F02-SC-2: OTP/link replay dan verification serentak hanya satu berhasil.
- F02-SC-3: Expired/revoked session ditolak dan cache privat tidak bocor.
- F02-SC-4: Enumeration/cooldown/resend/screen-reader error diuji.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

[F12 Identitas staff, izin dan audit perubahan](12-staff-identity-permissions-and-audit-2026-10-03.md); [F11 Notifikasi booking, payment dan operasional](11-notification-delivery-2026-10-03.md)

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

OTP versus magic link, channel/provider, TTL, claim historis, retention dan akses booker versus penghuni harus ditetapkan hotel/security.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.


## Alignment dengan Batch D existing

[PRD Batch D](checkout-idempotency-payment-batch-d-2026-10-03.md) baru ditemukan selama penulisan dan dipertahankan. Owner menyebut special_requests/guest_phone/estimated_arrival_time, key1–64, raw-body hash, X-Guest-Token untuk private DTO dan domain initiated/success/failed/received_after_expiry.
Nested guest DTO, guest cookie, hold_expires_at atau payment_status pada contoh paket ini adalah target view/proposal extension; bukan penggantian kontrak approved. Adapter/migration dan keputusan C14 wajib sebelum integrasi. Status actual remediasi tidak disimpulkan dari approval atau working-tree backend yang sedang berubah.
