# Technical Architecture & Design — Atomic OTP Challenge & Attempt Limiting (BE-R03)

**Fitur:** Atomic OTP Challenge, Attempt Limiting & Single-Use Verification  
**ID Gap:** BE-R03 (P1)  
**Terkait:** F02, BE-G13, BE-G14  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved / In Implementation  

---

## 1. Arsitektur Konkurensi & Penguncian Transaksional

```mermaid
sequenceDiagram
    autonumber
    actor Caller1 as Client 1 (Valid OTP)
    actor Caller2 as Client 2 (Concurrent Valid OTP)
    participant Svc as guest.DefaultService
    participant DB as PostgreSQL Transaction

    Note over Caller1,Caller2: Permintaan Verify Konkuren Serentak
    Caller1->>Svc: VerifyChallenge(email, code)
    Caller2->>Svc: VerifyChallenge(email, code)

    rect rgb(240, 248, 255)
    Note over Svc,DB: Client 1 Mengakuisisi Row Lock
    Svc->>DB: BEGIN TRANSACTION
    Svc->>DB: SELECT ... WHERE email = $1 FOR UPDATE
    DB-->>Svc: Row Locked (verified_at = NULL, attempts = 0)
    end

    rect rgb(255, 245, 245)
    Note over Svc,DB: Client 2 Menunggu Lock
    Svc->>DB: BEGIN TRANSACTION
    Svc->>DB: SELECT ... WHERE email = $1 FOR UPDATE (BLOCKING / WAITING)
    end

    Note over Svc,DB: Client 1 Eksekusi Mutasi Atomik
    Svc->>DB: UPDATE guest_auth_challenges SET verified_at = NOW()
    Svc->>DB: INSERT INTO guest_sessions (...) RETURNING id
    Svc->>DB: COMMIT TRANSACTION
    Svc-->>Caller1: 200 OK (Session Token Terbit)

    rect rgb(255, 245, 245)
    Note over Svc,DB: Client 2 Membaca Baris yang Sudah Terupdate
    DB-->>Svc: Lock Diberikan (verified_at = NOT NULL!)
    Note over Svc: verified_at tidak null -> Kode hangus
    Svc->>DB: ROLLBACK TRANSACTION
    Svc-->>Caller2: 401 Unauthorized (ErrInvalidOrExpiredCode)
    end
```

---

## 2. Invarian Database & Mekanisme Penguncian

### 2.1 Cooldown Request Challenge
- **Mekanisme**: `pg_advisory_xact_lock(hashtext('guest_challenge:' || $email))`.
- **Keunggulan**: Mengunci ruang transaksi berdasarkan hash string email tamu tanpa mengunci tabel atau mempengaruhi tamu lain. Lock dilepas otomatis saat commit atau rollback.

### 2.2 Strict Attempt Limiting
- **Kueri**:
  ```sql
  UPDATE guest_auth_challenges
  SET attempts = attempts + 1
  WHERE id = $1
  RETURNING attempts;
  ```
- **Keunggulan**: Increment dijalankan di level engine PostgreSQL di dalam transaksi terisolasi row-lock, meniadakan race condition *read-modify-write* di aplikasi.

---

## 3. Analisis Anti-Overengineering (Ponytail)

- **Zero Schema Change**: Tabel `guest_auth_challenges` dan `guest_sessions` sudah memiliki kolom `attempts`, `max_attempts`, `verified_at`, dan `expires_at`. Tidak memerlukan migrasi skema tabel baru.
- **Native ACID PostgreSQL**: Memanfaatkan kapabilitas transaksi `FOR UPDATE` dan `pg_advisory_xact_lock` bawaan PostgreSQL tanpa menambah dependensi external queue atau distributed locking eksternal yang rumit.
