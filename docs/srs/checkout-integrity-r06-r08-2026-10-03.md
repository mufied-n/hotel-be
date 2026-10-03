# SRS — Checkout Integrity (BE-R06, BE-R08)

## Functional requirements
- **FR-01** `booking.Service.Create` mengembalikan `ErrQuoteRequired` bila `QuoteID == ""`, sebelum akses inventory/gateway. Jalur fallback harga dasar dihapus.
- **FR-02** Urutan validasi create: field dasar → `QuoteID` kosong → consent → lookup quote → match parameter.
- **FR-03** `IdempotencyStore.Reserve(ctx, key, hash)` atomik: insert baris `response_code=0` (in-progress, TTL 2 menit) atau mengembalikan baris aktif. Baris kedaluwarsa dapat diambil alih.
- **FR-04** `Complete(ctx, rec)` menyimpan respons (TTL 24 jam). `Release(ctx, key)` menghapus reservasi bila create gagal.
- **FR-05** Handler `createBooking`: Reserve → (acquired) proses → Complete; (tidak acquired) hash beda → 409 conflict; selesai → replay; in-progress → 409 in-progress.

## Kontrak HTTP `POST /api/v1/bookings`
Request: JSON seperti sekarang; `quote_id`, `terms_accepted`, `privacy_accepted` wajib.

| Kondisi | Status | `code` |
|---|---|---|
| quote_id kosong | 400 | `QUOTE_REQUIRED` |
| consent false | 400 | `CONSENT_REQUIRED` |
| quote expired/tidak ada | 410 | `QUOTE_EXPIRED` |
| quote tidak cocok | 400 | `QUOTE_MISMATCH` |
| key sama, body beda | 409 | `IDEMPOTENCY_CONFLICT` |
| key sama, request pertama masih berjalan | 409 + `Retry-After: 1` | `IDEMPOTENCY_IN_PROGRESS` |
| key sama, body sama, selesai | 201 + `Idempotency-Replayed: true` | — |
| store idempotency error | 503 | `IDEMPOTENCY_UNAVAILABLE` |
| sukses | 201 | — |
