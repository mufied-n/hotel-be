# Catalog, search dan pricing

Audit: 3 Oktober 2026. Evidence source-level kecuali disebutkan. Prioritas adalah gate untuk replacement live; trial UI fixtures dapat berjalan paralel.

## BE-G01 — Katalog dan inventory belum mewakili PULANG

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [migrations/00002_seed.sql:2](/mnt/code/projects/jobs/pulang/current-booking/migrations/00002_seed.sql:2), [migrations/00001_init.sql:6](/mnt/code/projects/jobs/pulang/current-booking/migrations/00001_init.sql:6), [internal/api/router.go:53](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:53).

**Gap dan dampak:** Seed adalah Standard/Superior/Deluxe/Family/Suite (100 kamar), sedangkan website menyatakan 95 kamar dalam 5 keluarga dan results memiliki 7 varian king/twin. Tidak ada media, amenities, bed, ukuran, deskripsi, slug atau route catalog. Trial dengan seed bukan replacement hotel yang sesuai.

**Tindakan:** Pisahkan family konten, sellable room variant, dan rate plan; konfirmasi allocation fisik per varian dengan hotel. Catalog API harus membawa metadata/foto/alt/capacity dan stable ID. Jangan menyimpulkan 15 bespoke designs adalah 15 SKU inventory.

**Kriteria verifikasi:** Fixture mapping 5 keluarga→7 varian direview; stok fisik/kanal sesuai owner. FE tidak menanam UUID seed sebagai kontrak produksi.

## BE-G02 — Availability tidak menjamin rentang lengkap atau cukup untuk pencarian

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/router.go:89](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:89), [internal/inventory/postgres.go:20](/mnt/code/projects/jobs/pulang/current-booking/internal/inventory/postgres.go:20), [internal/inventory/availability.go:37](/mnt/code/projects/jobs/pulang/current-booking/internal/inventory/availability.go:37).

**Gap dan dampak:** GET availability memerlukan room_type_id yang harus sudah diketahui, mengembalikan rows tanpa menjalankan inventory.Check. Jika satu tanggal kosong dari DB tetapi tanggal lain ada, hasil 200 dengan quote semua malam. Tidak ada num_rooms/occupancy filter atau hasil semua varian. Create masih memeriksa rentang sehingga integrity create terbantu, tetapi search bisa menampilkan opsi tidak dapat dipesan.

**Tindakan:** Buat search lintas sellable variants dengan query dates/occupancy/rooms, validasi seluruh malam, effective minimum stock dan reason unavailable. Bedakan sold out, missing inventory, unknown room, dan beyond booking horizon.

**Kriteria verifikasi:** Test missing middle night, sold-out satu malam, request dua kamar dengan stok satu, semua kamar, dan empty result. Semua hasil bookable harus lolos pemeriksaan create saat kondisi inventory tidak berubah.

## BE-G03 — Occupancy dan batas booking belum divalidasi

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service.go:117](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:117), [migrations/00001_init.sql:9](/mnt/code/projects/jobs/pulang/current-booking/migrations/00001_init.sql:9), [internal/api/router.go:124](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:124).

**Gap dan dampak:** Create hanya memastikan counts positif; max_capacity tidak dibaca. num_guests=99 dengan satu kamar berkapasitas 2 dapat diterima jika inventory ada. Tidak ada adults/children/ages per room, tanggal lampau, batas LOS/horizon/max rooms, validasi nama/email/UUID. Data invalid dapat menjadi 500, overload query, atau reservation yang tidak dapat dilayani.

**Tindakan:** Definisikan occupancy hotel (anak/usia/extra bed/infant), minimum adults per room, max capacity per sellable unit, max stay/search horizon/rooms dan tanggal Asia/Jakarta. Validasi body/UUID/contact di boundary serta invariant domain.

**Kriteria verifikasi:** Test boundaries kapasitas, child ages, zero adults, tanggal lampau/horizon, max LOS, invalid UUID/email, nama kosong. Balas error field stabil 400/422; jangan rely DB untuk validation UX.

## BE-G04 — Rate plan, breakfast dan promo belum tersedia

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/rates/engine.go:24](/mnt/code/projects/jobs/pulang/current-booking/internal/rates/engine.go:24), [cmd/server/main.go:55](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:55), [internal/booking/service.go:59](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:59).

