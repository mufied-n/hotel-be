# Software Requirements Specification (SRS) — Pengiriman OTP Andal Berbasis Outbox & Anti-Enumeration (BE-R17)

**Nomor Dokumen:** SRS-PULANG-BE-R17  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering & Architecture Agent  
**Target Fitur / Gap:** [`BE-R17`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md)  
**Dokumen Terkait:** [PRD-PULANG-BE-R17](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/durable-otp-delivery-outbox-r17-2026-10-03.md), [F02 SRS](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/02-guest-access-and-session-2026-10-03.md)  

---

## 1. Kebutuhan Fungsional (Functional Requirements)

- **FR-01 (Transactional Outbox Insertion):**  
  Pada saat fungsi `guest.PostgresStore.CreateChallengeWithCooldown` berhasil membuat rekaman di tabel `guest_auth_challenges`, transaksi database yang sama wajib menyisipkan satu baris rekaman baru ke tabel `outbox` dengan:
  - `topic`: `"guest.otp_dispatch"`
  - `payload`: JSON objek berisi `challenge_id`, `email`, `otp_code`, `expires_at`
  - `status`: `"pending"`
  - `attempts`: 0
  - `next_retry_at`: `now()`

- **FR-02 (Outbox Relay OTP Handler):**  
  Menyediakan handler topic `"guest.otp_dispatch"` pada `workers.OutboxRelay`:
  ```go
  func HandleGuestOTPDispatch(ctx context.Context, pool *pgxpool.Pool, n Notifier, log *slog.Logger) func(ctx context.Context, payload []byte) error
  ```
  Alur eksekusi handler:
  1. Deserialisasi payload JSON.
  2. Periksa apakah `now() > expires_at`. Jika ya, abaikan pengiriman (*stale OTP*), catat log peringatan, dan kembalikan `nil` (outbox status `done`).
  3. Periksa tabel `guest_auth_challenges`: jika `verified_at IS NOT NULL`, abaikan pengiriman dan kembalikan `nil`.
  4. Panggil `n.SendGuestOTP(ctx, email, otpCode, challengeID)`.
  5. Jika `n.SendGuestOTP` berhasil, kembalikan `nil` (outbox status `done`).
  6. Jika gagal, kembalikan `error` agar *OutboxRelay* menjadwalkan retry dengan *exponential backoff*.

- **FR-03 (Per-Challenge Idempotency di Resend Notifier):**  
  Pada fungsi `ResendNotifier.SendGuestOTP(ctx, email, otpCode, challengeID ...string)`:
  - Jika argumen `challengeID` tersedia dan tidak kosong, header HTTP wajib diset:
    `Idempotency-Key: otp-challenge-<challengeID>`
  - Jika argumen tidak tersedia (fallback backward compatibility):
    `Idempotency-Key: otp-<email>-<unix_minute>`

- **FR-04 (Honest Anti-Enumeration API Response):**  
  Handler `handleGuestChallenge` pada endpoint `POST /api/v1/auth/guest/challenge` wajib mengembalikan respons:
  - HTTP Status: `200 OK`
  - Payload JSON:
    ```json
    {
      "status": "ok",
      "delivery_status": "accepted",
      "message": "Permintaan kode verifikasi telah diterima. Jika alamat email valid, kode verifikasi 6 digit akan dikirimkan ke email Anda.",
      "cooldown_seconds": 60
    }
    ```

- **FR-05 (Redaksi Data Sensitif & Audit Log):**  
  Log worker outbox dilarang mencatat `otp_code` plaintext. Nilai email wajib disanitasi/dimask bila diperlukan dan hanya mencatat `challenge_id`, `recipient`, dan `status`.

---

## 2. Spesifikasi Antarmuka HTTP & JSON Payload

### 2.1 Request Permintaan Tantangan OTP
- **Method:** `POST`
- **Path:** `/api/v1/auth/guest/challenge`
- **Headers:** `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "email": "tamu@example.com"
  }
  ```

### 2.2 Response Sukses (Accepted)
- **Status Code:** `200 OK`
- **Response Body:**
  ```json
  {
    "status": "ok",
    "delivery_status": "accepted",
    "message": "Permintaan kode verifikasi telah diterima. Jika alamat email valid, kode verifikasi 6 digit akan dikirimkan ke email Anda.",
    "cooldown_seconds": 60
  }
  ```

### 2.3 Response Rate Limit Exceeded
- **Status Code:** `429 Too Many Requests`
- **Response Body:**
  ```json
  {
    "error": "RATE_LIMIT_EXCEEDED",
    "message": "Harap tunggu 60 detik sebelum meminta kode verifikasi baru."
  }
  ```

---

## 3. Skema Data & Kontrak Antrian Outbox

### 3.1 Skema Tabel `outbox` (Ref: `migrations/00001_init.sql`)
```sql
CREATE TABLE IF NOT EXISTS outbox (
    id            BIGSERIAL PRIMARY KEY,
    topic         TEXT NOT NULL,
    payload       JSONB NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'failed')),
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.2 Struktur Payload Event `guest.otp_dispatch`
```json
{
  "challenge_id": "01923456-789a-7bc8-9012-3456789abcde",
  "email": "tamu@example.com",
  "otp_code": "481920",
  "expires_at": "2026-10-03T17:10:00Z"
}
```
