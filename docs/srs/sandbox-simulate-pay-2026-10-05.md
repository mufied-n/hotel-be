# SRS — Simulasi Bayar Sandbox

**Tanggal:** 2026-10-05 · PRD: [sandbox-simulate-pay-2026-10-05.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/sandbox-simulate-pay-2026-10-05.md)

## Functional Requirements
- **FR-01** BFF menyediakan `POST /api/bff/bookings/{id}/simulate-pay`.
- **FR-02** Endpoint mengembalikan 404 bila `public.sandboxPay` false.
- **FR-03** Endpoint mewajibkan header `X-Pulang-CSRF` / same-origin (`assertMutationRequest`) dan sesi dengan `bookingAccess[id].token` (401 jika tidak ada).
- **FR-04** BFF meneruskan ke backend `POST /fake-pay/{id}?booking_id={id}` → `bkSvc.Confirm` (idempoten).
- **FR-05** Setelah sukses BFF menghapus `paymentUrl` tersimpan di sesi.
- **FR-06** UI menampilkan tombol hanya bila `bookingMode=api`, `sandboxPay=true`, status `pending|pending_payment`; setelah klik halaman memanggil ulang status.

## Kontrak HTTP
`POST /api/bff/bookings/{id}/simulate-pay` — header `X-Pulang-CSRF: 1`, tanpa body.

| Status | Body | Kondisi |
|---|---|---|
| 200 | `{"status":"confirmed"}` | Sukses / sudah confirmed (idempoten) |
| 401 | problem | Sesi tak memiliki akses booking |
| 404 | problem | Sandbox nonaktif, atau booking tidak ditemukan (`BOOKING_NOT_FOUND`) |
| 409 | `HOLD_EXPIRED` / `ILLEGAL_TRANSITION` | Hold habis / status tidak valid (mis. cancelled) |
| 503 | problem | Backend tidak terjangkau |
