# Register gap backend — parity booking PULANG

Tanggal: 3 Oktober 2026. Kesimpulan: backend adalah fondasi demonstrasi modular monolith yang layak dilanjutkan, **belum sesuai untuk replacement live Book Secure**. UI Nuxt 4 dapat dimulai dengan fixture terpisah; pembayaran dan stok live diblokir sampai gate backend terpenuhi.

## Cara membaca

P0: blokir exposure/live booking. P1: requirement inti atau correctness/recovery sebelum integrasi/live. P2: hardening/keputusan operasional; scope yang ditunda harus eksplisit. Semua temuan memiliki trigger, bukti source, tindakan dan acceptance. OPEN bukan berarti semua fitur wajib selesai sebelum trial UI.

| ID | Prioritas | Temuan | Status |
|---|---|---|---|
| BE-G01 | P1 | [Katalog dan inventory belum mewakili PULANG](01-catalog-search-pricing.md) | OPEN |
| BE-G02 | P1 | [Availability tidak menjamin rentang lengkap atau cukup untuk pencarian](01-catalog-search-pricing.md) | OPEN |
| BE-G03 | P1 | [Occupancy dan batas booking belum divalidasi](01-catalog-search-pricing.md) | OPEN |
| BE-G04 | P1 | [Rate plan, breakfast dan promo belum tersedia](01-catalog-search-pricing.md) | OPEN |
| BE-G05 | P1 | [Kontrak uang dan breakdown harga belum lengkap](01-catalog-search-pricing.md) | OPEN |
| BE-G06 | P1 | [Tidak ada quote yang dapat dipertahankan dari results ke checkout](01-catalog-search-pricing.md) | OPEN |
| BE-G07 | P1 | [Guest details dan special request belum sesuai checkout](02-checkout-policy.md) | OPEN |
| BE-G08 | P0 | [Cancellation policy dan consent tidak ditegakkan](02-checkout-policy.md) | OPEN |
| BE-G09 | P1 | [Create belum idempoten dan sulit dipulihkan](02-checkout-policy.md) | OPEN |
| BE-G10 | P0 | [Pembayaran masih fake dan route dev dapat mengonfirmasi tanpa bayar](03-payment-hold-recovery.md) | OPEN |
| BE-G11 | P1 | [Payment attempt, status, refund dan rekonsiliasi tidak persisten](03-payment-hold-recovery.md) | OPEN |
| BE-G12 | P1 | [Hold deadline belum menjadi otoritas server dan recovery belum andal](03-payment-hold-recovery.md) | OPEN |
| BE-G13 | P1 | [Read booking mengungkap PII tanpa bukti kepemilikan](04-security-api.md) | OPEN |
| BE-G14 | P0 | [Identitas RBAC dapat dipalsukan dan enforcer fail-open](04-security-api.md) | OPEN |
| BE-G15 | P1 | [API publik belum membatasi abuse dan kontrak error](04-security-api.md) | OPEN |
| BE-G16 | P1 | [Notification dan outbox belum siap pengiriman nyata](05-inventory-workers-operations.md) | OPEN |
| BE-G17 | P1 | [Assignment paralel bisa gagal walau kamar lain bebas](05-inventory-workers-operations.md) | OPEN |
| BE-G18 | P1 | [Sumber inventory kanal lain, maintenance dan horizon belum ada](05-inventory-workers-operations.md) | OPEN |
| BE-G19 | P2 | [Metadata hotel, locale dan currency presentation belum punya kontrak](01-catalog-search-pricing.md) | OPEN |
| BE-G20 | P1 | [Bukti verifikasi DB dan payment belum tersedia](06-verification-remediation.md) | OPEN |
| BE-G21 | P2 | [Sweep dan resource configuration belum sepenuhnya observable](05-inventory-workers-operations.md) | OPEN |
| BE-G22 | P2 | [Early checkout, no-show dan continuous-room stay belum punya policy](05-inventory-workers-operations.md) | OPEN |

## Baseline dan batas bukti

