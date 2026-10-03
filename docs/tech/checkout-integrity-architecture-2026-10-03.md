# Tech Architecture — Checkout Integrity (BE-R06, BE-R08)

## Keputusan desain (anti-overengineering)
- **R06**: hapus cabang fallback di `Service.Create` dan tambah satu guard. Tidak ada endpoint/flag baru.
- **R08**: satu pernyataan SQL `INSERT ... ON CONFLICT DO UPDATE ... WHERE expires_at <= NOW() RETURNING` memberi klaim atomik tanpa tabel atau migrasi baru. Tabel `idempotency_keys` (migrasi 00007) dipakai ulang; `response_code = 0` menandai in-progress.
- Tidak memakai advisory lock atau Valkey: Postgres sudah menjadi sumber kebenaran booking.

```mermaid
sequenceDiagram
    participant C as Client
    participant H as createBooking
    participant S as IdempotencyStore
    participant B as BookingSvc
    C->>H: POST (key K, body)
    H->>S: Reserve(K, hash)
    alt acquired
        H->>B: Create(quote_id, consent)
        alt sukses
            H->>S: Complete(K, 201, body)
            H-->>C: 201
        else gagal
            H->>S: Release(K)
            H-->>C: 4xx/5xx
        end
    else hash beda
        H-->>C: 409 IDEMPOTENCY_CONFLICT
    else in-progress
        H-->>C: 409 IDEMPOTENCY_IN_PROGRESS
    else selesai
        H-->>C: 201 replay
    end
```

## Skema DB
Tidak ada perubahan. `idempotency_keys(key PK, request_hash, response_code, response_body, created_at, expires_at)`.

## Risiko residual (ditutup)
- Jika proses mati setelah booking commit tetapi sebelum `Complete`, reservasi kedaluwarsa dalam 2 menit. Retry tidak dapat membuat booking kedua karena migrasi `00016` menjadikan `bookings.quote_id` unik: hasilnya 409 `QUOTE_ALREADY_USED`. Klien kehilangan akses ke booking itu hanya jika token tamu tidak tersimpan; dapat dipulihkan lewat OTP/Booking Saya.
