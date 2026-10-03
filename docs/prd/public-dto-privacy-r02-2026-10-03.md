# Product Requirements Document (PRD) — Public DTO Privacy & Token Protection (BE-R02)

**Fitur:** Public DTO Privacy, Sensitive Free-Text & Token Protection  
**ID Gap:** BE-R02 (P1)  
**Terkait:** BE-G13, F02/F03  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  
**Target:** Monolith Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)  

---

## 1. Konteks Bisnis & Masalah

Pada sistem pemesanan hotel bintang 4 *Pulang ke Uttara*, tamu sering menyertakan catatan khusus pada saat reservasi (`special_requests`), seperti preferensi kamar (misal: "dekat lift karena disabilitas fisik"), kondisi kesehatan (misal: "alergi kacang ekstrem", "membutuhkan tempat tidur hypoallergenic"), atau catatan perayaan pribadi ("setup honeymoon"). Selain itu, tamu mengisi estimasi waktu kedatangan (`estimated_arrival_time`).

Data bebas (*free-text*) dan preferensi ini diklasifikasikan sebagai data pribadi sensitif menurut **UU Perlindungan Data Pribadi (UU PDP No. 27/2022)** dan panduan privasi perhotelan internasional (GDPR Art. 9).

### Temuan Masalah (BE-R02)
1. **Struktur `PublicDTO`:** Struct `PublicDTO` sebelumnya masih memuat definisi field `estimated_arrival_time` dan `special_requests`. Meskipun `ToPublicDTO()` mengosongkannya, keberadaan field tersebut dalam schema publik memaparkan ekspektasi dan risiko regresi kebocoran data.
2. **Eksposur `guest_token` pada pembacaan Staf:** Saat staf (`receptionist`, `gm_admin`, dsb.) melakukan `GET /api/v1/bookings/:id`, seluruh objek `Booking` dikembalikan termasuk `guest_token`. Kredensial rahasia sesi tamu tidak boleh diakses oleh staf internal (prinsip *least privilege* & *credential isolation*).
3. **Validasi Kepemilikan (Ownership):** Caller tanpa token atau dengan token yang tidak valid/salah tidak boleh menerima informasi PII apa pun maupun rincian preferensi tamu.

---

## 2. Persona & Hak Akses

| Persona | Hak Akses `GET /api/v1/bookings/:id` | Rincian Response |
|---|---|---|
| **Publik / Tamu Tanpa Token** | Read Public Status | `PublicDTO` murni: ID, RoomTypeID, CheckIn, CheckOut, NumRooms, Status, ExpiresAt, CreatedAt. Tidak ada nama, kontak, harga, free-text, maupun token. |
| **Tamu dengan Token Salah / Milik Booking Lain** | Read Public Status | Mengembalikan `PublicDTO` yang sama (tidak membocorkan keberadaan token atau data pemesan). |
| **Tamu Pemilik (Valid `X-Guest-Token`)** | Read Own Booking | Booking lengkap milik sendiri (nama, kontak, special requests, arrival time), namun `guest_token` tidak dikembalikan ulang pada response GET. |
| **Staf Hotel (`receptionist`, `gm_admin`, dll)** | Read Stay / Operations | Booking lengkap operasional (nama, kontak, kamar, special requests, arrival time untuk melayani tamu), namun `guest_token` dihapus (`guest_token: ""`). |

---

## 3. Kriteria Penerimaan (Acceptance Criteria)

- [x] **AC-01 (Stripped PublicDTO):** Struct `PublicDTO` tidak memiliki field `special_requests` dan `estimated_arrival_time` sama sekali. Response JSON untuk akses publik hanya memuat metadata publik minimal.
- [x] **AC-02 (Staff Read Token Redaction):** Endpoint `GET /api/v1/bookings/:id` tidak pernah mengembalikan `guest_token` kepada staf hotel (`b.GuestToken = ""`).
- [x] **AC-03 (Guest Read Token Redaction):** Endpoint `GET /api/v1/bookings/:id` untuk tamu terautentikasi tidak menyertakan `guest_token` dalam response JSON (`omitempty`). `guest_token` hanya diterbitkan pada respons inisiasi `POST /api/v1/bookings`.
- [x] **AC-04 (Negative Ownership Test):** Caller dengan token acak atau token milik booking lain hanya menerima `PublicDTO` tanpa marker sensitif dari `special_requests`.
- [x] **AC-05 (Staff Operational Utility):** Staf tetap dapat membaca `special_requests` dan `estimated_arrival_time` untuk keperluan pelayanan operasional hotel tanpa kebocoran kredensial.
