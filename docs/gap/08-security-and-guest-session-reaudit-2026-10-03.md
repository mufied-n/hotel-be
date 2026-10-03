# Review ulang keamanan, akses tamu dan sesi

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Basis: source snapshot pada manifest review ulang. Temuan adalah hasil inspeksi source; reproduksi race/fault dan acceptance tests di bawah merupakan pekerjaan verifikasi yang masih diperlukan. Kode backend tidak diubah oleh review ini.

Dokumen ini memperbarui konteks audit lama, tanpa menganggap semua BE-G tetap OPEN atau seluruh F01–F15 sudah selesai. [Register terkini](07-backend-reaudit-2026-10-03.md) menjelaskan status dan batas bukti.

## BE-R01 — Identitas staf masih dapat dipalsukan

**Prioritas:** P0. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G14; F12.

**Bukti source.** [middleware.go](../../internal/api/middleware.go) (lihat fungsi terkait) menganggap nama role atau `staff:role` sebagai Bearer terverifikasi. Baris 96 menerima `X-User-Role` karena kondisi `rawRole != ""` selalu benar di cabang itu. [router.go](../../internal/api/router.go), `getBooking`, mengembalikan agregat lengkap untuk role non-guest. Enforcer nil sekarang memang ditolak 503; perbaikan fail-closed tersebut tidak memperbaiki sumber identitas.

**Skenario pemicu.** Caller tanpa akun mengirim role `gm_admin` atau Bearer `receptionist`. Casbin mengevaluasi role yang sudah dipalsukan dan dapat memberikan hak baca/mutasi sesuai policy role tersebut.

**Dampak.** Akses data tamu dan operasi staf dapat melewati autentikasi. ID staf juga berasal dari header dan tidak menjadi identitas tepercaya untuk audit.

**Rekomendasi.** Hapus parsing role sebagai kredensial dan bypass testing dari jalur deployment. Verifikasi session atau token bertanda tangan dari identity provider yang disetujui; validasi issuer, audience, expiry dan revocation. Jika identitas datang dari gateway internal, gateway wajib membersihkan header publik, backend wajib membatasi trust boundary dan memverifikasi kredensial internal. Role dan scope properti harus berasal dari principal server.

**Kriteria penerimaan dan verifikasi.** Uji semua role melalui header/Bearer palsu: ditolak tanpa baca PII atau perubahan DB. Token valid tetapi role/scope tidak sesuai tetap ditolak. Token expired/revoked serta enforcer unavailable fail-closed. Negative tests harus memakai middleware deployment yang sama, bukan mode bypass.

**Implikasi frontend.** Jangan menyediakan login staf berbasis pilihan role atau meneruskan header role pengguna. Menyembunyikan tombol tidak menutup akses langsung backend.

## BE-R02 — DTO publik masih mengandung permintaan khusus tamu

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, lihat [laporan E2E](../../testing/e2e/report/2026-10-03-202200-public-dto-privacy-r02-e2e-report.md)). **Hubungan:** BE-G13; F02/F03.

**Bukti source.** [booking.go](../../internal/booking/booking.go) (lihat fungsi terkait) memasukkan `special_requests` dan `estimated_arrival_time` pada `PublicDTO`. `getBooking` mengembalikan DTO tersebut dengan 200 ketika token tidak ada atau tidak cocok. Nama/email/telepon sudah disaring, sehingga temuan lama tidak seluruhnya tetap berlaku.

**Skenario pemicu.** Tamu menulis kondisi kesehatan, kebutuhan aksesibilitas atau kontak pada permintaan khusus; caller yang mengetahui ID membaca field itu tanpa token.

**Dampak.** Free text dapat berisi informasi pribadi. Menghapus field nama/email saja tidak cukup untuk membentuk response publik minimal.

**Rekomendasi.** Jadikan detail booking privat dan tolak ownership yang gagal dengan response generik konsisten. Jika status publik tetap diperlukan, buat DTO khusus tanpa free text, arrival detail, token atau metadata sensitif. Jangan mengembalikan token akses pada read biasa untuk staf.

