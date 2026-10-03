# Review ulang checkout, pricing dan kebijakan

Tanggal: 3 Oktober 2026 (Asia/Jakarta). Basis: source snapshot pada manifest review ulang. Temuan adalah hasil inspeksi source; reproduksi race/fault dan acceptance tests di bawah merupakan pekerjaan verifikasi yang masih diperlukan. Kode backend tidak diubah oleh review ini.

Dokumen ini memperbarui konteks audit lama, tanpa menganggap semua BE-G tetap OPEN atau seluruh F01–F15 sudah selesai. [Register terkini](07-backend-reaudit-2026-10-03.md) menjelaskan status dan batas bukti.

## BE-R06 — Create tanpa quote melewati consent dan snapshot harga lengkap

**Prioritas:** P1. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G06/08; F01.

**Bukti source.** [booking/service.go](../../internal/booking/service.go) (lihat fungsi terkait) memeriksa dua consent hanya saat `QuoteID` terisi;  pada bagian terkait menyediakan fallback tanpa quote dengan harga nightly dasar. [router.go](../../internal/api/router.go), `createBooking`, menerima quote kosong. Agregat/migrasi pricing menyimpan terms tetapi tidak mempunyai snapshot privacy acceptance/version setara.

**Skenario pemicu.** Caller mengirim create dengan quote kosong dan terms/privacy false. Jika inventory/contact valid, jalur fallback tetap dapat membuat hold dan charge berdasarkan harga dasar.

**Dampak.** Kontrak checkout terkunci dapat dilewati; bukti persetujuan dan komponen pajak/paket berbeda dari alur quote resmi.

**Rekomendasi.** Wajibkan quote valid dan dua persetujuan untuk create publik. Jika compatibility internal masih dibutuhkan, pisahkan endpoint/permission dan periode migrasinya. Simpan version dokumen, waktu dan bukti persetujuan yang disetujui pemilik; jangan sekadar menambah checkbox FE.

**Kriteria penerimaan dan verifikasi.** Create publik dengan quote kosong/expired/mismatch atau salah satu consent false ditolak sebelum inventory/booking/provider berubah. Jalur sukses menyimpan snapshot pricing dan consent yang dapat dibaca untuk audit. Uji route server sesungguhnya, termasuk fallback.

**Implikasi frontend.** Kirim quote_id dan consent terpisah; tidak mencoba fallback tanpa quote saat expired.

## BE-R07 — Kapasitas search belum menjadi invariant checkout

**Prioritas:** P1. **Status:** **RESOLVED 2026-10-03** ([laporan](../../testing/e2e/report/2026-10-03-213000-search-quote-checkout-capacity-r07-e2e-report.md)). **Hubungan:** BE-G03; F01/F08.

**Bukti source.** [router.go](../../internal/api/router.go), `searchRooms`, membandingkan total tamu dan dewasa terhadap kapasitas agregat. Tidak memeriksa MaxChildren atau equality jumlah child_ages dengan children. [booking/service.go](../../internal/booking/service.go) (lihat fungsi terkait) hanya membatasi jumlah kamar dan guests positif, tanpa membaca kapasitas katalog; quote juga menerima jumlah yang dinormalisasi.

**Skenario pemicu.** Caller melewati search dan meminta quote/create dengan num_guests di atas kapasitas tipe kamar. Distribusi tamu yang tidak valid per kamar dapat lolos batas agregat.

**Dampak.** Reservasi dapat diterima meskipun kamar tidak memenuhi kapasitas fisik. FE validation tidak melindungi direct API.

**Rekomendasi.** Jadikan validasi kapasitas katalog, adults/children/ages dan distribusi kamar bagian quote/create. Bila v1 sengaja hanya menerima jumlah total, tetapkan batas agregat konservatif dan nyatakan keterbatasannya; jangan mengklaim validasi per kamar yang tidak disimpan. Search dan checkout harus menggunakan aturan yang sama.

