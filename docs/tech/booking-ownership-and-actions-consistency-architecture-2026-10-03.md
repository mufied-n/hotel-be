# Technical Architecture — Booking Ownership & Allowed Actions Consistency (BE-R05)

**Feature:** Canonical Email Normalization & Policy-Driven Allowed Actions  
**Gap ID:** BE-R05 (P1)  
**Date:** 2026-10-03  
**Status:** Approved for Implementation  
**Target Module:** `internal/booking`, `internal/guest`  

---

## 1. Flowchart: Canonical Email Normalization Pipeline

```mermaid
flowchart TD
    A["Guest Input Email<br/>(e.g. ' Tamu.Uttara@Example.COM ')"] --> B["Canonical Sanitization<br/>strings.ToLower(strings.TrimSpace(email))"]
    B --> C["Stored in bookings table<br/>('tamu.uttara@example.com')"]
    
    D["Guest Requests OTP Challenge<br/>(e.g. 'tamu.uttara@example.com')"] --> E["guest_auth_challenges<br/>email lowercase"]
    
    E --> F["Guest Verified Session<br/>GuestEmail in guest_sessions"]
    F --> G["PostgreSQL Lookup Query<br/>WHERE LOWER(TRIM(b.guest_email)) = $1"]
    C --> G
    G --> H["Deterministic Match (100% Found)"]
```

---

## 2. Decision Tree: `allowed_actions` Derivation

```mermaid
flowchart TD
    Start["Evaluate Booking Detail"] --> Status{"Status?"}
    
    Status -->|"pending"| PendingHold{"ExpiresAt != nil &&<br/>now > ExpiresAt?"}
    PendingHold -->|"Yes (Expired)"| ActPendingExp["CanPay: false<br/>CanCancel: false<br/>CanRequestAssistance: true"]
    PendingHold -->|"No (Active Hold)"| ActPendingActive["CanPay: true<br/>CanCancel: true<br/>CanRequestAssistance: true"]
    
    Status -->|"confirmed"| Policy{"Cancellation Policy?"}
    Policy -->|"non_refundable"| ActNonRef["CanCancel: false<br/>CanDownloadReceipt: true<br/>CanRequestAssistance: true"]
    Policy -->|"flexible_48h"| Cutoff{"now > FreeCancellationDeadline<br/>(H-2 14:00 WIB)?"}
    Cutoff -->|"Yes (Cutoff Exceeded)"| ActLate["CanCancel: false<br/>CanDownloadReceipt: true<br/>CanRequestAssistance: true"]
    Cutoff -->|"No (Before Cutoff)"| ActEarly["CanCancel: true<br/>CanDownloadReceipt: true<br/>CanRequestAssistance: true"]
    
    Status -->|"checked_in"| ActIn["CanDownloadReceipt: true<br/>CanRequestAssistance: true"]
    Status -->|"checked_out"| ActOut["CanDownloadReceipt: true"]
    Status -->|"cancelled / other"| ActNone["All False"]
```

---

## 3. Anti-Overengineering Analysis (Ponytail)

- **Reuse Existing Policy Machinery**: Memanfaatkan fungsi `booking.FreeCancellationDeadline` yang sudah terbukti andal pada perbaikan BE-R12 tanpa menduplikasi logika batas waktu 14:00 WIB.
- **Zero Migration Needed**: Normalisasi query `LOWER(TRIM(b.guest_email)) = $1` menyelesaikan inkonsistensi data historis maupun masa depan secara instan tanpa perlu skrip migrasi berisiko tinggi.