**Kriteria penerimaan dan verifikasi.** Isi permintaan khusus dengan marker sensitif; request tanpa token, token salah dan token booking lain tidak boleh mendapatkan marker tersebut. Token valid tetap mendapat detail yang diperlukan. Verifikasi response, cache dan log redaction.

**Implikasi frontend.** Jangan menganggap HTTP 200 pada endpoint lama sebagai bukti ownership. Booking reference/UUID bukan kredensial.

## BE-R03 — Challenge OTP dan penghitung percobaan belum atomik

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, lihat [laporan](../../testing/e2e/report/2026-10-03-204500-atomic-otp-challenges-r03-e2e-report.md)). **Hubungan:** F02; perluasan BE-G13/14.

**Bukti source.** [guest/service.go](../../internal/guest/service.go) (lihat fungsi terkait) melakukan cooldown read sebelum insert;  pada bagian terkait membaca challenge, membandingkan hash, memperbarui attempts/verified dan membuat sesi melalui operasi terpisah. Error update attempts dan mark verified diabaikan. [guest/postgres.go](../../internal/guest/postgres.go) (lihat fungsi terkait) tidak mengunci challenge atau memakai compare-and-set konsumsi.

**Skenario pemicu.** Dua verify dengan OTP valid yang sama membaca challenge belum dipakai lalu masing-masing membuat sesi. Percobaan salah paralel membaca angka attempts yang sama dan menulis nilai yang sama; limit efektif dapat terlampaui. Challenge paralel juga dapat melewati cooldown.

**Dampak.** Single-use OTP, batas percobaan dan cooldown belum dijamin oleh persistence. Ini analisis interleaving source, belum reproduksi race runtime dalam review ini.

**Rekomendasi.** Lakukan consume, increment attempts dan create-session dalam transaksi dengan row lock atau conditional update yang memeriksa expiry/verified/attempt limit. Pastikan satu challenge aktif dan cooldown per email dijaga atomik. Propagasikan kegagalan persistence; jangan menerbitkan sesi saat consume gagal. Tambahkan limit request per sumber di luar cooldown email.

**Kriteria penerimaan dan verifikasi.** Real-DB test: 20 verify serentak dengan satu OTP menghasilkan tepat satu konsumsi/sesi; percobaan salah paralel tidak melampaui batas efektif; dua challenge paralel memenuhi cooldown. Inject kegagalan consume/session insert, buktikan rollback dan tidak ada token sukses. Tidak ada skenario SKIP dianggap PASS.

**Implikasi frontend.** Disable-submit dan countdown membantu UX, tetapi backend tetap harus melindungi request paralel maupun lintas perangkat.

## BE-R04 — Logout mengaku berhasil walau pencabutan sesi gagal

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, lihat [laporan](../../testing/e2e/report/2026-10-03-205100-guest-session-logout-hardening-r04-e2e-report.md)). **Hubungan:** F02/F12.

**Bukti source.** [guest_auth.go](../../internal/api/guest_auth.go), `handleGuestLogout`, mengabaikan error `RevokeSession`, menghapus cookie lalu mengembalikan sukses. [guest/service.go](../../internal/guest/service.go) (lihat fungsi terkait) mengabaikan `TouchSession` lalu mengembalikan expiry baru seolah tersimpan. Handler verify memasang HttpOnly/SameSite cookie tetapi tanpa `Secure`, serta mengirim raw token dalam JSON.

**Skenario pemicu.** DELETE sesi gagal di DB ketika logout. Cookie lokal hilang tetapi token lama masih bisa dipakai lewat header. Touch expiry gagal sehingga response profil dan state DB berbeda.

**Dampak.** Logout belum menjamin revocation, dan lifecycle sesi berbeda antara browser dan server. Token JSON menambah tempat yang harus dijaga oleh client.