Source yang ditinjau: seluruh `.go` di cmd/internal pada baseline awal, dua migrasi awal; pengecekan akhir mencakup tambahan middleware/Casbin, model RBAC dan migrasi ketiga. Referensi diperbarui terhadap source akhir. Backend berubah selama audit, jadi laporan harus dibaca bersama manifest snapshot. README, Makefile, Dockerfile/Compose, desain dan remediation docs juga diperiksa. [Manifest source](source-manifest.json) merekam hash untuk re-audit. Direktori backend bukan Git checkout pada saat `git status`; tidak ada commit SHA yang dapat diklaim. Tidak mengubah kode atau schema dan tidak menjalankan transaksi pada DB milik pengguna.

`go version`: go1.27.0 linux/amd64. `go test -race -cover ./...`: exit 0; api 72.6%, booking 42.5%, inventory 26.5%, rates 91.7%, workers 11.0%; cmd/server, cmd/migrate, adapters dan platform 0.0%. `go vet ./...`: exit 0. Ini hasil test yang tersedia, bukan bukti semua SQL/production concurrency/provider integration lulus. Security scanner/CVE scan, deployment TLS, encryption-at-rest, backup/restore, external proxy auth dan real-DB test belum diverifikasi; tidak ada klaim vulnerability CVE tertentu atau production breach.

Query DB yang diperiksa menggunakan parameter/context. Create membungkus decrement, booking, hold, nightly price dan outbox dalam satu transaksi; kekurangan stok/missing date menyebabkan rollback. `FOR UPDATE` dan GiST adalah perlindungan berguna; masih perlu bukti integration sesuai BE-G20.

## Audit ulang temuan review lama

Dokumen lama `docs/tech/code-review-remediation-plan-2026-10-03.md` masih Proposed. Status di bawah berdasarkan source sekarang, bukan checklist lama.

| ID lama | Hasil audit source |
|---|---|
| CR-01 | Payment call sudah di luar transaksi; recovery ambigu masih BE-G11/G12 |
| CR-02 | Assignment multi-room sudah ada; concurrency/error mapping masih BE-G17 |
| CR-03 | Handler booking.expired terdaftar |
| CR-04 | Nightly prices sudah diinsert; price snapshot lengkap masih BE-G05 |
| CR-05 | HoldTimeout cfg diteruskan; deadline authority masih BE-G12 |
| CR-06 | Invalid range/counts positif sudah 400; capacity/contact/UUID masih BE-G03 |
| CR-07 | /ready sudah ping DB dan Valkey |
| CR-08 | Checkout dan no-show routes/use cases sudah ada; auth/policy masih BE-G14/G22 |
| CR-09 | Test service/router/outbox bertambah; integration evidence belum ada |
| CR-10 | Calendar integer days sudah diterapkan |

## Dokumen pemilik

- [Matriks parity publik](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md).
- [Design system FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/official-design-system.md).
- [Rencana frontend UI](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/plan/frontend/README.md).
- [Urutan remediasi dan verification gates](06-verification-remediation.md).

Tidak ada approval hotel terhadap katalog stock, kebijakan final, provider atau API proposal baru yang diasumsikan oleh audit ini.

## Update source selama audit

Casbin baru ditambahkan sebelum laporan selesai. BE-G13/G14 telah diperbarui: RBAC routing ada, tetapi ownership guest, verified authentication dan fail-closed masih belum terpenuhi. Tes awal di atas mendahului tambahan Casbin; hasil test akhir dicatat pada verifikasi di bawah. Jangan memakai status test awal untuk mengklaim perubahan baru lulus.

## Verifikasi akhir setelah tambahan Casbin

`go test -race -cover ./...`: exit 0; API 70.1%, platform/auth 13.3%, booking 42.5%, inventory 26.5%, rates 91.7%, workers 11.0%. `go vet ./...`: exit 0. Tes Casbin/RBAC yang ditemukan memakai enforcer in-memory, belum membuktikan identitas/session atau adapter persistence DB benar. Manifest source merekam state setelah pembaruan ini.
