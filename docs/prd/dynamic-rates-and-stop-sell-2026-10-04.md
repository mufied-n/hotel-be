# Product Requirements Document (PRD)
# Dynamic Rates, Room Allotment & Stop-Sell Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Dokumen Identitas:** `PRD-F08-F09-RATES-ALLOTMENT-2026-10-04`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status:** APPROVED FOR SPECIFICATION
- **Dokumen Pasangan:**
  - SRS: [`docs/srs/dynamic-rates-and-stop-sell-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/dynamic-rates-and-stop-sell-2026-10-04.md)
  - Arsitektur Teknis: [`docs/tech/dynamic-rates-and-stop-sell-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/dynamic-rates-and-stop-sell-architecture-2026-10-04.md)
- **Target Role Pengguna:** `revenue_mgr`, `gm_admin`, `receptionist`, `guest`

---

## 1. Latar Belakang & Urgensi Bisnis

Hotel **Pulang ke Uttara** adalah boutique hotel bintang 4 dengan 95 kamar fisik di Jl. Kaliurang Km 5, Sleman, Yogyakarta. 

### 1.1 Masalah Saat Ini (Current State Gaps)
1. **Tarif Statis / Kaku (*Rigid Pricing*):** 
   Tarif dasar (*base price*) di database saat ini bersifat flat per tipe kamar. Penyesuaian tarif weekend hanya mengandalkan multiplier konstan `1.25` di kode, tanpa kemampuan mengatur tarif khusus musim liburan (Lebaran, Natal & Tahun Baru, libur sekolah) atau tarif diskon *low-season*.
2. **Ketiadaan Fitur Stop-Sell (*Risk of Overbooking*):**
   Ketika sejumlah kamar dialokasikan untuk pemesanan grup korporat luring (*offline wedding/corporate gathering*) atau sedang dalam perawatan berkala, hotel tidak memiliki mekanisme untuk menutup penjualan kamar (*Stop-Sell*) pada tanggal tertentu. Akibatnya, tamu online tetap dapat memesan kamar yang sebenarnya sudah penuh secara fisik.
3. **Ketiadaan Aturan Durasi Menginap (*Restriction Constraints*):**
   Hotel tidak dapat memberlakukan *Closed to Arrival* (CTA), *Closed to Departure* (CTD), atau *Minimum Length of Stay* (MLOS) di hari-hari puncak (misal: tamu wajib memesan minimal 2 malam pada malam pergantian tahun 31 Desember).
4. **Promo Terikat Hardcoded Kode:**
   Promo code `OCTOBREAK` saat ini di-hardcode dengan logika statis 15%. Tim Revenue tidak dapat membuat kampanye promo baru dengan kuota pemakaian, tanggal aktif, dan batas maksimum diskon secara mandiri.

### 1.2 Dampak Bisnis yang Diharapkan (Expected Outcomes)
* **Peningkatan RevPAR (Revenue Per Available Room) sebesar 15–25%** melalui penyesuaian harga dinamis mengikuti tingkat okupansi dan musim di Yogyakarta.
* **0% Kasus Overbooking** karena integrasi stop-sell instan yang menutup kuota penjualan direct-booking dalam hitungan milidetik.
* **Otonomi Penuh Revenue Manager:** Pengelolaan harga dan promo dapat dilakukan via API/Dashboard tanpa perlu deploy kode backend baru.

---

## 2. Profil Persona & Matriks Hak Akses (RBAC Matrix)

| Persona | Peran & Tanggung Jawab | Hak Akses Fitur (Casbin Subject) |
| :--- | :--- | :--- |
| **Revenue Manager (`revenue_mgr`)** | Merancang strategi harga harian, menetapkan stop-sell, mengatur MLOS/CTA/CTD, dan membuat kampanye promo. | Read/Write kalender tarif, stop-sell, promo campaigns. |
| **General Manager (`gm_admin`)** | Mengawasi performa pendapatan, menyetujui batas diskon ekstrem, dan mengaudit perubahan tarif hotel. | Full Control (Read/Write/Delete/Audit) pada seluruh modul revenue. |
| **Meja Depan (`receptionist`)** | Melayani tamu walk-in dan check-in; perlu mengetahui alasan jika suatu kamar tidak bisa dijual (karena stop-sell). | Read-only kalender tarif & status ketersediaan. |
| **Tamu Publik (`guest`)** | Mencari ketersediaan kamar dan melakukan reservasi direct booking. | Read-only hasil pencarian yang sudah terfilter stop-sell & terkalkulasi tarif dinamis. |

### Matriks Otorisasi Casbin

| Route HTTP | Method | Role yang Diizinkan | Keterangan Kebijakan |
| :--- | :---: | :--- | :--- |
| `/api/v1/revenue/calendar` | `GET` | `revenue_mgr`, `gm_admin`, `receptionist` | Melihat matriks tarif & restriksi per rentang tanggal |
| `/api/v1/revenue/calendar/bulk` | `PUT` | `revenue_mgr`, `gm_admin` | Mengubah tarif harian, stop-sell, CTA/CTD, MLOS massal |
| `/api/v1/revenue/promos` | `GET` | `revenue_mgr`, `gm_admin` | Daftar seluruh kampanye promo dan kuota terpakai |
| `/api/v1/revenue/promos` | `POST` | `revenue_mgr`, `gm_admin` | Membuat kampanye promo baru |
| `/api/v1/revenue/promos/:id` | `PUT` | `revenue_mgr`, `gm_admin` | Mengubah status aktif / kuota promo |
| `/api/v1/search` | `GET` | `guest`, publik | Otomatis menyaring kamar stop-sell dan menghitung tarif harian |
| `/api/v1/quotes` | `POST` | `guest`, publik | Mengunci tarif harian gabungan ke quote 15 menit |

