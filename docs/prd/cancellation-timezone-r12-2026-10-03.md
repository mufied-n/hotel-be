# Product Requirements Document (PRD) — Cancellation Deadline Timezone Alignment (BE-R12)

**Fitur:** Cancellation Deadline WIB Timezone Alignment  
**ID Gap:** BE-R12 (P1)  
**Terkait:** BE-G08, BE-G19, F06/F13  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  
**Target:** Monolith Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)  

---

## 1. Konteks Bisnis & Masalah

Hotel *Pulang ke Uttara* (Yogyakarta) menetapkan kebijakan pembatalan tarif fleksibel (`flexible_48h`):
> *"Pembatalan gratis hingga 48 jam sebelum jam 14:00 WIB pada tanggal check-in. Pembatalan setelah batas waktu dikenakan biaya 100%."*

Zona waktu operasional hotel adalah **Waktu Indonesia Barat (WIB / Asia/Jakarta, UTC+7)**. Jam check-in resmi hotel untuk perhitungan pembatalan adalah **14:00 WIB**.

### Temuan Masalah (BE-R12)
1. **Perhitungan Jam Berbasis UTC Murni**: Pada implementasi awal di `internal/booking/service.go`, kode mengonstruksi waktu cutoff menggunakan:
   ```go
   checkInTime := time.Date(b.CheckIn.Year(), b.CheckIn.Month(), b.CheckIn.Day(), 14, 0, 0, 0, time.UTC)
   deadline := checkInTime.Add(-48 * time.Hour)
   ```
2. **Kesenjangan Waktu 7 Jam (The 7-Hour Window)**:
   - Tanggal Check-in: 10 Oktober 2026.
   - Kebijakan resmi: Batas waktu adalah 48 jam sebelum 10 Oktober 14:00 WIB $\rightarrow$ **8 Oktober 14:00 WIB (07:00:00 UTC)**.
   - Implementasi lama: Menetapkan batas waktu 8 Oktober 14:00 UTC $\rightarrow$ setara dengan **8 Oktober 21:00 WIB**.
   - **Dampak Finansial**: Tamu dapat membatalkan pesanan secara gratis hingga 7 jam melampaui batas waktu resmi hotel. Kamar yang dilepas terlambat berisiko tidak terjual kembali (*lost revenue*), dan menimbulkan perselisihan antara tamu dengan staf *Front Desk*.

---

## 2. Kriteria Penerimaan (Acceptance Criteria)

- [x] **AC-01 (WIB Timezone Enforcement):** Batas waktu pembatalan gratis dihitung secara presisi berbasis zona waktu WIB (UTC+7, `Asia/Jakarta`). Untuk check-in tanggal $D$, batas waktu cutoff pembatalan gratis adalah $D - 2 \text{ hari}$ tepat pukul 14:00:00 WIB (atau $07:00:00\text{ UTC}$).
- [x] **AC-02 (Eliminasi Gap 7 Jam):** Permintaan pembatalan yang dilakukan pada interval $[14:00:01\text{ WIB}, 21:00:00\text{ WIB}]$ pada H-2 **wajib ditolak** dengan error `ErrCancellationDeadlineExceeded` (HTTP 409 Conflict: `CANCELLATION_DEADLINE_EXCEEDED`).
- [x] **AC-03 (Clock-Controlled Determinism):** Layanan booking menyediakan injeksi provider waktu (`SetNowFunc`) agar pengetesan boundary waktu (1 detik sebelum, tepat waktu, 1 detik sesudah) dapat diuji secara deterministik tanpa flakiness dan independen dari timezone host runtime.
- [x] **AC-04 (Konsistensi Kebijakan Non-Refundable & Hold Release):** Kebijakan `non_refundable` untuk booking `confirmed` tetap ditolak pembatalan oleh tamu (`ErrNonRefundable`), sedangkan booking `pending` (hold) tetap dapat dibatalkan (dilepas) kapan pun.
