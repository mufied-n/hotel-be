# PRD: Kebijakan Biaya Sarapan & Multi-Room Guest Tiers (BE-R10)

**Nomor Dokumen:** PRD-PULANG-BE-R10  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R10, BE-G04, BE-G05, F01, F09  

---

## 1. Konteks Bisnis & Latar Belakang

Pada Hotel Bintang 4 "Pulang ke Uttara" (Yogyakarta, 95 kamar), tamu dapat memilih paket menginap *Room Only* (RO) atau *Bed & Breakfast* (BB). Restoran hotel menyediakan sarapan buffet harian khas Nusantara dan kontinental dengan biaya resmi:
- Dewasa: Rp 100.000 / orang / malam.
- Anak usia 6–11 tahun: Rp 50.000 / anak / malam (diskon 50%).
- Anak balita usia 0–5 tahun: Gratis (Rp 0 / complimentary) saat menginap bersama orang tua.
- Anak usia 12–17 tahun: Mengikuti tarif dewasa (Rp 100.000 / malam).

### Permasalahan (Gap BE-R10)
1. **Double Counting Multi-Room:**  
   Pada `internal/rates/engine.go`, rumus biaya sarapan sebelumnya adalah:
   $$\text{breakfastCharge} = \text{BreakfastRatePerPersonPerNight} \times \text{numGuests} \times \text{numNights} \times \text{numRooms}$$
   Karena `num_guests` dalam kontrak API adalah **total seluruh tamu dalam reservasi**, pengalian kembali dengan `numRooms` menyebabkan *double counting*. Contoh: Pemesanan 2 kamar untuk 4 tamu selama 1 malam dikenakan $100.000 \times 4 \times 1 \times 2 = \text{Rp } 800.000$ (seharusnya Rp 400.000 untuk 4 orang!).
2. **Tidak Ada Pemisahan Dewasa dan Anak (Child Tiers):**  
   `QuoteRequest` pada modul `rates` tidak menerima rincian `Adults`, `Children`, dan `ChildAges`, sehingga seluruh anak dikenakan biaya penuh dewasa (Rp 100.000) dan balita tidak memperoleh hak sarapan gratis.
3. **Ambiguitas DTO vs Snapshot Reservasi:**  
   Tidak ada kontrak formal yang menegaskan bahwa `num_guests` adalah agregat reservasi, dan breakdown sarapan belum mencatat komposisi usia tamu.

---

## 2. Persona & Pengguna Terdampak

1. **Guest (Tamu Publik / Keluarga):**
   - Mendapatkan tagihan sarapan yang adil, akurat, dan sesuai dengan jumlah orang sebenarnya tanpa *overcharge* saat memesan lebih dari satu kamar.
   - Memperoleh keuntungan tarif sarapan anak (50% untuk usia 6–11 tahun dan gratis untuk usia < 6 tahun).

2. **Revenue Manager:**
   - Memiliki kendali tarif sarapan baku yang konsisten dan patuh terhadap kebijakan hotel bintang 4.

3. **Front Desk / Receptionist & Restoran:**
   - Menghindari komplain tamu saat check-in atau saat masuk ke restoran perihal selisih kupon/voucher sarapan dengan tagihan invoice.

---

## 3. Matriks Kebijakan & Aturan Perhitungan Sarapan

| Kategori Tamu | Batas Usia | Tarif Sarapan Pulang ke Uttara |
| :--- | :--- | :--- |
| **Dewasa (Adult)** | $\ge 18$ tahun | Rp 100.000 / orang / malam |
| **Remaja (Teenager)** | 12–17 tahun | Rp 100.000 / orang / malam |
| **Anak (Child)** | 6–11 tahun | Rp 50.000 / orang / malam (50% diskon) |
| **Balita (Infant/Toddler)** | 0–5 tahun | Rp 0 (Complimentary) |

### Formula Perhitungan Sarapan Baku (BE-R10)
Jika paket tarif adalah `bed_and_breakfast`:
1. Jika payload menyertakan rincian `adults`, `children`, dan `child_ages`:
   $$\text{PerNightAdult} = adults \times 100.000$$
   $$\text{PerNightChild} = \sum_{age \in child\_ages} \text{TierRate}(age)$$
   $$\text{TotalBreakfastPerNight} = \text{PerNightAdult} + \text{PerNightChild}$$
   $$\text{TotalBreakfast} = \text{TotalBreakfastPerNight} \times num\_nights$$
2. Jika payload hanya menyertakan `num_guests` (legacy / fallback):
   $$\text{TotalBreakfast} = 100.000 \times num\_guests \times num\_nights$$
   *(Tidak lagi dikalikan dengan `num_rooms`)*.

---

## 4. Kriteria Penerimaan (Acceptance Criteria)

1. **AC-01 (Single Counting on Multi-Room):**
   - Pemesanan multi-kamar (misal: 2 kamar, 4 dewasa, 1 malam) menghasilkan biaya sarapan tepat Rp 400.000, bukan Rp 800.000.
2. **AC-02 (Complimentary Toddler Breakfast):**
   - Anak usia 0–5 tahun tidak dikenakan biaya sarapan (Rp 0).
3. **AC-03 (Child Tier 50% Rate):**
   - Anak usia 6–11 tahun dikenakan biaya sarapan Rp 50.000 / anak / malam.
4. **AC-04 (Teenager Full Rate):**
   - Anak usia 12–17 tahun dikenakan biaya sarapan Rp 100.000 / anak / malam.
5. **AC-05 (Tax & Discount Integrity):**
   - Diskon promo (misal `OCTOBREAK` 15%) tetap dihitung dari subtotal kamar saja.
   - Pajak PB1 10% dihitung dari total nilai kena pajak: $(\text{Subtotal Kamar} + \text{Biaya Sarapan} - \text{Diskon})$.
6. **AC-06 (Snapshot Integrity):**
   - `LockedQuote` menyimpan data `adults`, `children`, `child_ages`, dan `breakfast_charge_minor` secara transparan.
