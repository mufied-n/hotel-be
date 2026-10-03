# Walkthrough — Checkout Integrity (BE-R06, BE-R08)

Dokumen: [PRD](../prd/checkout-integrity-r06-r08-2026-10-03.md) · [SRS](../srs/checkout-integrity-r06-r08-2026-10-03.md) · [Tech](../tech/checkout-integrity-architecture-2026-10-03.md)

## Fase
- [x] 1 Research (audit E2E + pembacaan kode)
- [x] 2 PRD/SRS/Tech
- [x] 3 TDD R06
- [x] 4 TDD R08
- [x] 5 E2E (stack terisolasi)
- [x] 6 Verifikasi (`go vet`, `go test -race -cover ./...`)

## Log
- 13:30 Red: `TestCreate_QuoteRequired` gagal (create tanpa quote → nil error).
- 13:30 Green R06: guard `ErrQuoteRequired` + hapus fallback di `internal/booking/service.go`. 3 test lama dipaksa memakai quote (`withQuote`).
- 13:31 Test API/E2E runner disesuaikan (`mustQuoteID`, `e2eQuoteID`); E2E-42 memakai nominal refund jauh di atas saldo karena total kini termasuk pajak.
- 13:32 Red R08: `idempotency_test.go` (store, handler, store down, QUOTE_REQUIRED).
- 13:33 Green R08: `Reserve/Complete/Release` di `internal/api/idempotency.go` (memori + Postgres `INSERT ... ON CONFLICT`), handler `createBooking` memakai klaim atomik.
- 13:34 Integration test disesuaikan (`quoted`) + `TestRealDB_IdempotencyReserveAtomic`; `go mod tidy` hanya memindahkan `google/uuid` dari indirect.
- 13:35 E2E live: PASS=15 FAIL=0.

## File berubah
`internal/booking/service.go`, `internal/booking/service_test.go`, `internal/api/idempotency.go`, `internal/api/idempotency_test.go`, `internal/api/router.go`, `internal/api/router_test.go`, `testing/e2e/script/e2e_runner_test.go`, `testing/e2e/script/checkout_integrity_e2e.sh`, `testing/integration/postgres_concurrency_test.go`, `testing/integration/idempotency_test.go`, `go.mod`.

## Perintah
```bash
go vet ./...
TEST_DATABASE_URL=postgres://postgres:dev@localhost:25432/booking?sslmode=disable go test -race -count=1 -cover ./...
BASE_URL=http://localhost:28080 testing/e2e/script/checkout_integrity_e2e.sh
```
Laporan: [E2E report](../../testing/e2e/report/2026-10-03-133600-checkout-integrity-e2e-report.md)

## Tindak lanjut 13:40
`ErrQuoteAlreadyUsed` + migrasi `00016` + mapping 409; `db_helper.go` aman (`TEST_DATABASE_URL` wajib, DB `*_test`); tes `TestRealDB_QuoteSingleUse`, `TestRealDB_CheckInIdempotentReplay`, `TestBypassRoomReadinessContext`. Coverage booking gabungan 82.8%.
