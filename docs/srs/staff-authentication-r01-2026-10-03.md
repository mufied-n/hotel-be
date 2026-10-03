# SRS — Autentikasi Staf (BE-R01)

Pasangan: [PRD](../prd/staff-authentication-r01-2026-10-03.md).

## Functional requirements
- **FR-01** `POST /api/v1/auth/staff/login`: verifikasi bcrypt, cek `is_active` dan `locked_until`, buat sesi.
- **FR-02** `IdentifySubject(verifier)` hanya menerima `Authorization: Bearer stf_<token>` dan memverifikasinya ke store; header peran lain dihapus dari logika. Token berawalan `stf_` yang tidak valid → 401 `AUTHENTICATION_REQUIRED` (tidak jatuh menjadi tamu). Bearer lain (token sesi tamu) tidak disentuh.
- **FR-03** Verifikasi join `staff_sessions` dan `staff_users`: tidak dicabut, belum kedaluwarsa, `is_active = true`; role dari `staff_users.role`.
- **FR-04** `POST /api/v1/auth/staff/logout` mencabut sesi saat ini. `GET /api/v1/auth/staff/me` mengembalikan principal.
- **FR-05** Lockout: `failed_attempts` naik tiap gagal, reset saat sukses; ≥ 5 → `locked_until = now + 15m`.
- **FR-06** CLI `cmd/staffadmin set-password <username>` (password dari `STAFF_PASSWORD` atau stdin), min 12 karakter, bcrypt cost 12.
- **FR-07** Authorization Casbin tidak berubah (role dari principal).

## Kontrak HTTP
`POST /api/v1/auth/staff/login` — body `{"username":"fo_receptionist","password":"..."}`
200: `{"token":"stf_...","expires_at":"RFC3339","staff":{"username":"fo_receptionist","role":"receptionist","full_name":"..."}}`

| Kondisi | Status | `code` |
|---|---|---|
| body/field kosong | 400 | `INVALID_REQUEST` |
| user tidak ada / password salah / nonaktif / tanpa password | 401 | `INVALID_CREDENTIALS` |
| terkunci | 429 + `Retry-After` | `ACCOUNT_LOCKED` |
| store error | 503 | `AUTH_UNAVAILABLE` |
| `/me`, `/logout` tanpa sesi staf valid | 401 | `AUTHENTICATION_REQUIRED` |

Route staf lain: tanpa token valid → diperlakukan tamu → 403 (Casbin) seperti sekarang; token `stf_` tidak valid → 401.