**Resolusi (2026-10-03).** Penegakan invariant kapasitas fisik kamar katalog telah diimplementasikan secara terintegrasi pada seluruh alur pencarian, kuotasi harga, dan checkout:
1. `searchRooms` memvalidasi `adults >= rooms`, `len(child_ages) == children`, umur dalam $[0, 17]$, dan menandai varian `available: false` (`EXCEEDS_CAPACITY`) bila `adults > MaxAdults*rooms`, `children > MaxChildren*rooms`, atau `totalGuests > MaxCapacity*rooms`.
2. `calculateQuote` membaca `CatalogStore.GetVariant`, menolak overcapacity dengan 400 `EXCEEDS_CAPACITY` serta rasio tamu invalid dengan 400 `INVALID_GUEST_COUNT`.
3. `booking.Service.Create` dan `createBooking` mengikat port `CatalogReader` dan menolak pembuatan booking overcapacity dengan `ErrExceedsCapacity` (HTTP 400 `EXCEEDS_CAPACITY`) dan `ErrInvalidCapacity`.
4. Dokumen siklus hidup: [PRD](../../docs/prd/search-quote-checkout-capacity-invariant-r07-2026-10-03.md), [SRS](../../docs/srs/search-quote-checkout-capacity-invariant-r07-2026-10-03.md), [Tech Architecture](../../docs/tech/search-quote-checkout-capacity-invariant-architecture-2026-10-03.md), [Walkthrough](../../docs/walkthrough/search-quote-checkout-capacity-invariant-r07-walkthrough-2026-10-03.md), [E2E Test Report](../../testing/e2e/report/2026-10-03-213000-search-quote-checkout-capacity-r07-e2e-report.md).

**Kriteria penerimaan dan verifikasi.** Uji bypass search, overcapacity, MaxAdults/MaxChildren, jumlah age tidak cocok, adults kurang dari jumlah kamar dan distribusi invalid. Semua ditolak sebelum hold/provider; valid boundary diterima. Perubahan kapasitas setelah quote harus mengikuti policy version yang disepakati.

**Implikasi frontend.** Jangan membuat pemilihan mixed cart lalu memecahnya ke beberapa create tanpa atomic batch contract.

## BE-R08 — Idempotency checkout masih lookup-create-save

**Prioritas:** P1. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G09; F01/F05.

**Bukti source.** [router.go](../../internal/api/router.go), `createBooking`, membaca record sebelum `BookingSvc.Create` dan menyimpan record setelah provider/booking berhasil; error Save diabaikan. [idempotency.go](../../internal/api/idempotency.go) (lihat fungsi terkait) mengonversi semua error DB menjadi not-found,  pada bagian terkait upsert response setelah efek samping.

**Skenario pemicu.** Dua request key/body sama masuk bersamaan, keduanya membaca not-found kemudian masing-masing membuat hold dan invoice. Inventory locking mencegah oversell, tetapi tidak mencegah dua booking sah untuk satu intent jika stok masih cukup.

**Dampak.** Retry bisa menduplikasi reservasi dan invoice. Test replay serial tidak membuktikan atomic deduplication.

**Rekomendasi.** Reservasi key secara atomik sebelum efek samping; persist status in-progress/completed, fingerprint, scope actor dan resource. Bedakan not-found dari store unavailable. Kaitkan intent dengan transaksi booking dan provider idempotency/recovery; jangan mengganti response key yang konflik. Tentukan window retensi serta akses cached token/PII.

**Kriteria penerimaan dan verifikasi.** 20 request paralel key/body sama menghasilkan satu booking, satu decrement per malam dan satu provider intent; response menunjuk resource sama. Body berbeda ditolak 409 tanpa mutation. Inject DB failure/crash setelah commit dan setelah provider menerima; retry pulih tanpa duplikasi. Key hilang/terlalu panjang harus mengikuti kontrak terdokumentasi.

**Implikasi frontend.** Pertahankan key dan serialisasi body yang identik sepanjang retry. Tombol disabled hanya UX; FE tidak boleh mengklaim exactly-once.

## BE-R09 — Harga katalog CRUD tidak menjadi sumber rate engine

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, [PRD](../prd/catalog-crud-rate-engine-source-r09-2026-10-03.md), [SRS](../srs/catalog-crud-rate-engine-source-r09-2026-10-03.md), [Tech Architecture](../tech/catalog-crud-rate-engine-source-architecture-2026-10-03.md), [Walkthrough](../walkthrough/catalog-crud-rate-engine-source-r09-walkthrough-2026-10-03.md), [Laporan E2E](../../testing/e2e/report/2026-10-03-221000-catalog-crud-rate-engine-r09-e2e-report.md)). **Hubungan:** BE-G01/04/05; F08/F09.

