# Review ulang backend untuk integrasi frontend

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Status: source review selesai untuk cakupan yang dicatat; runtime/provider acceptance belum dijalankan ulang. Permintaan pengguna: temuan backend ditulis di docs/gap; review ini tidak mengubah implementasi backend.

## Kesimpulan berdasarkan source terbaru

Backend sudah bertambah dari baseline audit awal: katalog CRUD, pencarian rentang/kapasitas agregat, locked quote dengan pricing, consent pada quoted checkout, guest token, payment attempts, assignment retry, hold expiry, adapter Xendit/Resend, serta OTP/session, Booking Saya, receipt JSON dan kalender ICS sudah mempunyai implementasi. Endpoint guest auth yang belum ada pada awal pembacaan sekarang terdaftar di router dan GuestSvc sudah diwire di main. Temuan ketiadaan route tersebut tidak dipertahankan.

Namun status “seluruh roadmap selesai” belum didukung route dan invariants yang diperiksa. Ada celah autentikasi staf, idempotency konkuren, policy/consent bypass dan payment recovery. Kehadiran komponen atau laporan PASS tidak otomatis menutup defect source yang masih ada.

P0 berarti blokir exposure/deployment live yang terkait; P1 correctness atau kelengkapan fitur inti; P2 hardening/kontrak. Batas ini tidak melarang pengembangan FE dengan API yang sesuai dan lingkungan uji terisolasi. Recommendation dan acceptance adalah usulan remediasi, bukan perubahan kebijakan hotel yang sudah approved.

## Register temuan aktif