**Rekomendasi.** Tangani error revoke/touch dengan hasil yang jujur dan retry yang aman. Terapkan Secure untuk deployment HTTPS berdasarkan konfigurasi trusted, no-store untuk response privat, dan origin/CSRF protection pada mutasi berbasis cookie. Tentukan jalur token untuk BFF/native secara eksplisit; webapp tidak perlu menyimpan raw token di localStorage.

**Kriteria penerimaan dan verifikasi.** Inject kegagalan revoke: endpoint tidak menyatakan sesi sudah dicabut; setelah logout sukses replay token ditolak. Touch gagal tidak memalsukan expiry. Verifikasi cookie HTTPS, cache headers, request origin asing dan log tanpa token. Uji idle 24 jam dan absolute 7 hari dengan clock terkontrol.

**Implikasi frontend.** Gunakan cookie HttpOnly pada BFF dan normalisasi error sesi. Status logout lokal harus dibedakan dari server revocation bila server gagal.

## BE-R05 — Ownership email dan allowed_actions belum konsisten dengan booking

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03). **Hubungan:** F02/F03/F05/F06.

**Bukti source.** [guest/service.go](../../internal/guest/service.go) menormalisasi email menjadi lowercase; [guest/postgres.go](../../internal/guest/postgres.go), `ListBookingsByEmail`/`GetBookingDetailByEmail`, memakai equality `guest_email = $1`. [booking/service.go](../../internal/booking/service.go) (lihat fungsi terkait) menyimpan email input tanpa normalisasi. `computeAllowedActions` pada guest service hanya melihat status, sementara `Cancel` memeriksa policy dan deadline.

**Resolusi (2026-10-03):** Terselesaikan melalui normalisasi kanonikal email di `booking.Service.Create` (`strings.ToLower(strings.TrimSpace(email))`), kueri PostgreSQL case-insensitive `WHERE LOWER(TRIM(b.guest_email)) = $1` pada seluruh fungsi pencarian tamu (`CountActiveBookingsByEmail`, `ListBookingsByEmail`, `GetBookingDetailByEmail`, `GetBookingReceiptData`), serta komputasi dinamis `computeAllowedActions` yang mengintegrasikan `booking.FreeCancellationDeadline` (WIB cutoff H-2 14:00 WIB), kebijakan non-refundable, dan validasi hold `expires_at`. Terverifikasi 100% via unit tests (coverage 90.1%) dan E2E test script (`testing/e2e/script/booking_ownership_actions_consistency_r05_e2e.sh`).

**Skenario pemicu.** Booking dibuat dengan `Guest@Example.com`, login OTP memakai `guest@example.com`, lalu booking tidak muncul. Booking confirmed non-refundable mendapat `can_cancel:true`; pending mendapat `can_pay:true` tanpa jaminan link pembayaran tersedia.

**Dampak.** Booking milik tamu dapat hilang dari daftar dan UI menampilkan aksi yang pasti ditolak atau tidak bisa dijalankan.

**Rekomendasi.** Tetapkan canonical email yang sama pada create/challenge/lookup serta strategi backfill history dengan review collision; jangan mengubah ownership hanya lewat klaim email di client. Turunkan allowed_actions dari policy, deadline, hak akses dan kapabilitas payment/artifact yang tersedia. Definisikan pagination nyata agar booking setelah limit pertama dapat diakses.

**Kriteria penerimaan dan verifikasi.** Uji create dengan case/whitespace lalu OTP canonical menemukan booking yang sama. Non-owner tetap 404. Non-refundable/deadline lewat tidak menawarkan cancel, pending tanpa recovery URL tidak menawarkan pay yang buntu. Koleksi lebih dari 100 booking dapat ditelusuri tanpa duplikasi/hilang melalui kontrak pagination yang disepakati.

**Implikasi frontend.** Jangan mengganti email ownership dari query browser. Tangani error mutation meskipun allowed_actions mengizinkan; state bisa berubah setelah render.
