# SRS: Spesifikasi Biaya Sarapan & Multi-Room Guest Tiers (BE-R10)

**Nomor Dokumen:** SRS-PULANG-BE-R10  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R10, BE-G04, BE-G05, F01, F09  

---

## 1. Pendahuluan

Dokumen ini mendefinisikan spesifikasi kebutuhan perangkat lunak untuk perhitungan biaya paket sarapan (*Bed & Breakfast*) pada hotel Pulang ke Uttara (Yogyakarta). Dokumen ini mengakhiri ambiguitas parameter jumlah tamu (`num_guests`) pada reservasi multi-kamar dan memberlakukan tarif berjenjang (*guest tiers*) sesuai usia anak.

---

## 2. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Semantik Baku Parameter `num_guests`
- Nilai `num_guests` pada seluruh antarmuka HTTP dan internal domain didefinisikan secara tegas sebagai **Total Seluruh Tamu dalam Reservasi (Aggregate Reservation Guests)**, bukan kapasitas per kamar.
- Jika request menyertakan `adults` dan `children`, nilai $num\_guests = adults + children$.

### FR-02: Perhitungan Biaya Sarapan Tanpa Pengali Kamar
- Biaya sarapan paket `bed_and_breakfast` dihitung berdasarkan total tamu yang berhak per malam menginap ($nights$).
- Sistem **DILARANG** mengalikan kembali biaya sarapan dengan `num_rooms`.
- Rumus dasar:
  $$\text{BreakfastCharge} = \text{NightlyBreakfastTotal} \times nights$$

### FR-03: Tarif Berjenjang Usia Tamu (Guest Age Tiers)
Konstanta tarif resmi Pulang ke Uttara:
```go
const (
    BreakfastRatePerPersonPerNight = 100_000 // Dewasa (>=12 tahun)
    BreakfastRateChildPerNight     = 50_000  // Anak (6-11 tahun)
)
```
Aturan evaluasi per orang per malam:
1. Setiap orang dewasa ($adults$): Rp 100.000 / malam.
2. Setiap anak dalam $child\_ages$:
   - $0 \le age \le 5$: **Rp 0** (Gratis / Complimentary).
   - $6 \le age \le 11$: **Rp 50.000** (50% dari tarif dewasa).
   - $12 \le age \le 17$: **Rp 100.000** (100% tarif dewasa).
3. Jika $children > 0$ namun $child\_ages$ kosong (fallback):
   - Setiap anak diasumsikan kategori 6–11 tahun (Rp 50.000 / malam).
4. Jika $adults == 0$ dan $children == 0$ (panggilan legacy):
   - Seluruh $num\_guests$ diasumsikan dewasa (Rp 100.000 / malam).

### FR-04: Perluasan Port Domain `rates.QuoteRequest` & `rates.LockedQuote`
- Modul `rates` memperluas struct input dan output:
  ```go
  type QuoteRequest struct {
      RoomTypeID   string    `json:"room_type_id"`
      RatePlanCode string    `json:"rate_plan_code"`
      CheckIn      time.Time `json:"check_in"`
      CheckOut     time.Time `json:"check_out"`
      NumRooms     int       `json:"num_rooms"`
      NumGuests    int       `json:"num_guests"`
      Adults       int       `json:"adults,omitempty"`
      Children     int       `json:"children,omitempty"`
      ChildAges    []int     `json:"child_ages,omitempty"`
      PromoCode    string    `json:"promo_code"`
  }

  type LockedQuote struct {
      // Field sebelumnya tetap ada...
      Adults    int   `json:"adults,omitempty"`
      Children  int   `json:"children,omitempty"`
      ChildAges []int `json:"child_ages,omitempty"`
  }
  ```

---

## 3. Spesifikasi Kontrak HTTP JSON (`POST /api/v1/quotes`)

### 3.1 Contoh Request: 2 Kamar, 4 Dewasa (Bed & Breakfast, 1 Malam)
```json
{
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "rate_plan_code": "bed_and_breakfast",
  "check_in": "2026-10-14",
  "check_out": "2026-10-15",
  "num_rooms": 2,
  "adults": 4,
  "children": 0
}
```
* Respons JSON:
```json
{
  "quote_id": "01a10254-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "num_rooms": 2,
  "num_guests": 4,
  "adults": 4,
  "children": 0,
  "pricing": {
    "room_subtotal_minor": 1100000,
    "breakfast_charge_minor": 400000,
    "discount_minor": 0,
    "tax_minor": 150000,
    "total_price_minor": 1650000,
    "currency": "IDR"
  }
}
```
*(Perhatikan `breakfast_charge_minor` adalah 400.000, BUKAN 800.000!)*

### 3.2 Contoh Request: 1 Kamar, Keluarga (2 Dewasa, 1 Balita Usia 4, 1 Anak Usia 8)
```json
{
  "room_type_id": "01900000-0000-7000-8000-000000000006",
  "rate_plan_code": "bed_and_breakfast",
  "check_in": "2026-10-14",
  "check_out": "2026-10-16",
  "num_rooms": 1,
  "adults": 2,
  "children": 2,
  "child_ages": [4, 8]
}
```
* Rincian Sarapan per Malam:
  * 2 Dewasa: $2 \times 100.000 = 200.000$
  * 1 Balita (4 thn): $0$
  * 1 Anak (8 thn): $50.000$
  * Total per malam = $250.000$
  * 2 Malam = $500.000$
* Respons JSON:
```json
{
  "pricing": {
    "breakfast_charge_minor": 500000
  }
}
```
