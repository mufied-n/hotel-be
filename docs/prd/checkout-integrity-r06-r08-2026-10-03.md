# PRD — Checkout Integrity (BE-R06, BE-R08)

Sumber temuan: [dokumen 09](../gap/09-checkout-pricing-and-policy-reaudit-2026-10-03.md) dan [laporan audit E2E](../../testing/e2e/report/2026-10-03-132800-end-to-end-audit-e2e-report.md).

## Latar belakang bisnis
Audit E2E 3 Okt 2026 membuktikan dua cacat pada `POST /api/v1/bookings` untuk Pulang ke Uttara (95 kamar, bintang 4):
- **R06** — tanpa `quote_id`, booking terbentuk dengan pajak 10% hilang (1.500.000 vs 1.650.000), tanpa consent, dan tanpa kebijakan pembatalan WIB. Risiko pendapatan dan UU PDP.
- **R08** — 8 request paralel dengan `Idempotency-Key` sama membuat 4 booking dan 4 hold. Pada Xendit live, ini berarti beberapa invoice untuk satu klik.

## Persona
| Persona | Kebutuhan |
|---|---|
| Tamu (guest) | Satu klik = satu booking dengan harga dan kebijakan yang dilihatnya di quote. Retry jaringan aman. |
| Revenue manager | Semua booking memakai harga quote resmi (termasuk pajak). |
| Finance | Satu intent checkout = satu invoice. |

## Acceptance criteria
1. Create tanpa `quote_id` ditolak 400 `QUOTE_REQUIRED` sebelum inventory, booking, atau gateway berubah.
2. Quote tidak ditemukan/expired → 410 `QUOTE_EXPIRED`; mismatch → 400 `QUOTE_MISMATCH`; consent false → 400 `CONSENT_REQUIRED`.
3. 20 request paralel dengan key dan body sama menghasilkan tepat 1 booking, 1 hold, 1 payment attempt; sisanya menerima replay (201 + `Idempotency-Replayed: true`) atau 409 `IDEMPOTENCY_IN_PROGRESS` + `Retry-After`.
4. Key sama dengan body berbeda → 409 `IDEMPOTENCY_CONFLICT`, tanpa mutasi.
5. Create gagal (4xx/5xx) melepas key sehingga client dapat retry.
6. Error store idempotency → 503, bukan dianggap "key baru".

## Non-functional
- Coverage paket `internal/api` dan `internal/booking` tidak turun; kode baru ≥ 80%.
- Tanpa dependency baru dan tanpa migrasi baru.
