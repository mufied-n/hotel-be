# Review ulang pembayaran, recovery dan provider

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Basis: source snapshot pada manifest review ulang. Temuan adalah hasil inspeksi source; reproduksi race/fault dan acceptance tests di bawah merupakan pekerjaan verifikasi yang masih diperlukan. Kode backend tidak diubah oleh review ini.

Dokumen ini memperbarui konteks audit lama, tanpa menganggap semua BE-G tetap OPEN atau seluruh F01–F15 sudah selesai. [Register terkini](07-backend-reaudit-2026-10-03.md) menjelaskan status dan batas bukti.

## BE-R13 — Gateway timeout dianggap gagal pasti dan ledger error diabaikan

**Prioritas:** P1. **Status:** RESOLVED 2026-10-03 (gateway timeout classification, hold preservation, definitive failure compensation, UpdateAttemptByID, unswallowed ledger error logging). **Hubungan:** BE-G11/12; F05/F14.

**Bukti source.** [booking/service.go](../../internal/booking/service.go) (lihat fungsi terkait) memanggil gateway setelah commit. Semua error diperlakukan failed lalu Cancel dipanggil dengan error diabaikan. RecordAttempt/UpdateAttemptStatus errors diabaikan pada Create/Confirm. [booking/postgres.go](../../internal/booking/postgres.go) (lihat fungsi terkait) memperbarui attempt berdasarkan booking_id, bukan attempt tertentu.

**Skenario pemicu.** Provider membuat invoice tetapi response timeout. Booking dibatalkan/stock dirilis tanpa lookup invoice; atau cancel/ledger gagal tetapi create mengembalikan error generik. Payment kemudian datang untuk intent yang sudah dianggap gagal.

**Dampak.** Invoice, booking, stok dan ledger dapat tidak sinkron. Semua attempt untuk satu booking juga dapat kehilangan perbedaan status jika update dilakukan sekaligus.

**Rekomendasi.** Bedakan definitive failure dari outcome unknown. Persist provider intent/attempt sebelum call dengan idempotency, reconciliation job dan recovery status. Kompensasi harus durable/retryable; jangan menganggap release sukses jika gagal. Update attempt spesifik, dan simpan late-payment exception/refund intent tanpa auto-confirm stock yang telah dilepas.

**Kriteria penerimaan dan verifikasi.** Mock provider menerima request lalu timeout: retry/lookup menemukan invoice yang sama. Inject ledger/compensation failure menghasilkan recovery job/audit yang dapat ditelusuri. Real-DB fault tests membuktikan state, inventory dan outbox; failure tidak hilang dari ledger.

**Implikasi frontend.** Jangan menyarankan pembayaran baru ketika outcome belum pasti. Refresh status authoritative dan tampilkan bantuan/reference; tidak menyebut refund berhasil tanpa backend evidence.

## BE-R14 — Webhook tidak mencocokkan nilai dan invoice dengan ledger

**Prioritas:** P1. **Status:** RESOLVED 2026-10-03 (amount & ledger reconciliation, currency matching, stale expiry guard). **Hubungan:** BE-G10/11; F05/F14.

**Bukti source.** [payment/xendit.go](../../internal/adapter/payment/xendit.go) (lihat fungsi terkait) sudah memverifikasi callback token constant-time dan external_id. [router.go](../../internal/api/router.go), `xenditWebhook`, mengonfirmasi PAID/SETTLED berdasarkan ExternalID tanpa mencocokkan invoice ID, amount/currency dan attempt yang tercatat. EXPIRED mengabaikan error Cancel lalu mengakui sukses.

**Skenario pemicu.** Callback dengan shared token valid menunjuk booking yang benar tetapi invoice/amount berbeda; atau expiry callback gagal memutasi booking namun dibalas sukses.

**Dampak.** Keaslian channel callback belum memastikan pembayaran sesuai booking. Provider dapat berhenti retry walau local mutation gagal. Ini gap validasi source, bukan klaim adanya pemalsuan callback di produksi.

**Rekomendasi.** Cocokkan event ke attempt/reference, booking, amount/currency dan environment provider; deduplicate event secara persisten. Persist event/rejection sebelum acknowledge. Untuk expiry, bedakan event stale terhadap payment sukses dengan failure mutation dan buat reconciliation path.

**Kriteria penerimaan dan verifikasi.** Token valid + amount/currency/reference salah tidak mengonfirmasi; booking/stok tetap benar dan rejection tercatat. Duplicate/out-of-order PAID/EXPIRED tidak membatalkan paid booking atau mengulang notifikasi. Inject DB failure tidak mendapat acknowledgment final tanpa event durable untuk retry.

**Implikasi frontend.** Redirect `payment=success` bukan bukti bayar; halaman status hanya percaya state server.

## BE-R15 — Link pembayaran tidak tersedia untuk recovery lintas sesi

**Prioritas:** P1. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G11/12; F03/F05.

