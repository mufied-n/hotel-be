# Software Requirements Specification (SRS) — Atomic OTP Challenge & Attempt Limiting (BE-R03)

**Fitur:** Atomic OTP Challenge, Attempt Limiting & Single-Use Verification  
**ID Gap:** BE-R03 (P1)  
**Terkait:** F02, BE-G13, BE-G14  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  

---

## 1. Functional Requirements

### FR-01: Pembuatan Tantangan Atomik dengan Cooldown
Paket `internal/guest` wajib menyediakan metode persistensi:
```go
CreateChallengeWithCooldown(ctx context.Context, c *Challenge, cooldown time.Duration) error
```
1. Mengeksekusi transaksi PostgreSQL dengan `pg_advisory_xact_lock(hashtext('guest_challenge:' || email))` untuk serialisasi pembuatan tantangan per alamat email.
2. Memeriksa waktu pembuatan tantangan terakhir untuk email tersebut. Jika `c.CreatedAt - latest.CreatedAt < cooldown`, transaksi di-rollback dan mengembalikan `ErrRateLimited`.
3. Jika memenuhi syarat, lakukan `INSERT INTO guest_auth_challenges` dan commit transaksi.

### FR-02: Konsumsi dan Verifikasi Tantangan Atomik
Paket `internal/guest` wajib menyediakan metode persistensi:
```go
VerifyAndConsumeChallenge(ctx context.Context, email, inputHash string, now time.Time, newSession *GuestSession) (*GuestSession, error)
```
1. Membuka transaksi PostgreSQL.
2. Mengambil dan mengunci baris tantangan aktif terakhir dengan `FOR UPDATE`:
   ```sql
   SELECT id, email, code_hash, attempts, max_attempts, expires_at, verified_at
   FROM guest_auth_challenges
   WHERE email = $1
   ORDER BY created_at DESC
   LIMIT 1
   FOR UPDATE;
   ```
3. Validasi status:
   - Jika tidak ada baris, atau `verified_at IS NOT NULL`, atau `now > expires_at`: rollback dan kembalikan `ErrInvalidOrExpiredCode`.
   - Jika `attempts >= max_attempts`: rollback dan kembalikan `ErrMaxAttemptsExceeded`.
4. Komparasi kode waktu konstan (`subtle.ConstantTimeCompare`):
   - **Jika Salah:**
     Lakukan increment `attempts = attempts + 1` langsung di database:
     ```sql
     UPDATE guest_auth_challenges SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts;
     ```
     Commit transaksi.
     Jika `attempts + 1 >= max_attempts`: kembalikan `ErrMaxAttemptsExceeded`.
     Jika belum: kembalikan `ErrInvalidOrExpiredCode`.
   - **Jika Benar:**
     Tandai verifikasi:
     ```sql
     UPDATE guest_auth_challenges SET verified_at = $2 WHERE id = $1 AND verified_at IS NULL;
     ```
     Buat sesi baru dalam transaksi yang sama:
     ```sql
     INSERT INTO guest_sessions (guest_email, token_hash, expires_at, last_active_at, created_at)
     VALUES ($1, $2, $3, $4, $5) RETURNING id;
     ```
     Commit transaksi. Kembalikan objek sesi yang tersimpan.

---

## 2. Kontrak HTTP & Error Respon

### Endpoint: `POST /api/v1/auth/guest/verify`

#### Skenario A: Verifikasi Sukses
- **Status:** `200 OK`
- **Body:**
```json
{
  "token": "gst_sess_1234567890abcdef...",
  "session": {
    "guest_email": "tamu@example.com",
    "expires_at": "2026-10-04T10:00:00Z"
  }
}
```

#### Skenario B: Kode Salah atau Kedaluwarsa
- **Status:** `401 Unauthorized`
- **Body:**
```json
{
  "error": "INVALID_OR_EXPIRED_CODE",
  "message": "Kode verifikasi salah atau sudah kedaluwarsa."
}
```

#### Skenario C: Batas Percobaan Terlampaui (Brute-Force Lockout)
- **Status:** `429 Too Many Requests`
- **Body:**
```json
{
  "error": "MAX_ATTEMPTS_EXCEEDED",
  "message": "Batas percobaan verifikasi telah terlampaui. Silakan minta kode baru."
}
```

### Endpoint: `POST /api/v1/auth/guest/challenge`

#### Skenario: Cooldown Aktif (< 60 detik)
- **Status:** `429 Too Many Requests`
- **Body:**
```json
{
  "error": "RATE_LIMITED",
  "message": "Permintaan OTP terlalu sering. Harap tunggu beberapa saat."
}
```
