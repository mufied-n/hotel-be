# Software Requirements Specification (SRS) — Cancellation Deadline Timezone Alignment (BE-R12)

**Fitur:** Cancellation Deadline WIB Timezone Alignment  
**ID Gap:** BE-R12 (P1)  
**Terkait:** BE-G08, BE-G19, F06/F13  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  

---

## 1. Functional Requirements

### FR-01: Perhitungan Batas Waktu Pembatalan Berbasis WIB
Sistem wajib menyediakan fungsi murni `FreeCancellationDeadline(checkIn time.Time, policy string) (time.Time, bool)` di paket `internal/booking`:
1. Jika `policy != rates.PolicyFlexible48h`, fungsi mengembalikan `(time.Time{}, false)`.
2. Jika `policy == rates.PolicyFlexible48h`:
   - Ambil komponen tanggal `y, m, d = checkIn.Date()`.
   - Bentuk waktu cutoff resmi: `time.Date(y, m, d, 14, 0, 0, 0, LocationWIB)` di mana `LocationWIB` adalah `time.FixedZone("WIB", 7*3600)`.
   - Batas waktu pembatalan gratis adalah `cutoff.Add(-48 * time.Hour)`.
   - Mengembalikan `(deadline, true)`.

### FR-02: Penegakan Batas Waktu pada Metode Cancel
Pada saat metode `booking.Service.Cancel(ctx, bookingID)` dipanggil untuk booking berstatus `confirmed`:
1. Jika `b.CancellationPolicy == rates.PolicyNonRefundable`, kembalikan `booking.ErrNonRefundable`.
2. Jika `deadline, ok := FreeCancellationDeadline(b.CheckIn, b.CancellationPolicy); ok`:
   - Bandingkan waktu saat ini `s.now()` dengan `deadline`.
   - Jika `s.now().After(deadline)`, kembalikan `booking.ErrCancellationDeadlineExceeded`.
3. Jika pembatalan diizinkan, transisikan status booking ke `cancelled`, kembalikan ketersediaan kamar, dan publikasikan event outbox `booking.cancelled`.

### FR-03: Provider Waktu Clock-Controlled
Struktur `booking.Service` wajib menyediakan:
```go
func (s *Service) SetNowFunc(fn func() time.Time)
```
Default implementasi menggunakan `time.Now`.

---

## 2. Kontrak HTTP & Respon

### Endpoint: `POST /api/v1/bookings/{id}/cancel`

#### Skenario A: Pembatalan Diterima (Sebelum atau Tepat Deadline)
- **Waktu Request:** $\le D-2\text{ hari } 14:00:00\text{ WIB}$ (atau $\le 07:00:00\text{ UTC}$)
- **Status:** `200 OK`
- **Body:**
```json
{
  "status": "cancelled",
  "id": "01923456-789a-7b00-8c00-000000000001"
}
```

#### Skenario B: Pembatalan Melewati Batas Waktu (1 Detik Setelah Deadline s/d Kapan Saja)
- **Waktu Request:** $> D-2\text{ hari } 14:00:00\text{ WIB}$ (misal: 14:00:01 WIB / 07:00:01 UTC)
- **Status:** `409 Conflict`
- **Body:**
```json
{
  "code": "CANCELLATION_DEADLINE_EXCEEDED",
  "message": "batas waktu pembatalan gratis 48 jam sebelum check-in telah terlewati"
}
```

#### Skenario C: Pembatalan Tarif Non-Refundable
- **Status:** `409 Conflict`
- **Body:**
```json
{
  "code": "NON_REFUNDABLE_BOOKING",
  "message": "reservasi non-refundable tidak dapat dibatalkan oleh tamu"
}
```
