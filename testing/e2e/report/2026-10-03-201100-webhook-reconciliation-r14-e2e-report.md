# E2E Report — Webhook Ledger & Amount Reconciliation (BE-R14)

**Tanggal:** 2026-10-03 20:11 WIB  
**Lingkungan Uji:** Stack terisolasi Postgres 18 (`booking_test` :25432) & Valkey (:26379), migrasi 1–17.  
**Fitur Teruji:** BE-R14 (P1) — Webhook Amount, Currency & Ledger Verification.  

---

## 1. Ringkasan Eksekusi

| Suite / Skrip | Status | Hasil |
|---|---|---|
| `webhook_reconciliation_r14_e2e.sh` | **PASS** | 18 Total, 18 Passed, 0 Failed |
| `rtk go vet ./...` | **PASS** | Zero issues / warnings |
| `rtk go test -race ./...` | **PASS** | 761 passed in 23 packages |

---

## 2. Cakupan Pengujian Unit & Integrasi (Coverage $\ge$ 80%)

| Package | Cakupan Statement | Status |
|---|---|---|
| `internal/api` | **85.0%** | Memenuhi syarat ($\ge$ 80%) |
| `internal/adapter/payment` | **87.5%** | Memenuhi syarat ($\ge$ 80%) |

---

## 3. Matriks Verifikasi Kebutuhan (Sebelum vs Sesudah)

| Skenario | Sebelum (Vulnerable) | Sesudah (Hardened / BE-R14) |
|---|---|---|
| Callback `PAID` dengan nominal kurang (*underpayment*) | 200 OK (Booking terkonfirmasi tanpa cek nominal) | **422 Unprocessable Entity** (`PAYMENT_AMOUNT_MISMATCH`), booking tetap `pending` |
| Callback `PAID` dengan mata uang tidak cocok (misal USD) | 200 OK | **422 Unprocessable Entity** (`PAYMENT_CURRENCY_MISMATCH`), booking tetap `pending` |
| Callback `PAID` dengan invoice ID palsu/tidak tercatat di attempt | 200 OK | **422 Unprocessable Entity** (`INVOICE_ID_MISMATCH`), booking tetap `pending` |
| Callback `PAID` sah dengan nominal & invoice cocok | 200 OK | **200 OK**, booking berubah menjadi `confirmed` |
| Callback `PAID` duplikat (idempotent replay) | 200 OK | **200 OK** (`idempotent replay`) tanpa duplikasi notifikasi |
| Callback `EXPIRED` tiba terlambat pada booking yang sudah `confirmed` (*out-of-order*) | Booking terbatalkan secara keliru | **200 OK** (`stale expiry event ignored`), booking tetap `confirmed` |
| Callback `EXPIRED` pada booking `pending` | Booking dibatalkan | **200 OK** (`booking cancelled due to invoice expiry`), booking menjadi `cancelled` |

---

## 4. Kesimpulan

Perbaikan **BE-R14 (P1)** berhasil diimplementasikan dan diverifikasi 100%. Webhook Xendit kini memvalidasi nominal, mata uang, dan invoice ID terhadap buku besar `payment_attempts`, serta kebal terhadap pembatalan keliru akibat event kedaluwarsa yang tiba terlambat.
