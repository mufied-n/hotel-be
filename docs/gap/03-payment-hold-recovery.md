# Payment, hold dan recovery

Audit: 3 Oktober 2026. Evidence source-level kecuali disebutkan. Prioritas adalah gate untuk replacement live; trial UI fixtures dapat berjalan paralel.

## BE-G10 — Pembayaran masih fake dan route dev dapat mengonfirmasi tanpa bayar

Prioritas: **P0**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [cmd/server/main.go:64](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:64), [cmd/server/main.go:201](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:201), [internal/api/router.go:62](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:62), [internal/adapter/payment/fake.go:29](/mnt/code/projects/jobs/pulang/current-booking/internal/adapter/payment/fake.go:29).

**Gap dan dampak:** Main selalu memasang FakeGateway dan /fake-pay/{ref}. Handler mengonfirmasi booking_id dari query, tidak memverifikasi ref, amount atau pembayaran. Payment URL fake bahkan tidak menyertakan booking_id yang handler wajibkan, serta membuka URL browser GET tidak cocok dengan route POST. Ini dev stub, bukan hosted payment yang bisa dipakai tamu. Jika diekspos, caller dapat confirmed tanpa bayar.

**Tindakan:** Environment gate fail-closed: fake routes hanya development/test; produksi menolak startup tanpa provider nyata. Buat hosted payment adapter, webhook terverifikasi signature/event/amount/currency/reference; jangan menempatkan card fields di app.

**Kriteria verifikasi:** Production route fake-pay tidak ada; invalid signature/amount/currency/reference ditolak; sandbox pay success/failure/duplicate diverifikasi via webhook; URL yang dikembalikan bisa dipakai sesuai metode kontrak provider.

## BE-G11 — Payment attempt, status, refund dan rekonsiliasi tidak persisten

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service.go:43](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:43), [internal/booking/service.go:179](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:179), [internal/booking/service.go:192](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:192), [internal/booking/booking.go:104](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go:104).

**Gap dan dampak:** Charge ref/url hanya response; schema tidak punya payment attempts/events. Confirm hanya ID, duplikat no-op hanya saat confirmed; setelah checked_in webhook ulang gagal illegal transition. StatusFailed ada di enum tetapi tidak punya flow pembayaran gagal. Tidak ada refund/reconciliation/late-payment ledger.

**Tindakan:** Persist attempt idempotency/provider refs/amount/currency/deadline/status dan deduplicate webhook event ID di transaksi. Return status check dari backend; dukung events gagal/expired/reversed serta refund manual yang auditable, bukan mengasumsikan redirect sukses.

**Kriteria verifikasi:** Replay sebelum/sesudah check-in tetap ack sesuai provider tanpa transisi ilegal; partial/failure/late success tidak salah confirmed; reconciliation pulihkan timeout; event audit ledger lengkap.

## BE-G12 — Hold deadline belum menjadi otoritas server dan recovery belum andal

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service.go:104](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:104), [internal/booking/service.go:180](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:180), [internal/booking/service.go:192](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:192), [internal/api/router.go:170](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:170), [cmd/server/main.go:238](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:238).

**Gap dan dampak:** Hold expiry tidak masuk Booking/response; Confirm tidak membaca expires_at. Booking pending yang TTL lewat dapat confirmed sebelum sweep. releaseHold tidak mengecek deadline sendiri. Scheduled delay dihitung setelah CreateCharge, berbeda dari expires_at DB; enqueue error dibuang. Kompensasi Cancel memakai context request yang mungkin expired dan error dibuang. Sweep memang fallback sehingga jangan klaim stok tertahan selamanya.

**Tindakan:** Deadline DB wajib dicek dalam transaksi confirm/release; response includes expires_at/server_time. Jadwalkan berdasarkan absolute expiry melalui jalur durable, log/retry enqueue. Payment ambiguous timeout jangan langsung cancel tanpa reconcile; kompensasi pakai bounded recovery context/job terpantau.

**Kriteria verifikasi:** Fake clock + real DB race confirm/expiry; early release no-op; late payment needs assistance/refund; request cancelled setelah commit tetap dapat recover; worker delay/queue down akhirnya release sekali, inventory tidak over-increment.