**Bukti source.** [cmd/server/main.go](../../cmd/server/main.go), `baseRates`, memuat map tujuh ID/harga tetap; `rates.NewEngine` menggunakan map tersebut. Catalog CRUD menyimpan BasePriceMinor melalui store terpisah. Search mengabaikan error RateSvc.Quote ketika mengisi price.

**Skenario pemicu.** Revenue mengubah base_price_minor lewat katalog atau menambah tipe kamar. Quote masih memakai harga map lama atau unknown room type; search bisa menampilkan available tanpa harga yang valid.

**Dampak.** UI admin dan checkout membaca sumber harga berbeda. Kamar tampak dapat dipesan padahal tidak memiliki rate.

**Rekomendasi.** Pisahkan sumber metadata dan sumber tarif secara eksplisit, lalu hubungkan rate store/version dengan catalog identity. Bila base_price_minor bukan harga operasional, beri nama/deskripsi yang tidak menjanjikan perubahan checkout. Search harus menandai unpriced dan tidak menawarkan booking pada rate lookup gagal.

**Kriteria penerimaan dan verifikasi.** Update rate yang approved memengaruhi quote baru dan mempertahankan quote/history lama sesuai policy. Tipe baru dengan tariff valid dapat diquote; tanpa tariff tidak tampil sebagai opsi bookable bernilai nol. Uji restart dan invalidation cache.

**Implikasi frontend.** Ambil total dari quote server; jangan memakai harga katalog sebagai total final atau menskalakan harga demo.

## BE-R10 — Makna num_guests menyebabkan biaya sarapan ambigu

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, lihat [laporan E2E](../../testing/e2e/report/2026-10-03-223500-breakfast-pricing-guest-tiers-r10-e2e-report.md)). **Hubungan:** BE-G04/05; F01/F09.

**Bukti source.** [rates/engine.go](../../internal/rates/engine.go):230 menghitung `100000 × numGuests × nights × numRooms`. Search memakai adults/children total untuk semua kamar, sedangkan create/quote memakai num_guests tanpa distribusi atau pembeda dewasa/anak. Komentar biaya menyebut per orang dewasa.

**Skenario pemicu.** Dua kamar untuk empat tamu, satu malam: jika num_guests adalah total booking, sarapan dihitung Rp800.000, bukan Rp400.000. Jika dianggap per kamar, capacity dan ringkasan tamu memakai arti berbeda.

**Dampak.** Potensi overcharge multi-room dan charge anak yang tidak sesuai kebijakan. Angka contoh adalah konsekuensi rumus, bukan kebijakan hotel yang ditetapkan review.

**Rekomendasi.** Tetapkan satu arti num_guests di SRS dan DTO: total booking atau per kamar. Untuk total booking, jangan kalikan jumlah kamar lagi. Definisikan eligible breakfast adults/child tiers berdasarkan kebijakan hotel; gunakan satu shared calculation pada quote dan snapshot.

**Kriteria penerimaan dan verifikasi.** Table tests 1/2/3 kamar, okupansi berbeda, beberapa malam dan anak membuktikan unit biaya tepat tanpa double counting. Contoh 2 kamar/4 eligible guests/1 malam menghasilkan Rp400.000 jika biaya Rp100.000 dan total-booking contract dipilih. Pajak/discount memakai basis yang disepakati.

**Implikasi frontend.** Jangan menambal selisih dengan menghitung ulang total FE. Tampilkan breakdown otoritatif dan batas dukungan sampai kontrak diperjelas.

## BE-R11 — Quote hanya tersimpan pada memori satu instance

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, lihat [laporan E2E](../../testing/e2e/report/2026-10-03-231500-durable-quote-store-r11-e2e-report.md)). **Hubungan:** BE-G06; F01/F05.

**Bukti source.** [rates/engine.go](../../internal/rates/engine.go) (lihat fungsi terkait) menggunakan MemoryQuoteStore 15 menit;  pada bagian terkait mengabaikan error SaveQuote. Router menghubungkan engine quote store ke BookingSvc, jadi wiring quote telah ada dan bukan gap terpisah.

**Skenario pemicu.** Quote dibuat pada instance A lalu create menuju B, atau process restart sebelum TTL habis. Quote tidak ditemukan dan diperlakukan expired meskipun waktu quote masih valid.

