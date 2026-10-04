# Product Requirements Document (PRD)
# Official PDF Confirmation Voucher & PBJT Tax Invoice Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Dokumen Identitas:** `PRD-F04-PDF-VOUCHER-INVOICE-2026-10-04`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status:** APPROVED FOR SPECIFICATION
- **Dokumen Pasangan:**
  - SRS: [`docs/srs/official-pdf-voucher-and-tax-invoice-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/official-pdf-voucher-and-tax-invoice-2026-10-04.md)
  - Arsitektur Teknis: [`docs/tech/official-pdf-voucher-and-tax-invoice-architecture-2026-10-04.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/official-pdf-voucher-and-tax-invoice-architecture-2026-10-04.md)
- **Target Role Pengguna:** `guest`, `receptionist`, `finance`, `gm_admin`

---

## 1. Latar Belakang & Urgensi Bisnis

Saat ini, sistem booking Pulang ke Uttara telah berhasil menerbitkan data konfirmasi pemesanan dalam format JSON receipt dan file kalender `.ics`. Namun, dalam konteks industri perhotelan bintang 4 di Indonesia (khususnya wilayah Sleman / DI Yogyakarta), dokumen digital mentah tidak mencukupi untuk kebutuhan operasional hotel dan tamu:

### 1.1 Masalah Saat Ini (Current State Gaps)
1. **Ketiadaan Bukti Pemesanan Resmi (Official Voucher PDF):**
   Tamu hotel (terutama segmen keluarga, wisatawan domestik/mancanegara, dan pelancong bisnis) membutuhkan satu berkas PDF resmi yang ringkas, elegan, dan siap dicetak/disimpan di smartphone secara offline untuk ditunjukkan saat tiba di resepsionis meja depan.
2. **Ketiadaan Faktur Pajak Daerah Resmi (PBJT / PB1 Folio):**
   Sebagai hotel bintang 4 di Kabupaten Sleman, transaksi hotel tunduk pada **Pajak Barang dan Jasa Tertentu (PBJT) sebesar 10%** dan **Service Charge sebesar 10%** (berdasarkan Perda Kabupaten Sleman No. 7 Tahun 2023). Tamu instansi pemerintah (Kementerian/BUMN) dan korporat yang menghadiri kegiatan akademik atau dinas di Yogyakarta **wajib melampirkan Faktur Resmi ber-NPWPD** untuk proses pencairan dana dinas (*reimbursement / SPPD*). Ketiadaan PDF faktur otomatis memaksa tamu meminta cetak faktur manual di resepsionis saat check-out, memicu antrean panjang.
3. **Ketiadaan Verifikasi Cepat (Fast Verification / Anti-Fraud):**
   Voucher manual atau tangkapan layar rentan terhadap pemalsuan (seperti manipulasi tanggal atau tipe kamar). Resepsionis membutuhkan mekanisme verifikasi cepat via **QR Code terenkripsi digital** yang dicetak pada voucher untuk melakukan *instant lookup* ke sistem Front Desk saat proses *Express Check-in*.

### 1.2 Dampak Bisnis yang Diharapkan (Expected Outcomes)
* **Pengurangan Waktu Antrean Check-In & Check-Out sebesar 50%:** Tamu dapat mengunduh faktur pajak dan voucher secara mandiri tanpa membebani staf meja depan.
* **100% Kepuasan Tamu Korporat & Pemerintah:** Memenuhi standar pelaporan keuangan resmi dan SPPD instansi dengan rincian DPP, Service Charge 10%, dan PBJT 10% yang transparan.
* **Pencegahan Penipuan Voucher:** Setiap voucher memiliki QR Code berbasis tanda tangan kriptografis HMAC-SHA256 yang tervalidasi secara aman.

---

## 2. Profil Persona & Matriks Hak Akses (RBAC Matrix)

| Persona | Peran & Kebutuhan | Hak Akses Fitur (Casbin Subject) |
| :--- | :--- | :--- |
| **Tamu Publik (`guest`)** | Mengunduh dokumen Voucher PDF dan Faktur Pajak resmi untuk reservasi miliknya via portal *Booking Saya*. | Read-only dokumen PDF untuk reservasi yang dimilikinya (berdasarkan verifikasi email sesi atau `X-Guest-Token`). |
| **Resepsionis Meja Depan (`receptionist`)** | Memindai QR Code voucher fisik/digital tamu untuk memverifikasi data dan mempercepat proses check-in. | Read/Verify reservasi berdasarkan referensi booking QR Code. |
| **Finance Officer (`finance`)** | Mengunduh faktur pajak resmi untuk keperluan rekonsiliasi pembayaran dan pelaporan PBJT bulanan. | Read seluruh dokumen faktur booking hotel. |
| **General Manager (`gm_admin`)** | Mengaudit format legalitas dokumen dan keabsahan NPWPD properti. | Full Control & Audit. |

### Matriks Otorisasi Endpoint

| Route HTTP | Method | Role yang Diizinkan | Keterangan Kebijakan |
| :--- | :---: | :--- | :--- |
| `/api/v1/guest/bookings/:id/voucher.pdf` | `GET` | `guest` (pemilik), `receptionist`, `gm_admin` | Unduh dokumen resmi Voucher Booking PDF |
| `/api/v1/guest/bookings/:id/invoice.pdf` | `GET` | `guest` (pemilik), `finance`, `gm_admin` | Unduh Faktur Pajak Resmi PBJT & Service Charge |
| `/api/v1/front-desk/verify-voucher` | `GET` | `receptionist`, `gm_admin` | Validasi tanda tangan QR Code voucher saat tamu check-in |