**Bukti source.** `createBooking` mengembalikan payment_url/reference sekali. [booking.go](../../internal/booking/booking.go) (lihat fungsi terkait) dan [guest/model.go](../../internal/guest/model.go), BookingDetail, tidak memuat payment URL. PaymentAttempt menyimpan provider_reference tetapi tidak URL; router belum memiliki resume/lookup endpoint.

**Skenario pemicu.** Tamu refresh setelah state FE hilang, login di perangkat lain, atau kembali dari provider ke detail pending. Backend dapat membaca booking tetapi belum menyediakan cara aman meneruskan invoice yang sama.

**Dampak.** CanPay tidak cukup untuk membuat tombol berfungsi. FE hanya dapat mempertahankan response awal pada perangkat yang sama; recovery penuh tidak dapat diselesaikan di frontend.

**Rekomendasi.** Tambahkan private payment view/recovery endpoint yang me-resolve invoice existing dari attempt/provider. Validasi ownership, hold, provider expiry dan status. Jangan membuat invoice baru pada setiap GET; persist URL yang sesuai atau ambil ulang secara terkontrol.

**Kriteria penerimaan dan verifikasi.** Setelah create, hilangkan semua state client lalu login di perangkat baru: booking pending membuka invoice yang sama dengan attempt count tetap. Confirmed/expired tidak menawarkan pay. Provider lookup down memberi retryable recovery state tanpa invoice ganda.

**Implikasi frontend.** Simpan response awal hanya sebagai bantuan sesi, dengan batas yang jujur; jangan membentuk payment URL dari reference/UUID.

## BE-R16 — Deployment production masih dapat memilih fake gateway dan log notifier

**Prioritas:** P0. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G10/16; F05/F11/F15.

**Bukti source.** [cmd/server/main.go](../../cmd/server/main.go), wiring payment/notifier, memilih NewFake bila XenditSecretKey kosong dan LogNotifier bila ResendAPIKey kosong tanpa memeriksa production. Route fake-pay sekarang memang digate IsDevelopment; itu perbaikan nyata tetapi tidak menghentikan fallback adapter. LogNotifier OTP menulis kode ke log.

**Skenario pemicu.** APP_ENV production dengan key provider tidak terpasang tetap boot memakai fake payment/log email. User mendapat link dummy atau pesan OTP dikirim, padahal tidak ada email nyata.

**Dampak.** Environment misconfiguration tersamar sebagai service sehat. Kode OTP yang masuk log memperluas akses kredensial jika fallback aktif di deployment.

**Rekomendasi.** Fail startup/readiness production bila dependency provider wajib tidak valid. Fake payment dan OTP log harus khusus development/test dengan konfigurasi eksplisit. Pisahkan capability health dari liveness dan jangan mencatat OTP pada log production. Tentukan policy degraded mode notifikasi berdasarkan fungsi, bukan fallback diam-diam.

**Kriteria penerimaan dan verifikasi.** Production tanpa key gagal terkontrol; tidak membuat booking lewat fake adapter dan tidak mencatat OTP. Development tetap dapat memakai fake dengan label jelas. Deployment sandbox memakai kredensial sandbox dan menunjukkan kapabilitas readiness yang benar.

**Implikasi frontend.** Label demo/live berasal dari environment terverifikasi; jangan menghilangkan badge hanya karena healthz 200.

## BE-R17 — OTP delivery tidak memiliki retry durable dan tetap mengaku terkirim

**Prioritas:** P1. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G16; F02/F11.

**Bukti source.** [guest/service.go](../../internal/guest/service.go) (lihat fungsi terkait) menyimpan challenge sebelum mengirim OTP, lalu hanya log error notifier dan mengembalikan cooldown sukses. [notifier/resend.go](../../internal/adapter/notifier/resend.go) (lihat fungsi terkait) membuat idempotency key dari email dan menit wall clock, bukan challenge ID. Handler challenge mengatakan kode telah dikirim.

**Skenario pemicu.** Provider email timeout/reject setelah challenge tersimpan. Tamu menerima sukses dan cooldown 60 detik, sementara kode tidak sampai. Challenge paralel dapat menghasilkan key provider sama untuk body OTP berbeda.

**Dampak.** Login buntu tanpa delivery recovery; respons anti-enumeration tidak harus menjanjikan delivery yang tidak terbukti.

**Rekomendasi.** Enqueue OTP dispatch secara durable dengan idempotency per challenge dan retry terukur, jaga expiry saat dispatch terlambat. Response boleh generik accepted untuk anti-enumeration tetapi tidak menyatakan delivered. Bedakan accepted/sent/failed; redaksi log dan tentukan retensi data.

**Kriteria penerimaan dan verifikasi.** Inject timeout/reject email: intent tetap dapat ditelusuri dan retry memakai key yang sama untuk challenge sama. Challenge baru tidak memakai key challenge lama. OTP expired tidak dikirim ulang sebagai kode aktif; respons tidak membocorkan keberadaan booking/email.

**Implikasi frontend.** Tampilkan permintaan diterima dan petunjuk resend/cooldown, bukan jaminan email masuk. Jangan meminta pengguna membaca OTP dari log untuk alur live.
