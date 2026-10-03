# Software Requirements Specification (SRS) — Booking Ownership & Allowed Actions Consistency (BE-R05)

**Feature:** Canonical Email Normalization & Policy-Driven Allowed Actions  
**Gap ID:** BE-R05 (P1)  
**Date:** 2026-10-03  
**Status:** Approved for Implementation  
**Target Module:** `internal/booking`, `internal/guest`  

---

## 1. Functional Requirements

### FR-01: Canonical Email Normalization on Booking Creation
- Pada `booking.Service.CreateBooking` dan `booking.Service.CreateBookingQuoted`:
  ```go
  normEmail := strings.ToLower(strings.TrimSpace(in.GuestEmail))
  ```
  Nilai `normEmail` harus digunakan untuk validasi format dan disimpan ke atribut `b.GuestEmail`.

### FR-02: Case-Insensitive Matching on Guest Store Lookups
- Pada query PostgreSQL di `internal/guest/postgres.go`:
  - `ListBookingsByEmail`:
    ```sql
    WHERE LOWER(TRIM(b.guest_email)) = $1
    ```
  - `GetBookingDetailByEmail`:
    ```sql
    WHERE b.id = $1 AND LOWER(TRIM(b.guest_email)) = $2
    ```
  - `CountActiveBookingsByEmail`:
    ```sql
    WHERE LOWER(TRIM(guest_email)) = $1 AND status IN ('pending', 'confirmed')
    ```
  - `GetBookingReceiptData`:
    ```sql
    WHERE b.id = $1 AND LOWER(TRIM(b.guest_email)) = $2
    ```
- Argumen email yang diteruskan dari handler/service selalu di-normalisasi dengan `strings.ToLower(strings.TrimSpace(email))`.

### FR-03: Query Expansion for Booking Detail
- `GetBookingDetailByEmail` diperluas untuk membaca:
  - `COALESCE(b.cancellation_policy, '')`
  - `COALESCE(b.rate_plan_code, '')`
  - `b.expires_at`
- Model `BookingDetail` di `internal/guest/model.go` memuat:
  - `CancellationPolicy string`
  - `RatePlanCode string`
  - `ExpiresAt *time.Time`

### FR-04: Dynamic Allowed Actions Computation
- Fungsi `computeAllowedActions(d *BookingDetail, now time.Time) AllowedActions`:
  - **`pending`**:
    - Jika `d.ExpiresAt != nil && now.After(*d.ExpiresAt)`:
      - `CanPay = false`, `CanCancel = false`, `CanRequestAssistance = true`
    - Jika masa hold masih aktif:
      - `CanPay = true`, `CanCancel = true`, `CanRequestAssistance = true`
  - **`confirmed`**:
    - `CanDownloadReceipt = true`, `CanRequestAssistance = true`
    - Evaluasi `CanCancel`:
      - Jika `d.CancellationPolicy == rates.PolicyNonRefundable` ("non_refundable"): `CanCancel = false`
      - Jika kebijakan memiliki batas waktu (misal `flexible_48h`):
        - Hitung `deadline, ok := booking.FreeCancellationDeadline(checkInDate, d.CancellationPolicy)`
        - Jika `ok && now.After(deadline)`: `CanCancel = false`
        - Jika `ok && !now.After(deadline)`: `CanCancel = true`
  - **`checked_in`**:
    - `CanDownloadReceipt = true`, `CanRequestAssistance = true`
  - **`checked_out`**:
    - `CanDownloadReceipt = true`
  - **Status Lain (`cancelled`, `expired`, `failed`, `no_show`)**:
    - Seluruh flag bernilai `false`.

---

## 2. API Contract Specification

### GET `/api/v1/guest/bookings/:id`
- **Response 200 OK (Confirmed Non-Refundable)**:
  ```json
  {
    "booking": {
      "id": "01900000-0000-7000-8000-000000000001",
      "status": "confirmed",
      "cancellation_policy": "non_refundable",
      "allowed_actions": {
        "can_pay": false,
        "can_cancel": false,
        "can_download_receipt": true,
        "can_request_assistance": true
      }
    },
    "allowed_actions": {
      "can_pay": false,
      "can_cancel": false,
      "can_download_receipt": true,
      "can_request_assistance": true
    }
  }
  ```

- **Response 200 OK (Confirmed Flexible-48h Setelah Batas Waktu)**:
  ```json
  {
    "booking": {
      "id": "01900000-0000-7000-8000-000000000002",
      "status": "confirmed",
      "check_in": "2026-10-04",
      "cancellation_policy": "flexible_48h",
      "allowed_actions": {
        "can_pay": false,
        "can_cancel": false,
        "can_download_receipt": true,
        "can_request_assistance": true
      }
    },
    "allowed_actions": {
      "can_pay": false,
      "can_cancel": false,
      "can_download_receipt": true,
      "can_request_assistance": true
    }
  }
  ```

- **Response 200 OK (Pending Hold Kedaluwarsa)**:
  ```json
  {
    "booking": {
      "id": "01900000-0000-7000-8000-000000000003",
      "status": "pending",
      "allowed_actions": {
        "can_pay": false,
        "can_cancel": false,
        "can_download_receipt": false,
        "can_request_assistance": true
      }
    }
  }
  ```
