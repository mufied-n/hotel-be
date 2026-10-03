# Technical Architecture & Design — Cancellation Deadline Timezone Alignment (BE-R12)

**Fitur:** Cancellation Deadline WIB Timezone Alignment  
**ID Gap:** BE-R12 (P1)  
**Terkait:** BE-G08, BE-G19, F06/F13  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  

---

## 1. Perbandingan Garis Waktu: Bug UTC vs Koreksi WIB

```
Check-In Date: 10 Oktober 2026

Kebijakan Resmi: 48 jam sebelum 14:00 WIB (Asia/Jakarta, UTC+7)
------------------------------------------------------------------------------------------------------
Waktu Nyata:           8 Okt 07:00 UTC                 8 Okt 14:00 UTC                 10 Okt 07:00 UTC
                      (8 Okt 14:00 WIB)               (8 Okt 21:00 WIB)               (10 Okt 14:00 WIB)
                              |                               |                               |
                              |                               |                               |
[KOREKSI BE-R12] ------------>| BATAS DEADLINE RESMI          |                               | Check-in Resmi
                              | (Pembatalan setelah           |                               |
                              |  jam ini DITOLAK 409)         |                               |
                              |                               |                               |
[IMPLEMENTASI LAMA] --------->|                               | BATAS DEADLINE BUG            |
                              |<------- GAP 7 JAM ----------->| (Pembatalan salah diterima    |
                              |    BOCOR REVENUE HOTEL        |  karena menganggap 14:00 UTC) |
------------------------------------------------------------------------------------------------------
```

---

## 2. Diagram Alur Evaluasi Pembatalan

```mermaid
flowchart TD
    Req["POST /api/v1/bookings/:id/cancel"] --> Load["s.tx.GetForUpdate(ctx, id)"]
    Load --> CheckStatus{"Status Booking?"}

    CheckStatus -- "pending" --> CancelHold["Release Hold & Update Status Cancelled\n(Selalu diperbolehkan)"]
    CheckStatus -- "cancelled" --> Idempotent["Return 200 OK (Idempotent)"]
    CheckStatus -- "confirmed" --> CheckPolicy{"Kebijakan Tarif?"}

    CheckPolicy -- "non_refundable" --> ErrNonRef["Return ErrNonRefundable (HTTP 409)"]
    CheckPolicy -- "flexible_48h" --> CalcDeadline["FreeCancellationDeadline(b.CheckIn, policy)\nDeadline = CheckIn @ 14:00 WIB - 48 jam"]

    CalcDeadline --> CheckTime{"s.now().After(deadline)?"}
    CheckTime -- "YA (> 14:00 WIB H-2)" --> ErrDeadline["Return ErrCancellationDeadlineExceeded (HTTP 409)"]
    CheckTime -- "TIDAK (<= 14:00 WIB H-2)" --> ExecCancel["Restitusi Inventaris Kamar\nUpdate Status Cancelled\nPublish Outbox Event"]
    ExecCancel --> RespOK["Return HTTP 200 OK"]
```

---

## 3. Detail Implementasi & Anti-Overengineering (Ponytail)

### 3.1 Konstruksi Zona Waktu Go Tanpa Ketergantungan Eksternal
Menggunakan `time.FixedZone("WIB", 7*3600)` menghindari kegagalan `time.LoadLocation("Asia/Jakarta")` pada lingkungan container minimal yang tidak menginstal paket `tzdata` (seperti Alpine scratch):
```go
var LocationWIB = time.FixedZone("WIB", 7*3600)
```

### 3.2 Injeksi Clock untuk Pengujian Deterministik
```go
type Service struct {
    ...
    nowFunc func() time.Time
}

func (s *Service) now() time.Time {
    if s.nowFunc != nil {
        return s.nowFunc()
    }
    return time.Now()
}

func (s *Service) SetNowFunc(fn func() time.Time) {
    s.nowFunc = fn
}
```
Metode ini memungkinkan pengujian unit mensimulasikan detik-detik krusial di sekitar batas waktu 14:00 WIB tanpa manipulasi waktu sistem host.
