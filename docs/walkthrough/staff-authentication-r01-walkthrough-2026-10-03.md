# Walkthrough — Autentikasi Staf (BE-R01)

[PRD](../prd/staff-authentication-r01-2026-10-03.md) · [SRS](../srs/staff-authentication-r01-2026-10-03.md) · [Tech](../tech/staff-authentication-architecture-2026-10-03.md)

- [x] 1 Research (audit E2E, kode `middleware.go`, `staff_users`, pola `internal/guest`)
- [x] 2 PRD/SRS/Tech
- [x] 3 TDD `internal/staffauth`
- [x] 4 TDD middleware + handler + wiring `cmd/server` + CLI
- [x] 5 E2E (stack terisolasi, forged credential ditolak)
- [x] 6 Verifikasi

## Log
- 2026-10-03: `internal/staffauth` (88%), middleware `IdentifySubject(verifier)`, handler login/me/logout, CLI `cmd/staffadmin`, migrasi 00017, wiring `cmd/server`.
- E2E: `staff_auth_e2e.sh` 20/20; `checkout_integrity_e2e.sh` 15/15; `go vet` bersih; `go test -race -cover ./...` hijau (api 85%).
- Skrip legacy diperbarui (quote_id, fixture `lib_fixtures.sh`, key unik); seluruh 9 skrip E2E lulus, rbac 40/40 diulang.
- Laporan: `testing/e2e/report/2026-10-03-134800-staff-authentication-e2e-report.md`