| ID | Prioritas | Temuan dan dokumen pemilik | Hubungan audit/roadmap |
|---|---|---|---|
| BE-R01 | P0 (RESOLVED 2026-10-03, login staf + sesi) | [Identitas staf masih dapat dipalsukan](08-security-and-guest-session-reaudit-2026-10-03.md) | BE-G14; F12 |
| BE-R02 | P1 (RESOLVED 2026-10-03, PublicDTO privacy & token protection) | [DTO publik masih mengandung permintaan khusus tamu](08-security-and-guest-session-reaudit-2026-10-03.md) | BE-G13; F02/F03 |
| BE-R03 | P1 (RESOLVED 2026-10-03, atomic OTP & attempt lockout) | [Challenge OTP dan penghitung percobaan belum atomik](08-security-and-guest-session-reaudit-2026-10-03.md) | F02; perluasan BE-G13/14 |
| BE-R04 | P1 (RESOLVED 2026-10-03, honest logout & secure cookie) | [Logout mengaku berhasil walau pencabutan sesi gagal](08-security-and-guest-session-reaudit-2026-10-03.md) | F02/F12 |
| BE-R05 | P1 (RESOLVED 2026-10-03, canonical email ownership & policy-driven allowed_actions) | [Ownership email dan allowed_actions belum konsisten dengan booking](08-security-and-guest-session-reaudit-2026-10-03.md) | F02/F03/F05/F06 |
| BE-R06 | P1 | **RESOLVED 2026-10-03** ([laporan](../../testing/e2e/report/2026-10-03-133600-checkout-integrity-e2e-report.md)) — [Create tanpa quote melewati consent dan snapshot harga lengkap](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G06/08; F01 |
| BE-R07 | P1 (RESOLVED 2026-10-03, catalog room capacity invariant on search/quote/checkout) | [Kapasitas search belum menjadi invariant checkout](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G03; F01/F08 |
| BE-R08 | P1 | **RESOLVED 2026-10-03** (klaim atomik; risiko residual crash setelah commit, lihat laporan) — [Idempotency checkout masih lookup-create-save](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G09; F01/F05 |
| BE-R09 | P1 (RESOLVED 2026-10-03, catalog CRUD rate sync & search pricing guard) | [Harga katalog CRUD tidak menjadi sumber rate engine](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G01/04/05; F08/F09 |
| BE-R10 | P1 (RESOLVED 2026-10-03, multi-room breakfast parity & child tiers) | [Makna num_guests menyebabkan biaya sarapan ambigu](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G04/05; F01/F09 |
| BE-R11 | P1 | [Quote hanya tersimpan pada memori satu instance](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G06; F01/F05 |
| BE-R12 | P1 (RESOLVED 2026-10-03, WIB timezone alignment) | [Deadline pembatalan memakai UTC ketika kebijakan menyebut WIB](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G08/19; F06/F13 |
| BE-R13 | P1 | [Gateway timeout dianggap gagal pasti dan ledger error diabaikan](10-payment-recovery-and-provider-reaudit-2026-10-03.md) | BE-G11/12; F05/F14 |
| BE-R14 | P1 (RESOLVED 2026-10-03, amount & ledger reconciliation) | [Webhook tidak mencocokkan nilai dan invoice dengan ledger](10-payment-recovery-and-provider-reaudit-2026-10-03.md) | BE-G10/11; F05/F14 |
| BE-R15 | P1 | [Link pembayaran tidak tersedia untuk recovery lintas sesi](10-payment-recovery-and-provider-reaudit-2026-10-03.md) | BE-G11/12; F03/F05 |
| BE-R16 | P0 (RESOLVED 2026-10-03, fail-fast production startup & readiness) | [Deployment production masih dapat memilih fake gateway dan log notifier](10-payment-recovery-and-provider-reaudit-2026-10-03.md) | BE-G10/16; F05/F11/F15 |
| BE-R17 | P1 | [OTP delivery tidak memiliki retry durable dan tetap mengaku terkirim](10-payment-recovery-and-provider-reaudit-2026-10-03.md) | BE-G16; F02/F11 |
| BE-R18 | P2 | [Boundary payload dan error API belum seragam](09-checkout-pricing-and-policy-reaudit-2026-10-03.md) | BE-G09/15; F01/F02 |

## Perbaikan nyata terhadap audit awal

| Audit lama | Perubahan yang terlihat | Status review ulang |
|---|---|---|
| BE-G01/G02 | Katalog tujuh varian, CRUD, horizon dan pencarian multi-malam tersedia | IMPLEMENTED IN PART; rate/catalog identity dan channel/maintenance masih terpisah |
| BE-G03 | Date/LOS/room-count dan kapasitas agregat search ditambahkan | RESOLVED 2026-10-03; kapasitas katalog ditegakkan konsisten di search, quote, create (BE-R07) |
| BE-G04/G05 | Room Only/Breakfast, promo 15%, breakdown, IDR integer rupiah tersedia | RESOLVED 2026-10-03; base price sync dari katalog CRUD (BE-R09) dan eliminasi multi-room breakfast double counting + child tiers (BE-R10) |
| BE-G06 | Quote 15 menit dan wiring engine→BookingSvc tersedia | PARTIAL; in-memory durability dan public fallback (BE-R06/R11) |
| BE-G07 | Phone, arrival dan request length validation tersedia | IMPLEMENTED fields; snapshot contact dan normalization masih perlu konsistensi |
| BE-G08 | Quoted create meminta terms/privacy; cancel memeriksa non-refundable/deadline | PARTIAL; quote-less bypass dan timezone (BE-R06/R12) |
| BE-G09 | Key/body hash dan serial replay/conflict tersedia | PARTIAL; concurrency/error recovery (BE-R08) |
| BE-G10 | Xendit adapter dan development gate fake-pay tersedia | PARTIAL; production fallback dan event matching (BE-R14/R16) |
| BE-G11/G12 | Ledger/hold expires_at/late payment check/sweep tersedia | PARTIAL; ambiguous outcome/recovery (BE-R13/R15) |
| BE-G13 | Token ownership untuk cancel dan masking sebagian field tersedia | PARTIAL; public free text dan staff impersonation (BE-R01/R02) |
| BE-G14 | Enforcer nil ditolak 503 | Fail-closed subproblem RESOLVED IN SOURCE; trusted staff authentication masih OPEN |
| BE-G15 | Limiter global dan ProblemDetails sebagian route tersedia | PARTIAL; payload bounds dan guest error contract (BE-R18) |
| BE-G16 | Resend adapter dan handler confirmed tersedia | PARTIAL; actual delivery/outbox acceptance serta OTP recovery belum dibuktikan |
| BE-G17 | Ordered locks, SKIP LOCKED dan transient assignment retry tersedia | Implementation present; laporan DB ada, tidak diulang pada review ini |
| BE-G18/G19 | Horizon, katalog dan tipe Money tersedia | PARTIAL; channel/maintenance/configuration contract belum lengkap |
| BE-G20 | Suite real-DB dan laporan Batch F tersedia | Evidence present; closure dibatasi lima skenario laporan, bukan seluruh roadmap |
| BE-G21/G22 | Log sweep, early checkout dan no-show logic ditambahkan | Source present; policy/timezone dan acceptance deployment tidak dianggap selesai otomatis |

Status tabel ini menggantikan asumsi “semua 22 gap masih belum dikerjakan” dalam snapshot audit awal. Detail audit lama tetap disimpan sebagai sejarah; bukan checklist current-release.

## Batas review dan provenance

Source yang ditinjau: cmd/server wiring, API/router/identity/idempotency/guest handlers, booking/rates/catalog, guest service/repository/model, payment/notifier adapters, transaction/query hygiene terkait, worker expiry, migrasi yang tersedia, serta source dan laporan pengujian. Hash lengkap file dalam cmd/internal/config/migrations/testing dicatat di [manifest baru](source-manifest-reaudit-2026-10-03.json); hash merupakan snapshot, bukan bukti seluruh file mendapat audit mendalam setara.

Backend berubah selama review. Manifest baru berbeda dari manifest audit awal; Referensi fungsi menjadi anchor utama karena baris berubah saat pengembangan berjalan. Source baru setelah waktu manifest membutuhkan delta review. Tidak ada eksploit/mutasi DB/provider/email dilakukan dalam review ini. Satu probe read-only pada localhost:18080: healthz 200, catalog endpoint 404. Itu menunjukkan service yang diakses belum memenuhi route source yang sekarang terdaftar, tetapi tidak menentukan root cause deployment tanpa build/config inspection lebih lanjut.

Lihat [cakupan roadmap dan verifikasi](11-roadmap-coverage-and-verification-reaudit-2026-10-03.md) untuk daftar API aktual, dependency FE dan cara menutup temuan dengan bukti yang tepat.