---

## 3. Komponen Desain Dokumen PDF

### 3.1 Official Booking Confirmation Voucher
1. **Header Properti:**
   - Logo resmi *Hotel Pulang ke Uttara* (bintang 4).
   - Alamat: Jl. Kaliurang Km 5, Sleman, D.I. Yogyakarta 55281.
   - Kontak: Telepon Concierge & WhatsApp Meja Depan (+62 274 ...).
2. **Metadata Reservasi:**
   - Kode Referensi Booking (misal: `PKU-202610-8849`).
   - Tanggal Reservasi Dibuat & Status Pembayaran (`PAID / GUARANTEED`).
   - Nama Lengkap Tamu Utama, Nomor Kontak, dan Email.
3. **Detail Inap & Kamar:**
   - Tipe Kamar (misal: *Deluxe King Bay Window*).
   - Paket: *Room with Breakfast for 2 Persons*.
   - Jadwal: Check-in (`14:00 WIB`), Check-out (`12:00 WIB`), Durasi (`2 Malam`).
   - Jumlah Tamu: `2 Dewasa, 1 Anak`.
4. **QR Code Verifikasi Cepat:**
   - Memuat token bertanda tangan `HMAC-SHA256` untuk verifikasi instan di meja depan.
5. **Kebijakan & Informasi Hotel:**
   - Jam Sarapan: 06:00 – 10:00 WIB di Restoran Hotel.
   - Aturan bebas rokok (*100% Non-Smoking Rooms*).
   - Kebijakan pembatalan dan kontak darurat.

### 3.2 Official Tax Invoice (Faktur PBJT Hotel)
1. **Legalitas Wajib Pajak:**
   - Nama Perusahaan Pengelola: PT Pulang Uttara Sejahtera.
   - NPWPD (Nomor Pokok Wajib Pajak Daerah): `01.234.567.8-542.000` (Kabupaten Sleman).
   - Nomor Faktur Seri: `INV/PKU/202610/00482`.
2. **Tabel Rincian Keuangan (Rupiah Exponent 0):**
   - Biaya Kamar Bersih (Net Room Charges / DPP): `IDR 1.859.504`
   - Biaya Layanan Hotel (Service Charge 10%): `IDR 185.950`
   - Pajak Barang dan Jasa Tertentu (PBJT Sleman 10%): `IDR 204.546`
   - Total Dibayarkan (*Total Amount Paid*): `IDR 2.250.000`
3. **Status Pelunasan:**
   - Metode Pembayaran: *Xendit Virtual Account (Bank Mandiri)*.
   - Waktu Pelunasan: `04 Okt 2026, 09:15:22 WIB`.
   - Stempel Digital Hotel: *PAID & VERIFIED*.

---

## 4. Kriteria Keberhasilan & Penerimaan (Acceptance Criteria)

### AC-01: Kecepatan dan Efisiensi Pembangkitan PDF
* **Given** reservasi terkonfirmasi dengan status `CONFIRMED`.
* **When** endpoint `/api/v1/guest/bookings/:id/voucher.pdf` dipanggil.
* **Then** sistem mengalirkan (*stream*) file PDF lengkap dengan logo dan QR code dalam waktu $\le 50\text{ ms}$ dengan penggunaan memori $\le 2\text{ MB}$.

### AC-02: Isolasi Hak Akses dan Kepemilikan Dokumen
* **Given** tamu B mencoba mengunduh voucher milik tamu A tanpa token sesi yang sah.
* **When** tamu B memanggil endpoint PDF untuk booking ID milik tamu A.
* **Then** sistem menolak dengan kode status `401 UNAUTHORIZED` atau `403 FORBIDDEN` (data PII terlindungi).

### AC-03: Kepatuhan Format Faktur PBJT Kabupaten Sleman
* **Given** reservasi telah lunas (`PAID`).
* **When** faktur `/api/v1/guest/bookings/:id/invoice.pdf` diunduh.
* **Then** nilai DPP, Service Charge 10%, dan PBJT 10% dihitung secara presisi tanpa desimal, dan totalnya identik dengan nominal yang dibayarkan tamu pada gateway pembayaran.

### AC-04: Verifikasi QR Code Meja Depan
* **Given** voucher PDF yang memuat QR Code resmi.
* **When** resepsionis memindai QR Code tersebut.
* **Then** endpoint verifikasi memvalidasi tanda tangan HMAC-SHA256 dan mengembalikan data reservasi asli dalam waktu $\le 10\text{ ms}$. Jika isi QR dimanipulasi, sistem menolak dengan pesan `INVALID_SIGNATURE`.

---

## 5. Kebutuhan Non-Fungsional

1. **Zero-CGO & Portable:** Pembangkitan PDF wajib murni ditulis dalam Go tanpa dependensi binary eksternal seperti libwkhtmltox atau headless Chrome.
2. **Zero Disk I/O:** Dokumen PDF dialirkan langsung dari memori via `io.Writer` ke HTTP stream respon Gin tanpa menulis berkas temporer di filesystem server.
3. **MIME & Cache Control:** Respon HTTP menyertakan header `Content-Type: application/pdf`, `Content-Disposition: inline`, dan `Cache-Control: private, no-store`.
