# Checkout, policy dan idempotency

Audit: 3 Oktober 2026. Evidence source-level kecuali disebutkan. Prioritas adalah gate untuk replacement live; trial UI fixtures dapat berjalan paralel.

## BE-G07 — Guest details dan special request belum sesuai checkout

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/router.go:124](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:124), [internal/booking/booking.go:64](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go:64).

**Gap dan dampak:** Create hanya name/email dan total guests. Summary vendor menyediakan special request 500 karakter, informasi arrival dan konteks hotel. Belum ada phone/contact schema, per-room guest/occupancy, request persistence atau summary read model. Phone adalah kandidat kebutuhan baru yang perlu owner, bukan form vendor yang sudah diverifikasi.

**Tindakan:** Tentukan guest/contact minimal, special request max 500, per-room allocation, arrival optional dan summary endpoint aman. Tandai special requests tidak dijamin. Validasi/persist fields dan batasi PII/log.

**Kriteria verifikasi:** Save-read roundtrip special request Unicode/500+, guest validation, multi-room, optional fields, API tidak silently mengabaikan field yang FE tampilkan.

## BE-G08 — Cancellation policy dan consent tidak ditegakkan

Prioritas: **P0**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service.go:213](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:213), [internal/booking/booking.go:34](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go:34), [migrations/00001_init.sql:31](/mnt/code/projects/jobs/pulang/current-booking/migrations/00001_init.sql:31).

**Gap dan dampak:** Confirmed selalu dapat dibatalkan dan inventory dikembalikan, tanpa policy rate. Offer publik non-cancellable/non-modifiable dan no-show 100%; backend tidak snapshot policy, terms/privacy version atau consent. Mengganti vendor langsung menghasilkan hak pembatalan berbeda dan tidak ada jejak persetujuan checkout.

**Tindakan:** Pisahkan pending abandonment, guest cancellation sesuai rate, staff exception berizin, refund/payment reversal. Simpan policy snapshot dan consent versi/timestamp; terms marketing bukan menggantikan transaksi consent. Final rule dan wording disetujui hotel.

**Kriteria verifikasi:** Nonrefundable confirmed tidak dapat cancelled guest; pending dapat abandoned; refundable window/punishment akurat; staff override auditable; booking dibuat tanpa consent required ditolak. Refund state terpisah dari inventory state.

## BE-G09 — Create belum idempoten dan sulit dipulihkan

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/router.go:145](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:145), [internal/booking/service.go:156](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:156).

**Gap dan dampak:** POST ulang membuat booking/hold baru. Response hilang setelah commit membuat tamu retry dan mengambil stok serta payment attempt lagi. FE disable tombol saja tidak mencegah network retries/double click lintas tab.

**Tindakan:** Persist idempotency key terikat session dan payload hash, return resource yang sama untuk same request, conflict untuk key beda payload. Sediakan recovery booking/payment attempt tanpa menduplikasi stok.

**Kriteria verifikasi:** Concurrent same key→satu booking/hold/charge; timeout after commit lalu retry→same ID; beda payload key sama→409; key isolation antar guest.
