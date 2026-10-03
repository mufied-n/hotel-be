# Tech Architecture — Autentikasi Staf (BE-R01)

## Keputusan (anti-overengineering)
- Pakai tabel `staff_users` yang ada + 1 tabel `staff_sessions`; token opak acak (bukan JWT), sehingga pencabutan dan nonaktif langsung berlaku tanpa daftar revokasi atau manajemen kunci.
- Satu paket `internal/staffauth` (service + store Postgres); middleware hanya mengenal interface `StaffVerifier`.
- Fixture test `api.TestStaffVerifier()` (pola seperti `auth.DefaultTestEnforcer`) memetakan token `finance`, `gm_admin`, dst. ke principal, sehingga test lama tetap memakai `Bearer <role>`. Tidak pernah dipasang di `cmd/server`.

```mermaid
sequenceDiagram
    participant S as Staff
    participant A as API
    participant M as IdentifySubject
    participant V as staffauth.Service
    participant DB as Postgres
    S->>A: POST /auth/staff/login
    A->>V: Login(user, pass)
    V->>DB: staff_users (hash, lock) + INSERT staff_sessions(token_hash)
    A-->>S: stf_token
    S->>M: Authorization Bearer stf_token
    M->>V: Verify(token)
    V->>DB: join sessions+users (not revoked, not expired, active)
    M-->>A: role dari DB → Casbin Authorize
```

## Skema (migrasi 00017)
`staff_users` + `password_hash TEXT`, `failed_attempts INT`, `locked_until TIMESTAMPTZ`.
`staff_sessions(id uuid pk, staff_id uuid fk, token_hash char(64) unique, created_at, expires_at, revoked_at)`.

## Risiko dan batas
- Tanpa MFA, tanpa rotasi/refresh, tanpa audit event per aksi (F12 lanjutan). Rate limit login mengandalkan lockout per akun + limiter IP global.
- Menjalankan hotel nyata mewajibkan `APP_ENV=production`, password staf diset via CLI, dan TLS di depan server.
