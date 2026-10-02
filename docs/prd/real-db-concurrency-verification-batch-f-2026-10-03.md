# Product Requirements Document (PRD) — Batch BE-F: Verifikasi Konkurensi DB Nyata & Integrasi Sistem
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen ID:** `PRD-BATCH-F-REAL-DB-CONCURRENCY-2026-10-03`
- **Versi:** 1.0.0
- **Tanggal:** 3 Oktober 2026
- **Status:** **Approved / Ready for Implementation**
- **Referensi Audit & Gap Trace:** `BE-G20` (Bukti Verifikasi DB dan Payment Nyata), `BE-G17` (Alokasi Kamar Konkuren), `BE-G09` (Idempotensi), `BE-G12` (Otoritas Server Expired Hold).

---

## 1. Latar Belakang & Visi Produk

Sistem backend *Pulang ke Uttara* telah mengimplementasikan seluruh perbaikan modular monolith dari Batch BE-A hingga BE-E. Unit test dan mock test telah membuktikan logika bisnis internal berjalan dengan baik. Namun, berdasarkan standar kualitas perangkat lunak perbankan dan perhotelan internasional (**ISO/IEC 25010: Reliability & Fault Tolerance**, **ISO/IEC 27001 A.12.1.2**), pengujian berbasis *in-memory mock* saja **belum cukup** membuktikan bahwa:
1. Penguncian baris PostgreSQL (`SELECT ... FOR UPDATE` dan `FOR UPDATE OF r SKIP LOCKED`) benar-benar mencegah *double-booking* ketika $N$ pengguna memesan 1 kamar terakhir pada milidetik yang sama.
2. Batasan integritas GiST (`EXCLUDE USING gist`) pada PostgreSQL 18 mencegah dua tamu menempati nomor kamar fisik yang sama pada rentang tanggal yang saling tumpang tindih.
3. Transaksi multi-malam bersifat atomik (*All-or-Nothing*): jika 4 malam tersedia namun malam ke-5 habis, seluruh 4 malam yang sempat didecrement harus di-rollback 100% tanpa menyisakan *dangling inventory hold*.
4. *Race condition* antara worker pembatal hold (`SweepExpiredHolds`) dan pembayaran webhook gateway yang tiba terlambat ditangani secara deterministik tanpa duplikasi status atau inkonsistensi stok kamar.

Batch BE-F hadir untuk menyediakan rangkaian uji integrasi dan verifikasi beban konkurensi nyata (*Real Database Concurrency Test Suite*) yang dapat dijalankan secara terisolasi terhadap PostgreSQL 18 dan Valkey 8.

---

## 2. Persona & Pengguna yang Terdampak

1. **Tamu Publik (`Guest`):**
   - Mendapatkan jaminan 100% bahwa reservasi kamar yang telah dibayar tidak akan dibatalkan akibat *overbooking* sistemik (*zero double-booking guarantee*).
2. **General Manager & Tim Finansial Hotel Pulang ke Uttara:**
   - Memastikan reputasi hotel bintang 4 tetap terjaga tanpa risiko ganti rugi pemindahan tamu (*walked guest*) ke hotel lain akibat sistem overbooking.
3. **Lead Engineer & DevOps:**
   - Memiliki bukti matematis dan metrik eksekusi konkurensi nyata (bukan sekadar asumsi mock) sebelum memberikan lampu hijau (*go-live approval gate*) untuk replacement sistem lama.

---

## 3. Matriks Kepatuhan Standar Industri & Hukum

| Pilar Regulasi / Standar | Klausul & Aturan | Implementasi pada Batch BE-F |
|---|---|---|
| **ISO/IEC 25010** | *Reliability, Fault Tolerance & Concurrency Integrity* | Pengujian $N$ *concurrent workers* (10–50 goroutine) merebutkan 1 kamar terakhir. Tepat 1 transaksi berhasil (*success*), $N-1$ transaksi gagal secara bersih dengan HTTP `409 Conflict` / `INSUFFICIENT_ROOMS`. |
| **ISO/IEC 27001** | *A.12.1.2 Protection against data loss & corruption* | Verifikasi *Atomicity Rollback*: kegagalan reservasi di tengah jalan tidak merusak stok kalender inventaris. |
| **PCI-DSS v4.0 SAQ A** | *Payment Race Condition & Audit Trail* | Simulasi konkurensi antara kedatangan konfirmasi bayar dan kedaluwarsa hold 30 menit. Tidak ada status ambigu (*orphan pending*). |
| **PostgreSQL 18 Standard** | *MVCC, Row-Level Locking & GiST Exclusion* | Verifikasi langsung terhadap PostgreSQL 18 engine dengan checksums dan `io_method=worker`. |
| **Ponytail Rule** | *Anti-Overengineering & Clean Testing* | Menggunakan Go standard library (`testing`, `sync`, `net/http/httptest`) dan `pgxpool`. Tanpa framework testing eksternal yang membengkakkan dependency footprint. |

---

## 4. Kriteria Keberhasilan & Acceptance Criteria

1. **AC-F01 (Race on Last Room):**
   - Pada inventaris kamar dengan stok = 1 pada rentang tanggal tertentu, 20 request booking dikirim serentak via goroutines.
   - **Hasil Wajib:** Tepat 1 reservasi berhasil dibuat (`StatusPending` dengan hold), 19 request lainnya ditolak dengan error `ErrInsufficient` / `INSUFFICIENT_ROOMS`. Sisa inventaris tepat bernilai 0 (tidak pernah bernilai negatif).
2. **AC-F02 (Multi-Night Atomicity Rollback):**
   - Reservasi diajukan untuk 3 malam. Malam 1 dan 2 tersedia, malam 3 stok = 0.
   - **Hasil Wajib:** Transaksi dibatalkan secara atomik. Stok malam 1 dan 2 tetap utuh dan tidak berkurang.
3. **AC-F03 (Concurrent Physical Room Assignment with SKIP LOCKED):**
   - 2 tamu melakukan check-in paralel untuk tipe kamar yang sama di mana terdapat 2 kamar fisik bebas (misal 101 dan 102).
   - **Hasil Wajib:** Kedua transaksi check-in berhasil secara paralel tanpa error `23P01`. Tamu A mendapat kamar 101, Tamu B otomatis mendapat kamar 102 via `SKIP LOCKED`.
4. **AC-F04 (GiST Exclusion Constraint Safety):**
   - Uji pemaksaan alokasi kamar fisik yang sama pada tanggal tumpang tindih (*overlapping date range*).
   - **Hasil Wajib:** Database PostgreSQL menolak transaksi dengan kode SQLSTATE `23P01` (*exclusion_violation*), dan domain memetakannya menjadi `ErrNoRoomAvailable`.
5. **AC-F05 (Hold Expiry vs Payment Race):**
   - Simulasi balapan antara pembersihan hold kedaluwarsa dan konfirmasi pembayaran.
   - **Hasil Wajib:** Jika pembayaran diproses setelah waktu hold lewat, pembayaran ditolak (`HOLD_EXPIRED`) dan sisa inventaris dikembalikan ke kalender.
