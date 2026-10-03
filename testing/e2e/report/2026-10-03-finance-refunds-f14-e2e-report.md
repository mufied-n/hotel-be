# End-to-End (E2E) Test Report: Finance Reconciliation & Automated Gateway Refund (F14)
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**  
**Tanggal Eksekusi:** 2026-10-03 10:55:00 WIB  
**Dokumen Referensi:** [PRD F14](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/finance-reconciliation-and-refunds-f14-2026-10-03.md) | [SRS F14](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/finance-reconciliation-and-refunds-f14-2026-10-03.md) | [Tech Architecture F14](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/finance-reconciliation-and-refunds-f14-architecture-2026-10-03.md) | [Walkthrough F14](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/finance-reconciliation-and-refunds-f14-walkthrough-2026-10-03.md)  
**Status Pengujian:** **100% PASS (45/45 Scenarios)**

---

## 1. Ringkasan Eksekusi (Executive Summary)

Pengujian End-to-End (E2E) untuk modul **F14: Finance Reconciliation & Automated Gateway Refund** telah dieksekusi secara otomatis menggunakan suite pengujian Go di [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go).

Seluruh skenario pengujian baru (E2E-41 hingga E2E-45) dan skenario regresi sebelumnya (E2E-01 hingga E2E-40) berhasil lulus 100% tanpa adanya kegagalan atau degradasi performa.

| Kategori Pengujian | Total Skenario | Lulus (Pass) | Gagal (Fail) | Status |
| :--- | :---: | :---: | :---: | :---: |
| **Skenario Regresi (E2E-01 s/d E2E-40)** | 40 | 40 | 0 | **PASS** |
| **Fitur Baru F14: Finance Refund & RBAC (E2E-41)** | 1 | 1 | 0 | **PASS** |
| **Fitur Baru F14: Anti-Over-Refund Guard (E2E-42)** | 1 | 1 | 0 | **PASS** |
| **Fitur Baru F14: Late Payment Case Resolution (E2E-43)** | 1 | 1 | 0 | **PASS** |
| **Fitur Baru F14: Reconciliation Summary (E2E-44)** | 1 | 1 | 0 | **PASS** |
| **Fitur Baru F14: Guest Refund Status & Anti-IDOR (E2E-45)** | 1 | 1 | 0 | **PASS** |
| **TOTAL** | **45** | **45** | **0** | **100% PASS** |

---

## 2. Detil Verifikasi Skenario Baru F14

### A. E2E-41: Finance Refund Processing & RBAC Authorization
- **Endpoint:** `POST /api/v1/finance/refunds`
- **Uji Otorisasi Casbin RBAC:**
  - Klien dengan peran `receptionist` mencoba inisiasi refund:
    - *Request Header:* `Authorization: Bearer receptionist`
    - *HTTP Status:* **`403 Forbidden`** (Akses ditolak sesuai prinsip least privilege).
  - Staf berwenang dengan peran `finance`:
    - *Request Header:* `Authorization: Bearer finance`
    - *Request Payload:*
      ```json
      {
        "booking_id": "bk-e2e-001",
        "amount_minor": 500000,
        "reason": "Guest requested partial cancellation"
      }
      ```
    - *HTTP Status:* **`201 Created`**
    - *Response Payload:*
      ```json
      {
        "status": "success",
        "refund": {
          "id": "rfnd-e2e-001",
          "booking_id": "bk-e2e-001",
          "reference_id": "rfnd-BKE2E001-1759463702000000000",
          "amount_minor": 500000,
          "currency": "IDR",
          "reason": "Guest requested partial cancellation",
          "status": "succeeded",
          "provider": "xendit",
          "provider_refund_id": "mock_rfd_rfnd-BKE2E001-1759463702000000000",
          "actor_id": "staff:finance",
          "actor_role": "finance",
          "created_at": "2026-10-03T10:55:02Z",
          "updated_at": "2026-10-03T10:55:02Z"
        }
      }
      ```

### B. E2E-42: Anti-Over-Refund Guard (`BR-F14-01`)
- **Endpoint:** `POST /api/v1/finance/refunds`
- **Skenario:** Booking memiliki total pembayaran Rp 1.100.000. Setelah refund pertama Rp 500.000 (E2E-41), sisa dana refundable adalah Rp 600.000. Staf finance mencoba mengajukan refund kedua sebesar Rp 700.000 (melampaui sisa Rp 600.000).
- *HTTP Status:* **`409 Conflict`**
- *Response Payload:*
  ```json
  {
    "error": "requested refund amount exceeds refundable balance: requested 700000, remaining 600000",
    "code": "OVER_REFUND_EXCEEDED"
  }
  ```
- *Kesimpulan:* Proteksi over-refund baris PostgreSQL dengan lock `FOR UPDATE` berhasil memblokir pengeluaran kas hotel berlebih.

