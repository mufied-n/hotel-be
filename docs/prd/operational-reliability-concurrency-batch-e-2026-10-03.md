# PRD — Batch BE-E: Keandalan Operasional, Konkurensi & Observabilitas
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status:** **APPROVED / READY FOR IMPLEMENTATION**
- **Ruang Lingkup:** Keandalan Outbox Worker (`BE-G16`), Alokasi Kamar Paralel Bebas Konflik (`BE-G17`), Observabilitas & Graceful Shutdown (`BE-G21`), serta Kebijakan Early Checkout & No-Show Cutoff (`BE-G22`).

---

## 1. Konteks Bisnis & Latar Belakang

Hotel Pulang ke Uttara adalah hotel butik bintang 4 di kawasan Jl. Kaliurang, DI Yogyakarta dengan 95 kamar fisik terbagi ke dalam 7 varian kamar eksklusif. Pada jam-jam sibuk (*peak check-in window* 14:00–16:00 WIB) atau saat kedatangan grup wisata (*group tour*), meja resepsionis beroperasi serentak melalui beberapa terminal resepsionis.

Untuk menjamin kepuasan tamu dan pendapatan optimal (*revenue maximization*), sistem harus:
1. **Mencegah False Allocation Failure pada Check-In Paralel (`BE-G17`):** Dua resepsionis yang melakukan check-in bersamaan untuk tipe kamar yang sama tidak boleh saling menggagalkan satu sama lain ketika masih ada kamar fisik lain yang tersedia.
2. **Durabilitas Pengiriman Notifikasi & Outbox (`BE-G16`):** Menjamin setiap konfirmasi reservasi dan kuitansi pembayaran terkirim secara andal (at-least-once dengan deduplikasi efek samping) tanpa menahan transaksi database utama.
3. **Observabilitas Sistem & Graceful Shutdown (`BE-G21`):** Mencegah *abrupt termination* saat deployment atau rolling restart, menangani error query sweep, dan validasi konfigurasi *fail-fast*.
4. **Resale Inventory pada Early Check-Out & No-Show (`BE-G22`):** Ketika tamu check-out lebih awal dari jadwal semula, sistem secara otomatis mengembalikan sisa malam yang belum terpakai ke inventaris hotel agar dapat dijual kembali (*inventory recovery/resale*). Penetapan status *no-show* memiliki batas waktu wajar (mulai tanggal check-in).

---

## 2. Standar Kepatuhan Hukum, Industri & Engineering

1. **Standar Keandalan Data & Finansial (ISO/IEC 27001 & ACID):**
   - Transaksi outbox memisahkan dispatch I/O dari DB transaction lock untuk menjaga SLA response time (<100ms).
   - Pencegahan *phantom reservation* dan *race condition* dengan PostgreSQL `FOR UPDATE SKIP LOCKED`.
2. **Standar Industri Perhotelan (HTNG & PHRI Bintang 4):**
   - Penanganan *Early Departure / Early Check-out*: Sisa malam menginap di-restitusi ke stok kamar aktif untuk dijual kepada tamu *walk-in* atau pemesanan last-minute.
   - Penanganan *No-Show*: Status *no-show* hanya dapat dieksekusi setelah waktu check-in resmi (14:00 WIB pada hari H). Pembatalan sebelum hari H diklasifikasikan sebagai pembatalan biasa (*cancellation*), bukan no-show.
3. **Standar Keamanan API (OWASP API Security Top 10):**
   - API8 (Lack of Protection from Automated Threats): Graceful shutdown dan rate-limiting melindungi ketersediaan service (*availability*).
   - Sanitasi logging: Alamat email dan nomor telepon tidak dicatat secara plaintext dalam terminal log relay outbox.
4. **Prinsip Anti-Overengineering (Ponytail):**
   - Memaksimalkan fitur bawaan PostgreSQL (`FOR UPDATE OF r SKIP LOCKED`, `NOT EXISTS`) tanpa menambahkan distributed lock server eksternal (etcd/Zookeeper).

---

## 3. Matriks Kebutuhan Fitur (Feature Requirements)

| Kode Gap | Nama Fitur | Peran Pengguna | Deskripsi Kebutuhan |
|---|---|---|---|
| **BE-G16** | Outbox Relay Durability & Deduplication | System Worker / Guest | Pemrosesan outbox berulang aman dari replikasi email, penanganan dead-letter eksplisit, dan bounded backoff. |
| **BE-G17** | Non-Blocking Parallel Room Assignment | `receptionist`, `gm_admin` | Alokasi kamar fisik saat check-in menggunakan `FOR UPDATE OF r SKIP LOCKED` sehingga konkurensi multi-resepsionis tidak saling menggagalkan. |
| **BE-G21** | Observability & Graceful Shutdown | DevOps / SRE | Evaluasi terminal `rows.Err()` pada background sweep, validasi konfigurasi fail-fast, dan shutdown koordinatif worker. |
| **BE-G22** | Early Check-Out Inventory Restitution & No-Show Cutoff | `receptionist`, `revenue_mgr` | Pelepasan inventaris sisa malam saat early check-out; penolakan mark no-show sebelum tanggal check-in tiba. |

---

## 4. Kriteria Keberhasilan (Acceptance Criteria)

1. **AC-E01 (Parallel Check-In Concurrency):** Dua permintaan check-in simultan untuk tipe kamar yang sama berhasil menetapkan kamar fisik yang berbeda (misal 101 dan 102) secara mulus tanpa error 500 atau 23P01 exclusion violation.
2. **AC-E02 (Early Check-Out Inventory Resale):** Pemesanan 3 malam yang melakukan check-out pada malam ke-1 secara otomatis mengembalikan 2 malam tersisa ke inventaris tipe kamar tersebut.
3. **AC-E03 (No-Show Guard):** Percobaan menandai no-show untuk tanggal reservasi di masa depan ditolak dengan error `400 Bad Request` (`NO_SHOW_TOO_EARLY`).
4. **AC-E04 (Sweep & Outbox Robustness):** Background sweep memvalidasi `rows.Err()`; relay outbox menolak konfigurasi interval $\le 0$ dan mencatat *dead_letter* saat retry mencapai batas maksimum.
5. **AC-E05 (Code Coverage & Quality):** Seluruh unit test table-driven mencapai $\ge 80\%$ statement coverage dengan 0 lint/vet issue.
