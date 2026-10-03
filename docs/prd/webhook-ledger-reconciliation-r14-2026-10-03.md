# PRD — Webhook Ledger & Amount Reconciliation (BE-R14)

**Fitur:** Webhook Amount, Currency & Ledger Verification (Anti-Underpayment & Out-of-Order Expiry)  
**Tanggal:** 2026-10-03  
**Status:** Approved for Implementation  
**Terkait:** BE-R14 (P1), BE-G10, BE-G11, F05, F14  

---

## 1. Latar Belakang & Masalah Bisnis

Pada pemrosesan callback webhook pembayaran pihak ketiga (Xendit) untuk hotel **Pulang ke Uttara (Yogyakarta)**:
1. **Penerimaan Nominal Buta (*Blind Confirmation*):** Sebelum perbaikan ini, handler `xenditWebhook` mengonfirmasi status booking ke `CONFIRMED` hanya berdasarkan string status `PAID`/`SETTLED` dan `ExternalID` (Booking ID), tanpa memverifikasi apakah `amount` yang dibayar di gateway sama persis dengan `total_price_minor` yang tercatat pada reservasi. Jika terjadi kesalahan kalkulasi, manipulasi nominal invoice, atau pembayaran parsial, booking senilai jutaan rupiah dapat dikonfirmasi hanya dengan pembayaran beberapa rupiah (*underpayment vulnerability*).
2. **Ketiadaan Validasi Mata Uang (*Currency Verification*):** Mata uang pada callback tidak dicocokkan dengan mata uang reservasi hotel (IDR).
3. **Pencocokan Invoice ID dengan Buku Besar Percobaan (*Payment Attempt Ledger*):** Invoice ID (`id`) dari payload callback belum diverifikasi terhadap `provider_reference` yang dicatat saat invoice dibuat di Xendit.
4. **Pembatalan Keliru Akibat Event Kedaluwarsa Usang (*Stale Expiry Event*):** Callback `EXPIRED` dari Xendit yang tiba terlambat atau di luar urutan (*out-of-order delivery*) dapat membatalkan booking yang sebenarnya sudah berstatus `CONFIRMED` dan lunas.

---

## 2. Persona & Pengguna Terkait

1. **Finance & Revenue Manager (`finance`, `rev_mgr`):**
   - Menuntut setiap dana yang masuk tercatat di buku besar (*ledger*) sesuai tagihan rupiah sebenarnya.
   - Terhindar dari selisih audit pembukuan akibat callback dengan nominal yang tidak valid.
2. **Tamu Publik (`guest`):**
   - Reservasi yang sudah dibayar sah tidak boleh dibatalkan secara keliru oleh event `EXPIRED` yang datang belakangan.
3. **Sistem Perhotelan (Core Engine):**
   - Menolak konfirmasi transaksi yang tidak cocok dengan buku besar dengan HTTP status code 422 Unprocessable Entity yang terstandarisasi.

---

## 3. Matriks Kebutuhan & Acceptance Criteria

| ID | Kategori | Deskripsi | Acceptance Criteria |
|---|---|---|---|
| AC-01 | Amount Matching | Verifikasi nominal pembayaran | Jika `payload.Amount != booking.TotalPriceMinor`, webhook menolak konfirmasi dengan HTTP 422 `PAYMENT_AMOUNT_MISMATCH` dan mencatat audit log. |
| AC-02 | Currency Matching | Verifikasi mata uang | Jika `payload.Currency` disediakan dan tidak sama dengan `booking.Currency` (case-insensitive), webhook menolak dengan HTTP 422 `PAYMENT_CURRENCY_MISMATCH`. |
| AC-03 | Invoice Ledger Matching | Pencocokan invoice ID terhadap attempt | Jika terdapat catatan `PaymentAttempt` dengan `ProviderReference`, `payload.ID` wajib cocok dengan invoice ID yang tercatat; ketidakcocokan ditolak dengan HTTP 422 `INVOICE_ID_MISMATCH`. |
| AC-04 | Out-of-Order Expiry Guard | Proteksi pembatalan pada booking lunas | Jika event `EXPIRED` diterima untuk booking yang sudah `CONFIRMED`, sistem mengabaikan event pembatalan secara aman dan merespons HTTP 200 dengan status `"ignored"` tanpa membatalkan kamar. |
| AC-05 | Idempotent Replay | Penerimaan duplikasi callback `PAID` | Callback `PAID` kedua kali pada booking yang sudah `CONFIRMED` mengembalikan HTTP 200 OK idempoten tanpa duplikasi outbox event notifikasi. |

---

## 4. Kebutuhan Non-Fungsional (NFR)

1. **Integritas Finansial:** Nol toleransi selisih minor unit (Rupiah) antara tagihan hotel dan penerimaan payment gateway.
2. **Ketahanan Konkurensi & Ordering:** Tahan terhadap *out-of-order network arrival* callback webhook gateway.
3. **Observabilitas:** Pencatatan error terstruktur saat terjadi *mismatch* dengan label `payment.webhook.amount_mismatch` dan `payment.webhook.invoice_mismatch`.
