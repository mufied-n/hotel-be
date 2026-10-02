# Verifikasi dan urutan remediasi

Audit: 3 Oktober 2026. Evidence source-level kecuali disebutkan. Prioritas adalah gate untuk replacement live; trial UI fixtures dapat berjalan paralel.

## BE-G20 — Bukti verifikasi DB dan payment belum tersedia

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service_test.go:312](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service_test.go:312), [internal/api/router_test.go:109](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router_test.go:109), [internal/workers/outbox_test.go:1](/mnt/code/projects/jobs/pulang/current-booking/internal/workers/outbox_test.go:1).

**Gap dan dampak:** Race+coverage dan vet lulus, namun fake/mock tidak menjalankan SQL row locks, GiST, DB rollback, outbox commit failure, provider webhook atau real delivery. Tidak ada integration suite di file yang diperiksa. Coverage tinggi satu paket tidak membuktikan integrity produksi.

**Tindakan:** Tambahkan integration tests PG18/Valkey isolated dan payment sandbox sesuai BE-G09–G18; failure injection/concurrency/replay. Simpan command/env/build/source hash/result, bedakan release evidence dari mock.

**Kriteria verifikasi:** Last room N concurrent requests→tepat available successes; missing night rollback; expiry/payment race; assignment conflict; queue down sweep; provider duplicate event; restore drill sebelum live.

## Urutan eksekusi dan dependency

| Batch / PR | Gap | Dependensi | Gate / downstream |
|---|---|---|---|
| BE-A: boundary protection | G10(dev gate), G13, G14, G15 | Pilihan guest access/staff auth | Public UI tidak bisa mutate staff; fake disabled produksi |
| BE-B: katalog & sellability | G01, G02, G03, G18(horizon/allocation) | Inventaris owner hotel | IDs/capacity/media stabil untuk fixture→API mapping |
| BE-C: rate/quote/policy | G04, G05, G06, G08, G19 | BE-B; keputusan benefit/rounding/policy | Results/review menampilkan harga/policy yang sama |
| BE-D: checkout & recovery | G07, G09, G11, G12 | BE-A/C; provider sandbox | Submit/retry tidak duplicate stock/charge; status server authoritative |
| BE-E: operational reliability | G16, G17, G21, G22 | BE-D; staff policies | DB concurrent assignment, outbox retry/delivery, worker recovery |
| BE-F: live switch gate | G18(channel), G20 | Semua gate live di atas | Reconcile stock, sandbox payment/email, rollback & restore drill |

Setiap PR wajib mencatat gap ID, kontrak yang berubah, migrasi/data impact, test nyata yang dijalankan, dan handoff ke FE. Skema detail bukan hasil audit ini; schema change perlu review data volumes, constraints dan rollout dari owner backend.

## TODO per owner

- [ ] Owner hotel: mapping sellable variants/rooms, occupancy anak, breakfast entitlement, policy/promo/benefit dan Money rounding.
- [ ] Backend: endpoint contract proposal, access model, quote snapshots, idempotency dan durable payment/hold.
- [ ] Backend/ops: authoritative channel stock, maintenance, admin exception, late-payment refund runbook.
- [ ] QA: real PG18/Valkey concurrency + failure/replay matrix; payment/email sandbox.
- [ ] FE: mulai fixture UI; jangan menggunakan public UUID GET/cancel atau fake-pay sebagai integrasi produksi.

## Penutupan gap

Status RESOLVED hanya setelah source fix + acceptance evidence tertaut. Status IMPLEMENTED_UNVERIFIED boleh dipakai setelah source fix tanpa integration evidence. DEFERRED memerlukan scope dan dampak yang eksplisit. Jangan menutup gap berdasarkan dokumen rencana, screenshot, build, atau unit mock saja.