**Dampak.** Checkout gagal secara acak pada scale/restart. Implementasi future persistent store juga bisa mengembalikan quote sukses walau Save gagal.

**Rekomendasi.** Gunakan shared durable quote store dengan expiry, policy/rate version dan lifecycle cleanup. Propagasikan kegagalan persist. Jika deployment tetap satu instance, nyatakan restart invalidation secara eksplisit sebagai keterbatasan trial; sticky session tidak memberi durability.

**Kriteria penerimaan dan verifikasi.** Quote dari A dapat digunakan di B dan setelah restart sebelum expiry; sesudah expiry ditolak. Inject Save failure tidak menghasilkan quote sukses. Requote tidak mengubah harga/policy tanpa persetujuan ulang.

**Implikasi frontend.** QUOTE_EXPIRED memicu pilihan quote baru dan review ulang; jangan diam-diam mengirim quote kosong.

## BE-R12 — Deadline pembatalan memakai UTC ketika kebijakan menyebut WIB

**Prioritas:** P1. **Status:** RESOLVED (2026-10-03, lihat [laporan E2E](../../testing/e2e/report/2026-10-03-202800-cancellation-timezone-r12-e2e-report.md)). **Hubungan:** BE-G08/19; F06/F13.

**Bukti source.** [booking/service.go](../../internal/booking/service.go) (lihat fungsi terkait) membangun jam 14:00 dengan `time.UTC`. [rates/engine.go](../../internal/rates/engine.go):235 menyebut jam 14:00 WIB. Snapshot lama official check-in 15:00 adalah keputusan berbeda, bukan alasan mengganti cutoff sendiri.

**Skenario pemicu.** Check-in 10 Oktober: 48 jam sebelum 14:00 WIB adalah 8 Oktober 07:00 UTC; source memberi cutoff 8 Oktober 14:00 UTC, tujuh jam lebih lambat.

**Dampak.** Pembatalan gratis bisa diterima setelah batas yang dijanjikan. Tamu dan front desk memiliki interpretasi waktu berbeda.

**Rekomendasi.** Tetapkan cutoff cancellation resmi dengan hotel, muat Asia/Jakarta dan hitung instant UTC dari waktu lokal. Simpan deadline/version pada snapshot dan kembalikan pada private detail/allowed_actions; pisahkan cancellation cutoff dari check-in time.

**Kriteria penerimaan dan verifikasi.** Clock-controlled test tepat sebelum/di/sesudah deadline WIB; test tidak bergantung timezone process. Response policy, allowed_actions dan cancel endpoint memakai instant yang sama untuk booking historis.

**Implikasi frontend.** Tampilkan deadline dari server; jangan menyimpulkan jam cutoff dari tanggal atau menghitung kebijakan sendiri.

## BE-R18 — Boundary payload dan error API belum seragam

**Prioritas:** P2. **Status:** OPEN pada snapshot review. **Hubungan:** BE-G09/15; F01/F02.

**Bukti source.** `createBooking` dan `xenditWebhook` memakai io.ReadAll tanpa MaxBytesReader; guest handlers memakai decoder tanpa batas body eksplisit. Booking errors memakai ProblemDetails, guest endpoints memakai `{error,message}`. Header key checkout belum ditegakkan batasnya pada handler meskipun Batch D menyebut 1–64 karakter.

**Skenario pemicu.** Caller mengirim JSON sangat besar, key di luar batas schema, atau FE membaca semua error seolah selalu memiliki code/detail.

**Dampak.** Beban memori dan error generic/500 dapat muncul; integrasi perlu normalisasi dua bentuk error. Rate limiter global tidak menggantikan body limit.

**Rekomendasi.** Terapkan limit request body per endpoint dan batas panjang field/key sebelum domain/provider. Konsistenkan machine code dan HTTP status secara bertahap dengan compatibility terdokumentasi. Bedakan error DB/internal yang tidak boleh terpapar dan user-fixable validation.

**Kriteria penerimaan dan verifikasi.** Oversized body mendapat 413 tanpa mutation, malformed JSON 400, key invalid ditolak sesuai kontrak. FE contract tests mencakup kedua bentuk error selama migrasi. Rate limit menggunakan 429 dengan retry metadata yang dapat dipakai client.

**Implikasi frontend.** Normalisasi code/message/status; jangan hanya memeriksa `.detail`. Form validation tetap dibantu enforcement server.
