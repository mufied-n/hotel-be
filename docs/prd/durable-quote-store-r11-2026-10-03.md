# Product Requirements Document (PRD): Penyimpanan Kuotasi Harga Terdistribusi (Durable Quote Store) (BE-R11)

**Nomor Dokumen:** PRD-PULANG-BE-R11  
**Tanggal Efektif:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Stakeholder Terkait:** Revenue Manager, DevOps / SRE, Hotel Guests, Frontend Engineers  
**Terkait Audit & Gap:** `BE-R11`, `BE-G06`, `F01`, `F05`

---

## 1. Latar Belakang & Konteks Bisnis

Pada mesin pemesanan kamar hotel bintang 4 (*Pulang ke Uttara, Yogyakarta*), calon tamu publik (`guest`) melalui alur *Search* $\rightarrow$ *Select Room & Rate Plan* $\rightarrow$ *Quote Generation* $\rightarrow$ *Guest Detail & Consent* $\rightarrow$ *Checkout / Payment*.

Berdasarkan audit teknis `BE-R11`:
1. **Kerentanan Multi-Instance & Container Restart:**
   Sebelum perbaikan ini, `rates.Engine` menggunakan `MemoryQuoteStore` yang menyimpan data penawaran harga terkunci (*locked quote*) dalam struktur in-memory `map[string]LockedQuote` pada memori lokal satu proses instance.
   Di lingkungan *production* (atau arsitektur kontainer terdistribusi dengan lebih dari 1 replika backend di balik *load balancer* atau saat proses server di-restart/auto-scaled), kuotasi harga yang dibuat oleh Instance A tidak dapat diakses oleh Instance B. Hal ini mengakibatkan kegagalan transaksi acak (*checkout failure*) dengan error `QUOTE_NOT_FOUND` atau `QUOTE_EXPIRED` ketika panggilan `POST /api/v1/bookings` diarahkan ke replika yang berbeda.
2. **Pengabaian Error Penyimpanan (*Swallowed Persistence Error*):**
   Pada implementasi sebelumnya, pemanggilan penyimpanan kuotasi mengabaikan error (`_ = e.quoteStore.SaveQuote(ctx, lq)`). Jika penyimpanan gagal, sistem tetap merespons dengan HTTP 200 OK beserta nomor quote baru. Tamu yang melanjutkan ke tahap pemesanan akan mendapati kuotasi tersebut tidak ditemukan, menciptakan pengalaman pengguna yang buruk dan inkonsistensi transaksi.

Oleh karena itu, diperlukan **Durable Distributed Quote Store** yang memanfaatkan infrastruktur memori terdistribusi Valkey/Redis 8 dengan TTL native 15 menit dan propagasi error fail-closed yang ketat.

---

## 2. Persona Pengguna & Kebutuhan

| Persona | Kebutuhan Utama | Nilai Tambah Fitur |
| :--- | :--- | :--- |
| **Calon Tamu (`guest`)** | Kepastian harga terkunci selama 15 menit tanpa gangguan sesi saat checkout. | Mencegah transaksi gagal acak akibat pergantian instansi backend atau restart container. |
| **Revenue Manager** | Jaminan kepatuhan tarif dan kebijakan pembatalan terkunci secara otoritatif. | Memastikan setiap pemesanan merujuk pada kuotasi resmi yang tersimpan valid, ber-TTL, dan tidak dimanipulasi. |
| **DevOps / SRE** | Kemampuan *horizontal auto-scaling* dan *rolling deployment* tanpa merusak sesi pengguna. | Stateful quotes dipindahkan ke Valkey cluster terpusat; instansi aplikasi backend menjadi *stateless*. |

---

## 3. Matriks Fitur & Aturan Bisnis

1. **Penyimpanan Terdistribusi (Distributed Persistence):**
   - Setiap kuotasi terkunci yang diterbitkan oleh `rates.Engine.CalculateLockedQuote` wajib disimpan ke Valkey/Redis dengan key `hotel:quote:{quote_id}`.
   - Masa berlaku kuotasi (*TTL*) disetel tepat sesuai durasi `ExpiresAt` (default 15 menit / 900 detik).
2. **Ketersediaan Lintas Instansi (Cross-Instance Parity):**
   - Kuotasi yang dibuat oleh replika/instansi manapun (Instance A) wajib dapat dibaca dan dieksekusi secara instan oleh seluruh replika lain (Instance B, C, dst.).
   - Restart pada proses backend tidak boleh menghapus kuotasi aktif yang belum kedaluwarsa di Valkey.
3. **Propagasi Error Fail-Closed:**
   - Jika operasi penyimpanan ke Valkey/Redis gagal (koneksi terputus, timeout, dsb.), pembuatan kuotasi **WAJIB GAGAL** dan mengembalikan status HTTP 500 `INTERNAL_ERROR`. Sistem dilarang keras mengembalikan kuotasi sukses palsu (*phantom quote*) kepada tamu.
4. **Pencegahan Requote Tak Terotorisasi:**
   - Setelah kedaluwarsa (TTL habis), pemesanan dengan `quote_id` tersebut wajib ditolak dengan HTTP 400 `QUOTE_EXPIRED`.
   - Data kuotasi di-serialize dengan format JSON lengkap mencakup rincian harga kamar, tarif sarapan, diskon promo, pajak, breakdown malam, dan snapshot komposisi tamu.

---

## 4. Kriteria Keberhasilan (Acceptance Criteria)

- [x] **AC-01:** Kuotasi yang dibuat melalui `POST /api/v1/quotes` tersimpan secara atomik di Valkey dengan TTL 15 menit.
- [x] **AC-02:** Instansi backend kedua yang terhubung ke Valkey yang sama dapat memvalidasi dan memproses checkout menggunakan `quote_id` dari instansi pertama tanpa hambatan.
- [x] **AC-03:** Kuotasi tetap bertahan dan valid setelah proses backend pertama dihentikan/direstart sebelum masa berlaku 15 menit habis.
- [x] **AC-04:** Kegagalan persistensi penyimpanan kuotasi menghasilkan respons error HTTP 500 dan membatalkan penerbitan quote.
- [x] **AC-05:** Permintaan pemesanan dengan kuotasi yang telah habis masa berlakunya di Valkey secara tegas ditolak dengan error `QUOTE_EXPIRED`.
- [x] **AC-06:** Test coverage modul `internal/rates` bertahan $\ge 80\%$ dan seluruh skenario terverifikasi via automated E2E test.
