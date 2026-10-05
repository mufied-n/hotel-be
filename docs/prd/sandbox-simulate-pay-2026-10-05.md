# PRD — Simulasi Bayar Sandbox (Post-Checkout)

**Tanggal:** 2026-10-05 · **Status:** Implemented (FE) · **Repo terkait:** `hotel-fe` (BFF + UI), `hotel-be` (`/fake-pay`, tanpa perubahan)

## 1. Latar Belakang
Setelah checkout, booking berstatus `PENDING` (hold 30 menit). Payment gateway riil belum terintegrasi; `FakeGateway` mengembalikan URL lokal (`localhost:8080/fake-pay/...`) yang **disaring BFF** di produksi sehingga tombol "Lanjut pembayaran" tidak muncul. Tamu/penguji menemui jalan buntu: tidak ada cara membayar dan tidak ada email.

## 2. Keputusan
| Pertanyaan | Keputusan |
|---|---|
| Kirim email "segera bayar" saat checkout? | **Tidak.** Belum ada instruksi bayar nyata (VA/rekening); hemat kuota Resend. Email hanya untuk `CONFIRMED` (voucher). |
| Pembayaran sandbox | **Opsi B:** booking tetap `PENDING` (timer hold terlihat), halaman status menampilkan tombol **"Simulasi Bayar Sekarang (Sandbox)"**. Klik → `CONFIRMED` → email voucher Resend terkirim via outbox `booking.confirmed`. |

## 3. Persona & Acceptance Criteria
- **Guest/Penguji:** AC1 tombol tampil hanya jika status `pending`/`pending_payment`, mode API, dan flag sandbox aktif. AC2 setelah klik, status menjadi "Terkonfirmasi" dan email voucher diterima. AC3 hold kedaluwarsa → pesan error `HOLD_EXPIRED`, bukan konfirmasi.
- **Staff Front Desk:** melihat booking `confirmed` setelah simulasi (SSE `booking.confirmed`).

## 4. Non-Functional
- Fitur **default OFF** (`NUXT_PUBLIC_SANDBOX_PAY` ≠ `true` → endpoint BFF 404, tombol tersembunyi).
- Backend `/fake-pay` hanya terpasang jika `APP_ENV != production`; produksi live otomatis aman (404).
- Wajib sesi pemilik booking (`bookingAccess[id].token`) — tamu lain tak bisa memicu.
- Dihapus saat gateway riil (Xendit/Midtrans) diaktifkan.
