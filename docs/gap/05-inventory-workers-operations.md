# Inventory, workers dan operasional

Audit: 3 Oktober 2026. Evidence source-level kecuali disebutkan. Prioritas adalah gate untuk replacement live; trial UI fixtures dapat berjalan paralel.

## BE-G16 — Notification dan outbox belum siap pengiriman nyata

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/adapter/notifier/log.go:22](/mnt/code/projects/jobs/pulang/current-booking/internal/adapter/notifier/log.go:22), [internal/workers/outbox.go:65](/mnt/code/projects/jobs/pulang/current-booking/internal/workers/outbox.go:65), [internal/workers/outbox.go:80](/mnt/code/projects/jobs/pulang/current-booking/internal/workers/outbox.go:80), [cmd/server/main.go:75](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:75).

**Gap dan dampak:** Email hanya structured log termasuk PII; event PMS/desk hanya log. Outbox memanggil external handler sambil menahan transaction, tidak punya effect idempotency. Success email lalu crash/commit failure menyebabkan replay. Retry/dead-letter UPDATE/Commit errors dibuang; query error dianggap empty queue. Ini durability/recovery gap, bukan kehilangan email nyata yang sudah terbukti.

**Tindakan:** Real notifier dengan idempotency key/event delivery record, bounded timeout, retry dan delivery status; claim/lease strategy sebelum external call bila dipilih. Tangani semua DB error; dead-letter replay terkontrol; redact PII dan retention. Tambah desk/PMS handler sesuai scope.

**Kriteria verifikasi:** Provider timeout→retry; send success + mark-done fail→tidak duplicate effect; retry commit failure terlihat/log/metric; malformed payload dead-letter; email sandbox diterima dan desk workflow terverifikasi.

## BE-G17 — Assignment paralel bisa gagal walau kamar lain bebas

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/postgres.go:149](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go:149), [internal/booking/postgres.go:168](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go:168), [internal/api/router.go:228](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:228).

**Gap dan dampak:** Dua booking memilih free room terendah tanpa row lock/retry. GiST melindungi overlap, tetapi salah satu transaksi dapat gagal meski room lain tersedia. SQLSTATE 23P01 dipetakan hanya di Query error, bukan rows.Err, sehingga konflik deferred menghasilkan 500. Ini false allocation failure, bukan double assignment lolos DB.

**Tindakan:** Strategi kandidat terkunci/serialized per type atau bounded whole-tx retry yang direview; map PG conflict dari semua jalur termasuk rows.Err/commit. Jaga all-or-nothing multi-room.

**Kriteria verifikasi:** Real PG parallel check-in dua booking/dua room sukses; satu room konflik menghasilkan controlled 409 tanpa partial assign; cukup sebagian dari count→rollback; replay kembali rooms yang sama.

## BE-G18 — Sumber inventory kanal lain, maintenance dan horizon belum ada

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [migrations/00002_seed.sql:9](/mnt/code/projects/jobs/pulang/current-booking/migrations/00002_seed.sql:9), [internal/inventory/postgres.go:54](/mnt/code/projects/jobs/pulang/current-booking/internal/inventory/postgres.go:54), [cmd/server/main.go:96](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:96).

**Gap dan dampak:** Inventory 365 hari sekali seed; tidak ada rolling provisioning, catalog/rates/inventory admin API, room block atau PMS/channel manager input. Lock lokal hanya melindungi transaksi engine ini, bukan OTA/PMS yang menjual kamar sama. Saat horizon seed lewat, search berhenti tersedia.

**Tindakan:** Tentukan authority inventory hotel, direct allocation vs channel manager sync, reconciliation/idempotent external updates dan maintenance availability. Provision tanggal secara operasional. Jika integrasi ditunda, trial harus berlabel sandbox dan tidak menjual stok live bersama kanal lain.

**Kriteria verifikasi:** Uji offline/manual sell, channel update replay/out-of-order, maintenance, horizon roll; stok source of truth reconcile; hotel menyetujui alokasi sebelum switch vendor.

## BE-G21 — Sweep dan resource configuration belum sepenuhnya observable

Prioritas: **P2**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/workers/workers.go:106](/mnt/code/projects/jobs/pulang/current-booking/internal/workers/workers.go:106), [internal/platform/config.go:38](/mnt/code/projects/jobs/pulang/current-booking/internal/platform/config.go:38), [internal/platform/config.go:61](/mnt/code/projects/jobs/pulang/current-booking/internal/platform/config.go:61), [cmd/server/main.go:217](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:217).

**Gap dan dampak:** Sweep tidak memeriksa rows.Err setelah iterasi. NewDB tidak Close pool saat Ping error. Duration config menerima zero/negative OutboxInterval yang menyebabkan NewTicker panic; config invalid fallback diam. Shutdown tidak mengoordinasi asynq/relay sampai selesai; listen failure hanya log lalu main tetap menunggu signal.

**Tindakan:** Tangani terminal row error, cleanup gagal-init, validasi config fail-fast, bounded shutdown worker/relay sebelum pool closed, readiness untuk worker fatal dan startup HTTP failure. Pool memakai default pgx/DSN; jangan klaim unlimited tanpa bukti.

**Kriteria verifikasi:** Failure injection sweep/DB init; invalid duration reject; worker/listen failures ditangani; shutdown dengan in-flight outbox tidak memalsukan success; tune pool dari load evidence.

## BE-G22 — Early checkout, no-show dan continuous-room stay belum punya policy

Prioritas: **P2**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/booking/service.go:240](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:240), [internal/booking/service.go:261](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go:261), [internal/booking/postgres.go:155](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go:155).

**Gap dan dampak:** Checkout hanya status, assignment range asli tetap. Ini benar untuk checkout normal half-open, tetapi early checkout resale tidak didukung. No-show release semua tanggal termasuk lampau tanpa batas waktu. Stok fungible per malam juga tidak menjamin satu physical room continuous: A dipakai malam 1, B malam 2, masing-masing malam punya stok 1 tetapi tidak ada room kosong dua malam.

**Tindakan:** Owner menetapkan early checkout/resell, no-show cutoff/penalty, room reassignment/split-stay dan housekeeping turnaround. Implementasi state/inventory/assignment konsisten setelah keputusan; jangan patch checkout normal dengan increment semua malam.

**Kriteria verifikasi:** Test normal back-to-back stays, early checkout remaining nights, no-show sebelum/after cutoff, fragmented assignment dan ops escalation. Jangan menjual guarantee room continuous tanpa alokasi/reassignment yang valid.
