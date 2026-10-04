# Product Requirements Document (PRD)
# Last-Room Hospitality Safeguards: LRDA Safety Buffer, Dynamic Hold & 1-Click Upgrade Resolver

- **Fitur ID:** `FEAT-LAST-ROOM-SAFEGUARDS`
- **Tanggal Efektif:** 4 Oktober 2026
- **Status Dokumen:** APPROVED / IN IMPLEMENTATION
- **Penanggung Jawab:** Lead Systems Architect & Principal Hospitality Engineer
- **Target Properti:** Hotel Pulang ke Uttara (95 Kamar, Sleman, D.I. Yogyakarta)

---

## 1. Latar Belakang & Konteks Bisnis (*Business Background*)

Pada pengoperasian hotel butik bintang 4 dengan 95 kamar seperti **Pulang ke Uttara**, kondisi **"Kamar Tinggal 1" (*The Last Room Availability / Critical Inventory State*)** adalah momen paling krusial yang menentukan profitabilitas, kepuasan tamu, dan integritas operasional hotel. 

Meskipun fondasi transaksional database telah aman dari *double booking* berkat PostgreSQL *Row-Level Lock* (`SELECT ... FOR UPDATE`), terdapat 4 friksi bisnis mendesak di industri perhotelan bintang 4:
1. **Kerugian Margin Komisi OTA (15% - 20%):** Kamar terakhir di saat *high season* atau *weekend* adalah aset bernilai tertinggi. Menjual kamar terakhir ke OTA seperti Agoda/Traveloka memotong pendapatan hotel sebesar 15-20% komisi, padahal kamar tersebut dapat dijual dengan harga penuh tanpa potongan komisi kepada tamu *direct booking* (website resmi) atau tamu *walk-in* di meja depan.
2. **Bahaya Sandera "Ghost Hold" (Denial-of-Inventory):** Hold reservasi reguler selama 30 menit sangat berisiko saat kamar tinggal 1. Calon tamu anonim di internet dapat menahan kamar terakhir selama 30 menit tanpa membayar, menyebabkan tamu *walk-in* di lobi hotel ditolak. Ketika hold kadaluarsa, hotel kehilangan calon tamu di lobi yang sudah siap bayar.
3. **Ketiadaan Sinyal Proaktif ke OTA (Asymmetric Two-Way Sync):** Saat kamar terakhir terjual via *direct web*, OTA eksternal tidak langsung mengetahui kondisi habisnya alokasi jika sistem hanya menunggu *inbound polling*, memicu gelombang webhook reservasi gagal (*overbooking conflict*).
4. **Insiden Tamu Terlantar di Lobi Hotel:** Ketika terjadi *allotment conflict* dari OTA dan reservasi masuk ke karantina, tamu OTA mungkin sudah dalam perjalanan ke Yogyakarta membawa voucher konfirmasi. SOP bintang 4 melarang penolakan kasar di lobi (*guest walk-away*); solusi elegan adalah **Complimentary Room Upgrade** ke tipe kamar lebih tinggi yang masih tersedia (misal dari Superior ke Deluxe).

Fitur ini mengadopsi standar sistem PMS kelas dunia (**Opera Cloud, Mews, SiteMinder, dan Cloudbeds**) yang disesuaikan secara ringkas dan bebas over-engineering (*Ponytail-compliant*).

---

## 2. Persona Pengguna & Matriks Kebutuhan

| Persona | Peran & Tanggung Jawab | Kebutuhan Utama pada Kamar Terakhir |
| :--- | :--- | :--- |
| **Direct Guest (Tamu Web)** | Tamu publik yang memesan melalui situs resmi pulangkeuttara.id | Mendapatkan prioritas akses pada kamar terakhir; mendapatkan kejelasan waktu checkout pembayaran yang adil (15 menit). |
| **Walk-in Guest** | Tamu fisik yang datang langsung ke meja depan di Jl. Kaliurang | Tidak tertolak sia-sia oleh reservasi web yang tidak dibayar (*ghost holds*). |
| **Receptionist (Meja Depan)** | Staf garda depan operasional 24 jam | Menerima peringatan instan saat kamar tinggal 1; mampu melakukan resolusi *1-Click Free Upgrade* bagi tamu OTA yang terdampak karantina overbooking. |
| **Revenue Manager** | Pengelola alokasi kamar, harga, dan OTA | Menerapkan *Safety Buffer* (default 1 kamar) untuk memotong pasokan ke OTA saat stok kritis demi mengamankan margin profit direct 100%. |
| **OTA Partner (Agoda/Traveloka)** | Kanal distribusi pihak ketiga terhubung via Webhook | Menerima sinyal *Outbound Stop-Sell* saat kuota alokasi habis; mendapatkan respons status yang jelas (409 Conflict) jika memaksakan pemesanan di luar alokasi. |

---

## 3. Matriks Fitur & Aturan Bisnis (*Business Rules*)