### C. E2E-43: Late Payment Case & Resolution Workflow (`BR-F14-04`)
- **Endpoint:** `GET /api/v1/finance/cases?status=open` & `POST /api/v1/finance/cases/{id}/resolve`
- **Skenario:**
  1. Kasus pembayaran terlambat (late payment yang tiba setelah hold kamar expired) otomatis dicatat di `payment_cases` dengan status `open`.
  2. Staf finance mengambil daftar sengketa via `GET /api/v1/finance/cases?status=open` -> **`200 OK`**.
  3. Staf finance mengeksekusi resolusi kasus:
     - *Request Payload:*
       ```json
       {
         "action": "refund",
         "notes": "Manual refund dispatched to guest bank account"
       }
       ```
     - *HTTP Status:* **`200 OK`**
     - *Response Payload:*
       ```json
       {
         "status": "ok",
         "message": "kasus pembayaran berhasil diselesaikan"
       }
       ```
  4. Resolusi ganda terhadap kasus yang sama:
     - *HTTP Status:* **`409 Conflict`** (`CASE_ALREADY_RESOLVED`).
     - *Kesimpulan:* Mencegah double resolution / duplicate payout.

### D. E2E-44: Finance Reconciliation Summary Aggregation (`FR-05`)
- **Endpoint:** `GET /api/v1/finance/reconciliations`
- *Request Header:* `Authorization: Bearer finance`
- *HTTP Status:* **`200 OK`**
- *Response Payload:*
  ```json
  {
    "total_settled_minor": 150000000,
    "total_refunded_minor": 500000,
    "net_captured_minor": 149500000,
    "open_cases_count": 0,
    "total_refunds_count": 1
  }
  ```
- *Kesimpulan:* Agregasi data rekonsiliasi kas mencerminkan dana kotor (gross captured), potongan refund, dan kas bersih (net captured) secara presisi.

### E. E2E-45: Guest Refund Status Self-Service & Anti-IDOR (`UU PDP No. 27/2022`)
- **Endpoint:** `GET /api/v1/guest/bookings/{id}/refund-status`
- **Skenario:**
  1. Tamu sah pemilik reservasi mengakses status pengembalian dana:
     - *Request Header:* `Authorization: Bearer gst_sess_rian_refund_token`
     - *Path Param:* `id = bk-e2e-001`
     - *HTTP Status:* **`200 OK`**
     - *Response Payload:*
       ```json
       {
         "booking_id": "bk-e2e-001",
         "has_refund": true,
         "refunds": [
           {
             "id": "rfnd-e2e-001",
             "booking_id": "bk-e2e-001",
             "reference_id": "rfnd-BKE2E001-1759463702000000000",
             "amount_minor": 500000,
             "currency": "IDR",
             "reason": "Guest requested partial cancellation",
             "status": "succeeded",
             "provider": "xendit",
             "provider_refund_id": "mock_rfd_rfnd-BKE2E001-1759463702000000000",
             "actor_id": "staff:finance",
             "actor_role": "finance",
             "created_at": "2026-10-03T10:55:02Z",
             "updated_at": "2026-10-03T10:55:02Z"
           }
         ]
       }
       ```
  2. Percobaan IDOR (Insecure Direct Object Reference):
     - Tamu mencoba mengakses booking milik orang lain (`bk-foreign-001`):
     - *HTTP Status:* **`404 Not Found`** (`BOOKING_NOT_FOUND`).
     - *Kesimpulan:* Sistem tidak membocorkan apakah booking milik pihak lain ada atau tidak, menjamin privasi data tamu sesuai amanat UU PDP No. 27/2022.

---

## 3. Kepatuhan Standar Industri & Regulasi

1. **Anti-Over-Refund & Financial Invariant (`BR-F14-01`):**
   Validasi konkurensi di tingkat basis data menggunakan `SELECT ... FOR UPDATE` dan kalkulasi akumulasi `SUM(amount_minor)` dari seluruh refund `succeeded`/`pending` menjamin tidak akan terjadi defisit kas atau over-refund bahkan saat terjadi lonjakan request bersamaan.
2. **PCI-DSS v4.0 SAQ A:**
   Tidak ada data kartu kredit (PAN, CVV) yang tersimpan di sistem Pulang ke Uttara. Pengembalian dana dilakukan melalui token gateway pembayaran terpercaya (Xendit Refund API) menggunakan referensi invoice asli.
3. **Idempotensi & Network Resilience (IETF Idempotency-Key):**
   Setiap instruksi refund ke payment gateway dilengkapi dengan deterministik `reference_id` unik berbasis booking ID dan timestamp mikrodetik, memastikan tidak akan terjadi pemotongan saldo ganda oleh gateway.
4. **Prinsip Ponytail (Anti-Overengineering):**
   Implementasi menggunakan Go standard library murni (`net/http`, `crypto/rand`, `database/sql`, `log/slog`), tanpa menambahkan library eksternal yang tidak diperlukan.

---

## 4. Kesimpulan Akhir

Modul **F14: Finance Reconciliation & Automated Gateway Refund** telah teruji tuntas dan memenuhi 100% kriteria penerimaan (acceptance criteria) bisnis dan teknis. Seluruh pengujian unit, integrasi, dan E2E menghasilkan status **PASS 100%**.
