# SRS — F14: Rekonsiliasi Finansial & Otomasi Gateway Refund (Xendit Refund API)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

Dokumen Terkait:
- **PRD Rujukan:** [PRD-F14-Finance](../prd/finance-reconciliation-and-refunds-f14-2026-10-03.md)
- **Tech Architecture:** [TECH-F14-Finance](../tech/finance-reconciliation-and-refunds-f14-architecture-2026-10-03.md)
- **Walkthrough Tracking:** [Walkthrough F14](../walkthrough/finance-reconciliation-and-refunds-f14-walkthrough-2026-10-03.md)

---

## 1. Functional Requirements (FR)

- **FR-01 (Process Refund):** Sistem wajib menyediakan kapabilitas eksekusi pengembalian dana (penuh atau sebagian) ke payment gateway Xendit, memvalidasi sisa saldo refundable, memotong secara atomik, dan mencatat mutasi di `payment_refunds`.
- **FR-02 (Payment Cases & Late Payment Ingestion):** Sistem wajib secara otomatis mencatat *payment case* ketika terjadi anomali pembayaran (pembayaran tiba setelah hold kedaluwarsa, selisih nominal tagihan, atau duplikasi pembayaran).
- **FR-03 (Resolve Payment Case):** Sistem wajib menyediakan endpoint bagi staf finance untuk mereview kasus pembayaran dan mengeksekusi resolusi (apakah di-refund langsung atau penyesuaian operasional kamar).
- **FR-04 (Role-Based Authorization & Fail-Closed):** Endpoint finansial mutasi wajib diproteksi oleh Casbin RBAC, hanya dapat diakses oleh role `finance` dan `gm_admin`. Role lain ditolak HTTP 403 Forbidden.
- **FR-05 (Guest Refund Status & Anti-IDOR):** Tamu dapat melihat riwayat dan status proses refund reservasinya sendiri melalui endpoint privat terproteksi sesi tamu. Jika booking ID bukan milik email tamu, kembalikan HTTP 404 Not Found.
- **FR-06 (Gateway Idempotency & Serialized Lock):** Eksekusi refund wajib menyertakan `reference_id` deterministik dan dilindungi oleh database lock `FOR UPDATE` untuk mencegah *race condition* double-refund.

---

## 2. Spesifikasi Endpoint HTTP RESTful

### 2.1 POST /api/v1/finance/refunds

* **Otorisasi:** RBAC Casbin (Role: `finance`, `gm_admin`).
* **Request JSON:**
```json
{
  "booking_id": "01924b12-3456-789a-bcde-f0123456789a",
  "amount_minor": 158950000,
  "reason": "Pembatalan tamu sesuai kebijakan fleksibel H-1",
  "notes": "Disetujui oleh Finance Lead"
}
```
* **Response Sukses (HTTP 201 Created):**
```json
{
  "refund_id": "01924c88-9999-789a-bcde-f0123456789a",
  "booking_id": "01924b12-3456-789a-bcde-f0123456789a",
  "reference_id": "rfnd-01924b12-001",
  "amount_minor": 158950000,
  "currency": "IDR",
  "status": "succeeded",
  "provider": "xendit",
  "provider_refund_id": "rfd_xen_123456789",
  "created_at": "2026-10-03T10:45:00Z"
}
```

### 2.2 GET /api/v1/finance/cases

* **Otorisasi:** RBAC Casbin (Role: `finance`, `gm_admin`).
* **Response Sukses (HTTP 200 OK):**
```json
{
  "data": [
    {
      "id": "case-001",
      "booking_id": "01924b12-3456-789a-bcde-f0123456789a",
      "case_type": "late_payment",
      "status": "open",
      "amount_minor": 158950000,
      "currency": "IDR",
      "provider_reference": "inv_xen_late_01",
      "notes": "Pembayaran diterima setelah hold kedaluwarsa",
      "created_at": "2026-10-03T10:40:00Z"
    }
  ],
  "total": 1
}
```

### 2.3 POST /api/v1/finance/cases/{id}/resolve

* **Otorisasi:** RBAC Casbin (Role: `finance`, `gm_admin`).
* **Request JSON:**
```json
{
  "action": "refund",
  "notes": "Kamar sudah penuh, dana dikembalikan penuh ke tamu"
}
```
* **Response Sukses (HTTP 200 OK):**
```json
{
  "case_id": "case-001",
  "status": "resolved",
  "resolution_action": "refund",
  "resolved_at": "2026-10-03T10:46:00Z"
}
```

### 2.4 GET /api/v1/guest/bookings/{id}/refund-status

* **Otorisasi:** Wajib sesi tamu (`X-Guest-Session`).
* **Response Sukses (HTTP 200 OK):**
```json
{
  "booking_id": "01924b12-3456-789a-bcde-f0123456789a",
  "has_refund": true,
  "refunds": [
    {
      "id": "01924c88-9999-789a-bcde-f0123456789a",
      "amount_minor": 158950000,
      "currency": "IDR",
      "status": "succeeded",
      "reason": "Pembatalan tamu sesuai kebijakan fleksibel H-1",
      "created_at": "2026-10-03T10:45:00Z"
    }
  ]
}
```

---

## 3. Penanganan Error & Failure Semantics

| Kondisi | HTTP Status | Kode Error | Keterangan |
|---|---|---|---|
| Role staf tidak berwenang | 403 Forbidden | `FORBIDDEN` | Pengguna tidak memiliki permission `finance` atau `gm_admin` |
| Nominal refund melebihi tagihan | 400 Bad Request | `INVALID_REFUND_AMOUNT` | Nominal refund melebihi sisa dana yang dapat dikembalikan |
| Booking belum berstatus lunas | 400 Bad Request | `BOOKING_NOT_PAID` | Reservasi belum dibayar sehingga tidak dapat di-refund |
| Alasan refund kosong | 400 Bad Request | `VALIDATION_ERROR` | Alasan refund wajib diisi minimal 5 karakter |
| Booking tidak ditemukan / IDOR | 404 Not Found | `BOOKING_NOT_FOUND` | Data reservasi tidak ditemukan |
| Gateway timeout / failure | 502 Bad Gateway | `GATEWAY_REFUND_FAILED` | Gateway Xendit mengembalikan kegagalan saat proses refund |
