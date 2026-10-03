# PRD — Autentikasi Staf (BE-R01, P0)

Sumber: [dokumen 08](../gap/08-security-and-guest-session-reaudit-2026-10-03.md), [laporan audit E2E](../../testing/e2e/report/2026-10-03-132800-end-to-end-audit-e2e-report.md). Irisan minimal dari [PRD-F12](12-staff-identity-permissions-and-audit-2026-10-03.md) (F12-AC-01 dan F12-AC-03); MFA, IdP eksternal, approval role, dan audit event ditunda.

## Latar belakang
Audit E2E membuktikan `Authorization: Bearer gm_admin` atau `X-User-Role: finance` tanpa akun memberi akses penuh ke finance, housekeeping, front desk, dan stay. Data tamu (PII), refund, dan status kamar dapat dibaca atau diubah siapa pun. Ini memblokir deployment publik.

## Persona
receptionist, housekeeping, revenue_mgr, finance, gm_admin (tabel `staff_users`, 5 akun seed tanpa password).

## Acceptance criteria
1. Staf login dengan `username` + `password`; sukses menghasilkan token sesi acak (`stf_...`) dengan masa berlaku 8 jam. Token hanya disimpan sebagai hash SHA-256.
2. Nama role literal sebagai Bearer, `X-User-Role`, `X-User-ID`, `X-Internal-Secret`, dan `X-Testing-Role` tidak memberi hak apa pun (diperlakukan sebagai tamu).
3. Role diambil server dari `staff_users` saat tiap request; akun nonaktif atau sesi dicabut/kedaluwarsa langsung ditolak (401).
4. Kredensial salah, user tidak ada, dan akun nonaktif menghasilkan respons identik (401 `INVALID_CREDENTIALS`) tanpa membocorkan keberadaan user.
5. 5 kegagalan berturut-turut mengunci akun 15 menit (429 `ACCOUNT_LOCKED`).
6. Logout mencabut sesi; token yang sama kemudian ditolak.
7. Akun seed tanpa password tidak dapat login. Password awal diset operator lewat CLI, minimal 12 karakter, disimpan bcrypt.
8. Tidak ada jalur fail-open: verifier tidak terkonfigurasi atau DB error pada verifikasi berarti request staf ditolak (401/503), bukan menjadi tamu berhak staf.

## Non-functional
Respons login `Cache-Control: no-store`; password dan token tidak pernah masuk log. Coverage paket baru ≥ 80%. Dependency baru: `golang.org/x/crypto/bcrypt` (sudah ada di go.sum sebagai indirect).
