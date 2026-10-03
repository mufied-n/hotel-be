# PRD: Penegakan Invariant Kapasitas Kamar Katalog pada Search, Quote, dan Checkout (BE-R07)

**Nomor Dokumen:** PRD-PULANG-BE-R07  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R07, BE-G03, F01, F08  

---

## 1. Konteks Bisnis & Latar Belakang

Hotel "Pulang ke Uttara" di Yogyakarta memiliki 95 kamar fisik dengan 7 varian kamar komersial:
- Superior King / Superior Twin (Max Capacity: 3, Max Adults: 2, Max Children: 1)
- Deluxe King / Deluxe Twin (Max Capacity: 3, Max Adults: 2, Max Children: 1)
- Executive King (Max Capacity: 3, Max Adults: 2, Max Children: 1)
- Junior Suite (Max Capacity: 4, Max Adults: 2, Max Children: 2)
- Presidential Suite (Max Capacity: 6, Max Adults: 4, Max Children: 2)

### Permasalahan (Gap BE-R07)
Sebelum perbaikan ini:
1. Endpoint `GET /api/v1/search` (`searchRooms`) mengecek batas kapasitas agregat namun tidak memvalidasi `MaxChildren` pada varian kamar, tidak memvalidasi bahwa jumlah `child_ages` sama dengan `children`, serta tidak memvalidasi `adults < rooms` pada level parameter search (sehingga menghasilkan respons tidak konsisten).
2. Endpoint `POST /api/v1/quotes` (`calculateQuote`) sama sekali tidak memvalidasi kapasitas terhadap data katalog (`d.CatalogStore`). Klien dapat meminta quote untuk 10 tamu di 1 kamar Superior King.
3. Endpoint `POST /api/v1/bookings` (`createBooking`) dan domain core `booking.Service.Create` hanya memeriksa bahwa `num_rooms` antara 1-8 dan `num_guests >= 1`, tanpa membaca kapasitas fisik varian kamar dari katalog. Caller yang mem-bypass search/quote dapat memesan kamar melampaui kapasitas fisik hotel (*overcapacity*).

Hal ini melanggar standar keselamatan hotel berbintang 4 dan aturan operasional hospitality Yogyakarta.

---

## 2. Persona & Pengguna Terdampak

1. **Guest (Tamu Publik):**
   - Mendapatkan kepastian bahwa kamar yang dicari, dihitung harganya (quote), dan dipesan memenuhi batas kapasitas aman dan legal hotel.
   - Tidak dapat memesan kamar tanpa jumlah dewasa minimal 1 per kamar (anak-anak tidak diizinkan menginap tanpa pendamping dewasa).
   - Memasukkan data usia anak (`child_ages`) yang konsisten dengan jumlah anak yang diinput.

2. **Front Desk & Receptionist:**
   - Terbebas dari friksi saat tamu check-in membawa tamu melebihi kapasitas fisik tempat tidur dan ruang kamar.
   - Mencegah sengketa biaya tambahan (*extra bed charge*) yang tidak terduga karena pesanan overcapacity.

3. **General Manager & Revenue Manager:**
   - Menjaga integritas inventori kamar dan kepatuhan terhadap regulasi keselamatan kebakaran serta batas hunian legal hotel bintang 4.

---

## 3. Matriks Aturan Kapasitas & Invariant

| Variabel | Aturan Validasi | Kode Error / HTTP Status |
| :--- | :--- | :--- |
| `rooms` / `num_rooms` | Integer antara 1 hingga 8 | `400 INVALID_ROOM_COUNT` |
| `adults` | Integer $\ge 1$ dan $\ge$ `rooms` (minimal 1 dewasa per kamar) | `400 INVALID_GUEST_COUNT` |
| `children` | Integer $\ge 0$ | `400 INVALID_GUEST_COUNT` |
| `child_ages` | Jika `children == 0`: tidak boleh diisi.<br>Jika diisi: jumlah elemen harus tepat sama dengan `children`.<br>Setiap umur harus antara 0 hingga 17 tahun. | `400 CHILD_AGE_COUNT_MISMATCH`<br>`400 INVALID_CHILD_AGE` |
| Okupansi Kamar Search | Untuk tiap varian `v`:<br>- `adults <= v.MaxAdults * rooms`<br>- `children <= v.MaxChildren * rooms`<br>- `(adults + children) <= v.MaxCapacity * rooms` | Bila melanggar: `item.Available = false`, `item.UnavailableReason = "EXCEEDS_CAPACITY"` |
| Quote Okupansi | Validasi terhadap `CatalogStore.GetVariant(room_type_id)`:<br>- `num_guests <= v.MaxCapacity * num_rooms`<br>- Bila adults/children dikirim: periksa `MaxAdults` dan `MaxChildren` | `400 EXCEEDS_CAPACITY` |
| Booking Okupansi | Validasi ganda pada handler API dan domain `booking.Service.Create`:<br>- `num_guests <= v.MaxCapacity * num_rooms`<br>- `num_guests >= num_rooms` | `400 EXCEEDS_CAPACITY`<br>`400 INVALID_GUEST_COUNT` |

---

## 4. Kriteria Penerimaan (Acceptance Criteria)

1. **AC-01 (Search Parameter Fail-Fast):**
   - Query dengan `adults < rooms` ditolak dengan HTTP 400 `INVALID_GUEST_COUNT`.
   - Query dengan `children == 0` tetapi menyertakan `child_ages` non-kosong ditolak dengan HTTP 400 `CHILD_AGE_COUNT_MISMATCH`.
   - Query dengan `children > 0` dan `child_ages` dengan jumlah tidak cocok ditolak dengan HTTP 400 `CHILD_AGE_COUNT_MISMATCH`.
   - Query dengan usia anak di luar [0, 17] ditolak dengan HTTP 400 `INVALID_CHILD_AGE`.

2. **AC-02 (Search Variant Capacity Evaluation):**
   - Varian yang kapasitasnya dilampaui (`adults > MaxAdults*rooms`, `children > MaxChildren*rooms`, atau `totalGuests > MaxCapacity*rooms`) ditandai `available: false` dengan `unavailable_reason: "EXCEEDS_CAPACITY"`.

3. **AC-03 (Quote Capacity Guard):**
   - Endpoint `POST /api/v1/quotes` memeriksa varian pada `CatalogStore`. Jika `num_guests > v.MaxCapacity * num_rooms`, request ditolak dengan HTTP 400 `EXCEEDS_CAPACITY`.
   - Jika `num_guests < num_rooms`, request ditolak dengan HTTP 400 `INVALID_GUEST_COUNT`.
   - Jika `room_type_id` tidak ditemukan di katalog, request ditolak dengan HTTP 404 `ROOM_NOT_FOUND`.

4. **AC-04 (Booking Creation Capacity Invariant):**
   - Endpoint `POST /api/v1/bookings` dan `booking.Service.Create` memeriksa varian pada `CatalogReader`.
   - Direct booking attempt (melewati search/quote) dengan overcapacity ditolak dengan HTTP 400 `EXCEEDS_CAPACITY`.
   - Direct booking attempt dengan `num_guests < num_rooms` ditolak dengan HTTP 400 `INVALID_GUEST_COUNT` / `INVALID_CAPACITY`.

5. **AC-05 (Zero Regresion & Coverage):**
   - Seluruh test suite `internal/api` dan `internal/booking` mencapai coverage $\ge 80\%$.
   - Full automated E2E script memvalidasi skenario overcapacity, bypass search, age mismatch, dan boundary valid.