**Gap dan dampak:** Rate engine hanya base map hardcoded dan Friday/Saturday factor 1.25. Tidak menerima rate_plan_id atau promo code, tidak ada meal inclusion/benefits/validity/restrictions. Seluruh varian tarif OCTOBREAK dari vendor tidak dapat direpresentasikan.

**Tindakan:** Tambah kontrak rate plan dan konfigurasi hotel yang dapat dikelola, breakfast inclusion/entitlement, package benefits, periode booking/stay, promo eligibility/stacking dan restriction. Snapshot public tidak dijadikan rumus produksi.

**Kriteria verifikasi:** Golden quote hotel untuk search 3–4 Oktober, Room Only/Breakfast, promo expired/invalid, multi-night/weekend, child/room allocation; verify manfaat sesuai rate yang dipilih.

## BE-G05 — Kontrak uang dan breakdown harga belum lengkap

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/rates/engine.go:17](/mnt/code/projects/jobs/pulang/current-booking/internal/rates/engine.go:17), [internal/rates/engine.go:45](/mnt/code/projects/jobs/pulang/current-booking/internal/rates/engine.go:45), [internal/booking/service.go:137](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:137), [internal/booking/postgres.go:106](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go:106).

**Gap dan dampak:** Label RateMinor menyebut sen/cent tetapi base IDR 550_000 tidak menyatakan exponent. Total belum menguraikan original/subtotal/discount/tax/service/rounding. Engine menggunakan float factor. Nightly rate tersimpan, tetapi tidak snapshot rate plan, taxes atau discount; num_rooms perlu eksplisit dalam breakdown. Risiko FE/gateway mengalikan atau membagi 100 berbeda dan double-count included tax.

**Tindakan:** Tetapkan Money contract currency+integer amount+exponent yang seragam sampai gateway. Gunakan integer/rational percentage dan rounding policy. Snapshot seluruh monetary components/policy/version; sum per-night×rooms + charges - discount harus sama total.

**Kriteria verifikasi:** Golden tests tax included/excluded, multi-room, pembulatan pecahan, quote snapshot setelah rates berubah, provider amount mapping. Jangan klaim bug 100× sudah terjadi; sekarang kontraknya ambigu dan gateway masih fake.

## BE-G06 — Tidak ada quote yang dapat dipertahankan dari results ke checkout

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/router.go:114](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:114), [internal/booking/service.go:133](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:133).

**Gap dan dampak:** Search dan create menghitung harga independen; tidak ada quote_id, expiry, version atau acceptance harga berubah. Rate plan belum tersimpan. Saat rate berubah di kemudian hari tamu bisa melihat total lama lalu membayar total baru tanpa review.

**Tindakan:** Search menghasilkan quote terikat dates/occupancy/variant/rate/currency dengan TTL dan snapshot. Create memvalidasi quote; bila stale balas QUOTE_EXPIRED/PRICE_CHANGED dengan next action. Requote memerlukan review tamu.

**Kriteria verifikasi:** Test quote expired, input tampered, inventory sold-out setelah quote, harga berubah saat review. FE tidak menentukan harga final sendiri.

## BE-G19 — Metadata hotel, locale dan currency presentation belum punya kontrak

Prioritas: **P2**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service.go:150](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:150), [internal/api/router.go:114](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:114).

**Gap dan dampak:** Currency hardcoded IDR; response availability bahkan tidak menyertakan currency/exponent. Tidak ada property metadata/jam/tz/contact/policy content. Vendor menawarkan language/currency UI, backend tidak memiliki model localization/conversion.

**Tindakan:** MVP IDR dengan timezone Asia/Jakarta, locale tampilan id/en dan property content fixture terlebih dahulu. Backend monetary contract wajib eksplisit; currency lain hanya bila rate FX+rounding/provider disepakati. Metadata owner tunggal.

**Kriteria verifikasi:** Formatter IDR konsisten dengan Money; date-only tetap sama lintas browser timezone; check-in 15:00/check-out 12:00 selalu jelas. Unsupported currency tidak silently diperlakukan IDR.