### BR-01: Last-Room Direct Allocation (LRDA / Safety Stock Buffer)
1. Setiap mitra kanal (`channel_partners`) memiliki konfigurasi `safety_buffer` (integer, default: 1).
2. Ketika ketersediaan stok fisik kamar suatu tipe pada tanggal yang diminta bernilai $\le \text{safety\_buffer}$:
   - Akses reservasi dari kanal eksternal (OTA) **ditutup secara otomatis (*Stop-Sell*)**.
   - Permintaan webhook OTA ditolak dengan HTTP `409 Conflict` (`ALLOTMENT_EXHAUSTED`) dan event otomatis dimasukkan ke tabel karantina `channel_sync_issues`.
   - Kamar terakhir tersebut **100% diproteksi khusus untuk Direct Booking (Web Tamu Resmi dan Front Desk Walk-in)**.
3. Revenue Manager dapat menyesuaikan nilai `safety_buffer` per mitra (misal Agoda = 1, Traveloka = 1, Direct = 0).

### BR-02: Dynamic Last-Room Hold Timeout (Anti-Ghost Booking)
1. Untuk pemesanan direct di portal tamu:
   - Jika untuk seluruh malam yang dipesan ketersediaan kamar $\ge 2$: durasi hold pembayaran adalah standar **30 menit**.
   - Jika terdapat setidaknya 1 malam di mana ketersediaan kamar $\le 1$ (*Critical Last Room*): durasi hold pembayaran otomatis dipersingkat menjadi **15 menit**.
2. Pengurangan durasi hold ini tercatat pada field `expires_at` pemesanan dan ditransmisikan ke portal tamu agar hitung mundur (*countdown timer*) menyesuaikan secara akurat.

### BR-03: Outbound Stop-Sell Dispatcher (Proactive Two-Way Channel Broadcast)
1. Setiap kali terjadi pemotongan stok kamar yang menyebabkan sisa kamar menyentuh $\le \text{safety\_buffer}$:
   - Sistem menerbitkan event transaksional outbox `channel.outbound_stopsell`.
   - Background dispatcher mengirimkan notifikasi HTTP Webhook ke endpoint mitra OTA:
     ```json
     {
       "event": "channel.inventory.stop_sell",
       "provider": "AGODA",
       "room_type_id": "01900000-0000-7000-8000-000000000001",
       "date": "2026-10-10",
       "allotment": 0,
       "reason": "SAFETY_BUFFER_REACHED"
     }
     ```
2. Mencegah OTA menerima pesanan baru dari sisi aplikasi mereka, menekan insiden overbooking hingga 99%.

### BR-04: 1-Click Complimentary Upgrade Resolution Pathway
1. Insiden overbooking yang masuk ke `channel_sync_issues` dapat diselesaikan oleh staf berwenang (`receptionist`, `revenue_mgr`, `gm_admin`) melalui endpoint:
   `POST /api/v1/staff/channel-sync-issues/:id/resolve`
2. Pilihan aksi resolusi:
   - `COMPLIMENTARY_UPGRADE`: Memindahkan reservasi overbooking ke tipe kamar yang lebih tinggi (misal dari Superior ke Deluxe/Executive) tanpa mengenakan biaya tambahan kepada tamu.
   - `REJECT_AND_CANCEL`: Membatalkan reservasi overbooking dan menginstruksikan OTA untuk merelokasi tamu.
   - `FORCE_OVERBOOK_CONFIRMED`: Menyetujui overbooking fisik dengan persetujuan General Manager (misal membuka kamar cadangan manajemen).
3. Jika aksi adalah `COMPLIMENTARY_UPGRADE`:
   - Sistem secara atomik memverifikasi dan memotong stok kamar pada `target_room_type_id`.
   - Status isu diperbarui menjadi `RESOLVED`.
   - Mencatat identitas staf pengeksekusi (`resolved_by`) dan waktu resolusi (`resolved_at`).

---

## 4. Kriteria Keberhasilan & Penerimaan (*Acceptance Criteria*)

- [x] **AC-01 (Safety Buffer Enforcement):** OTA yang mencoba memesan saat stok kamar = 1 otomatis ditolak HTTP 409 dan dikarantina, sementara Tamu Web Direct pada detik yang sama berhasil melakukan booking (Hold).
- [x] **AC-02 (Dynamic Hold Time):** Reservasi langsung pada kamar dengan stok = 1 memiliki `expires_at` tepat 15 menit sejak pembuatan (bukan 30 menit).
- [x] **AC-03 (Resolution Pathway):** Endpoint resolusi `POST /api/v1/staff/channel-sync-issues/:id/resolve` dengan aksi `COMPLIMENTARY_UPGRADE` berhasil memotong alokasi kamar target secara atomik dan mencatat audit trail staf.
- [x] **AC-04 (RBAC Protection):** Staf tanpa izin (seperti `housekeeping` atau `guest`) ditolak HTTP 403 Forbidden saat mencoba mengeksekusi resolusi isu sinkronisasi.
- [x] **AC-05 (Zero Lint & Race Free):** Seluruh kode baru lolos `go vet ./...`, `go test -race ./...`, dan mencapai unit test coverage $\ge 80\%$.