---

## 3. Alur Pengalaman Pengguna (User Journeys)

### Journey 1: Revenue Manager Menutup Penjualan Libur Tahun Baru (Stop-Sell & MLOS)
1. Revenue Manager membuka modul Revenue Management untuk periode `2026-12-30` s/d `2027-01-02`.
2. Untuk tipe kamar *Deluxe King Balcony*, Revenue Manager menginput:
   - Tarif dinamis: `IDR 2.200.000` (High Season).
   - Minimum Length of Stay (MLOS): `2 malam`.
   - Closed to Arrival (CTA) pada tanggal `2026-12-31`: `TRUE` (tamu tidak boleh baru tiba di malam tahun baru).
3. Untuk tipe kamar *Presidential Suite*, Revenue Manager mengaktifkan:
   - `is_stop_sell: TRUE` (karena sudah dipesan secara offline oleh tamu VIP).
4. Revenue Manager menekan tombol Simpan (*Bulk Update*). Sistem melakukan pembaruan di PostgreSQL dan langsung meng-invalidasi cache tarif di Valkey.
5. Saat tamu publik mencari kamar pada tanggal 31 Desember, *Presidential Suite* tidak muncul, dan *Deluxe King* hanya dapat dipesan jika tamu memilih minimal 2 malam.

### Journey 2: Tamu Menggunakan Kuota Promo Spesial
1. Tim Revenue membuat kode promo `JOGJASERU` (diskon 20%, kuota 50 transaksi, minimum 2 malam).
2. Tamu mencari kamar 2 malam, memilih Room Only, dan memasukkan promo `JOGJASERU` saat meminta *Quote*.
3. Sistem memverifikasi:
   - Tanggal berada dalam periode aktif kampanye.
   - Durasi inap $\ge 2$ malam.
   - Sisa kuota $> 0$.
4. Sistem menghitung potongan harga maksimal 20% dan menghasilkan *Locked Quote* 15 menit. Kuota terpakai direservasi saat checkout berhasil.

---

## 4. Kriteria Keberhasilan & Penerimaan (Acceptance Criteria)

### AC-01: Penegakan Stop-Sell pada Search & Quote
* **Given** tipe kamar Deluxe King di-set `is_stop_sell = true` untuk tanggal 2026-11-10.
* **When** tamu mencari kamar dari 2026-11-09 s/d 2026-11-11 (melintasi tanggal 10).
* **Then** endpoint `/api/v1/search` menandai kamar tersebut tidak tersedia (`available: false` / `reason: STOP_SELL`), dan endpoint `/api/v1/quotes` menolak pembuatan quote dengan kode error `400 ROOM_STOP_SELL`.

### AC-02: Kalkulasi Tarif Dinamis Multi-Malam
* **Given** tipe kamar Superior memiliki base price IDR 1.000.000, dengan override tanggal 2026-10-10 sebesar IDR 1.500.000 dan tanggal 2026-10-11 sebesar IDR 1.200.000.
* **When** tamu meminta quote untuk 2 malam tersebut.
* **Then** quote menghasilkan rincian nightly breakdown yang tepat (Malam 1: 1.500.000, Malam 2: 1.200.000, Total Kamar: 2.700.000 + pajak & service charge yang sesuai).

### AC-03: Penegakan Minimum Length of Stay (MLOS)
* **Given** tanggal 2026-12-31 memiliki aturan `min_los = 2`.
* **When** tamu mencoba membuat quote hanya untuk 1 malam (31 Des – 1 Jan).
* **Then** sistem menolak dengan pesan `400 MIN_LENGTH_OF_STAY_VIOLATED` dan menyertakan rincian minimum malam yang diwajibkan.

### AC-04: Transaksi Atomik Kuota Promo
* **Given** promo `JOGJASERU` memiliki sisa kuota 1 transaksi.
* **When** dua tamu melakukan pembayaran bersamaan (concurrency race).
* **Then** hanya satu transaksi yang berhasil menggunakan promo tersebut; transaksi kedua ditolak secara anggun tanpa menyebabkan kuota bernilai negatif.

---

## 5. Kebutuhan Non-Fungsional (Non-Functional Requirements)

1. **Performa Waktu Respon:** 
   - Pencarian tarif kamar dengan evaluasi stop-sell dan override kalender wajib selesai dalam waktu $\le 20\text{ ms}$ (didukung cache Valkey dan index PostgreSQL).
2. **Konsistensi Data (ACID):**
   - Perubahan kalender tarif massal (*bulk update*) wajib dibungkus dalam satu transaksi SQL utuh (all-or-nothing rollback jika salah satu tanggal gagal).
3. **Audit Trail:**
   - Setiap mutasi tarif dan stop-sell mencatat identitas staf (`staff_id`), waktu perubahan, nilai sebelum, dan nilai sesudah.
