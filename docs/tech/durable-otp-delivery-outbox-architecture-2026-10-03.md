# Technical Architecture & Design: Pengiriman OTP Andal Berbasis Outbox (BE-R17)

**Nomor Dokumen:** ARCH-PULANG-BE-R17  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering & Architecture Agent  
**Target Fitur / Gap:** [`BE-R17`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/gap/10-payment-recovery-and-provider-reaudit-2026-10-03.md)  
**Dokumen Terkait:** [PRD-PULANG-BE-R17](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/durable-otp-delivery-outbox-r17-2026-10-03.md), [SRS-PULANG-BE-R17](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/durable-otp-delivery-outbox-r17-2026-10-03.md)  

---

## 1. Arsitektur Komponen & Diagram Alir (Mermaid)

Sistem mengadopsi **Transactional Outbox Pattern** untuk memisahkan siklus penyimpanan tantangan kode OTP dengan proses jaringan eksternal ke provider email (Resend / SMTP).

### 1.1 Sequence Diagram: Permintaan Tantangan OTP & Asynchronous Durable Dispatch

```mermaid
sequenceDiagram
    autonumber
    actor Guest as Tamu (Client)
    participant API as HTTP API Handler
    participant Svc as Guest Service
    participant Store as PostgresStore
    participant DB as PostgreSQL (Challenges + Outbox)
    participant Relay as OutboxRelay Worker
    participant Resend as Resend Email Gateway

    Guest->>API: POST /api/v1/auth/guest/challenge {email}
    API->>Svc: RequestChallenge(ctx, email)
    Svc->>Svc: Generate 6-digit OTP & CodeHash
    Svc->>Store: CreateChallengeWithCooldown(c, cooldown)
    
    rect rgb(240, 248, 255)
        Note over Store,DB: Single PostgreSQL Transaction (BEGIN ... COMMIT)
        Store->>DB: Lock advisory per email
        Store->>DB: Check cooldown
        Store->>DB: INSERT INTO guest_auth_challenges RETURNING id
        Store->>DB: INSERT INTO outbox (topic='guest.otp_dispatch', payload)
        Store->>DB: COMMIT
    end
    
    Store-->>Svc: Success (ID, 60s cooldown)
    Svc-->>API: 60s cooldown
    API-->>Guest: HTTP 200 OK {"status":"ok","delivery_status":"accepted"}

    par Asynchronous Processing
        Relay->>DB: SELECT FOR UPDATE SKIP LOCKED FROM outbox WHERE topic='guest.otp_dispatch'
        DB-->>Relay: Row (challenge_id, email, otp_code, expires_at)
        Relay->>DB: Check if challenge expired or already verified
        alt Challenge Still Valid
            Relay->>Resend: POST /emails (Idempotency-Key: otp-challenge-{id})
            alt Resend Succeeded (HTTP 200)
                Resend-->>Relay: 200 OK (id: re_xxx)
                Relay->>DB: UPDATE outbox SET status='done'
            else Resend Failed / Timeout (5xx/Timeout)
                Resend-->>Relay: 5xx / Network Timeout
                Relay->>DB: UPDATE outbox SET attempts=attempts+1, next_retry_at=now()+backoff
            end
        else Challenge Expired / Verified
            Relay->>DB: UPDATE outbox SET status='done' (discarded)
        end
    end
```

---

## 2. Struktur Modul & Dependency Injection

```mermaid
classDiagram
    class Challenge {
        +string ID
        +string Email
        +string CodeHash
        +string PlainCode
        +int Attempts
        +int MaxAttempts
        +time.Time ExpiresAt
        +time.Time CreatedAt
    }

    class GuestStore {
        <<interface>>
        +CreateChallengeWithCooldown(ctx, c, cooldown) error
        +VerifyAndConsumeChallenge(ctx, email, hash, now, session) (*GuestSession, error)
        +GetLatestActiveChallenge(ctx, email) (*Challenge, error)
    }

    class OutboxRelay {
        +Pool *pgxpool.Pool
        +BatchSize int
        +MaxAttempts int
        +Handlers map[string]HandlerFunc
        +Run(ctx, interval)
    }

    class ResendNotifier {
        +BaseURL string
        +APIKey string
        +SendGuestOTP(ctx, email, otpCode, challengeID...) error
    }

    GuestStore <|.. PostgresStore
    PostgresStore --> Challenge : creates challenge & outbox event
    OutboxRelay --> ResendNotifier : dispatches with per-challenge idempotency
```

---

## 3. Strategi Retry & Dead-Letter Queue

| Percobaan (*Attempt*) | Jeda Waktu (*Backoff Delay*) | Status Outbox | Tindakan |
| :--- | :--- | :--- | :--- |
| **Attempt 1** | Instan (interval polling relay) | `pending` | Mengirim ke Resend API |
| **Attempt 2** | +5 detik | `pending` | Retry dengan Idempotency-Key sama |
| **Attempt 3** | +10 detik | `pending` | Retry dengan Idempotency-Key sama |
| **Attempt 4** | +20 detik | `pending` | Retry dengan Idempotency-Key sama |
| **Attempt 5** | +40 detik | `pending` | Retry dengan Idempotency-Key sama |
| **Attempt 6** | +80 detik (~1.3 menit) | `pending` | Retry dengan Idempotency-Key sama |
| **Attempt 7** | +160 detik (~2.6 menit) | `pending` | Retry jika challenge belum expired |
| **Attempt 8** | +300 detik (5 menit cap) | `failed` (Dead Letter) | Dicatat ke log dead letter, dihentikan |

---

## 4. Analisis Anti-Overengineering (Ponytail)

- **Tidak menambah message broker terpisah:** Memanfaatkan tabel `outbox` yang sudah ada pada skema PostgreSQL tanpa perlu menambahkan Redis queue baru atau Kafka.
- **Transaksional murni:** Tidak memerlukan library *two-phase commit* atau distributed transaction coordinator; cukup satu `pgx.Tx` standar Go.
- **Idempotensi cerdas:** Header HTTP `Idempotency-Key` didukung secara *native* oleh Resend API dan gateway email modern.
