# Security dan kontrak API

Audit: 3 Oktober 2026. Evidence source-level kecuali disebutkan. Prioritas adalah gate untuk replacement live; trial UI fixtures dapat berjalan paralel.

## BE-G13 — Read booking mengungkap PII tanpa bukti kepemilikan

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/router.go:56](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:56), [internal/api/router.go:180](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:180), [internal/booking/booking.go:64](/mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go:64).

**Gap dan dampak:** GET by UUID public mengembalikan name/email dan detail stay. Casbin baru membatasi route berdasarkan role, tetapi policy guest masih mengizinkan GET/cancel semua ID tanpa ownership. Tidak ada verified guest session/access token. UUID tidak sama dengan authorization; siapa pun yang memperoleh ID dari URL/log/response dapat membaca booking. Tidak ada booking ownership restriction saat cancel.

**Tindakan:** Guest session atau scoped high-entropy access token dengan expiry dan revocation; public status DTO minim PII, private summary harus authorize owner. Jangan menaruh PII/access token di query analytics/log.

**Kriteria verifikasi:** Guest A tidak dapat read/cancel B walau tahu UUID; invalid/expired token ditolak; response dan cache headers tidak bocor PII; staff akses per role.

## BE-G14 — Identitas RBAC dapat dipalsukan dan enforcer fail-open

Prioritas: **P0**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/middleware.go:41](/mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go:41), [internal/api/middleware.go:82](/mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go:82), [cmd/server/main.go:177](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:177), [migrasi policy](/mnt/code/projects/jobs/pulang/current-booking/migrations/00003_casbin_rbac.sql:47), [internal/api/router.go:38](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:38), [internal/api/router.go:57](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:57), [internal/api/router.go:216](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:216).

**Gap dan dampak:** Pada pengecekan akhir, routes sudah memakai IdentifySubject/Authorize Casbin. Namun IdentifySubject mengambil X-User-Role/X-User-ID langsung dari caller, atau memakai literal Bearer token sebagai role tanpa memverifikasi signature/session. Caller dapat mengirim X-User-Role: receptionist atau Bearer gm_admin untuk melewati RBAC. Selain itu main hanya log error saat NewEnforcer gagal, lalu router melewati Authorize bila enforcer nil: fail-open. Query staff_users/is_active tidak digunakan untuk autentikasi. Proxy yang memverifikasi/menghapus header belum tampak; deploy public tidak diasumsikan sudah terjadi.

**Tindakan:** Fail-closed saat enforcer gagal. Verifikasi identitas/signature/session server-side, derive role dari identity yang trusted; header role dari public client harus diabaikan. Pisahkan namespace staff/guest; authentication dan role permission untuk mutate operational states, policy exception dan refund. Tambah actor/reason/audit; jangan expose operations di public booking FE.

**Kriteria verifikasi:** Forged role header/Bearer gm_admin tidak memberikan akses; enforcer gagal tidak melayani API protected; inactive staff ditolak. Unauthorized→401, wrong role→403, cross-resource deny, staff action audit, date constraints: tidak check-in future/no-show sebelum batas yang disetujui hotel.

## BE-G15 — API publik belum membatasi abuse dan kontrak error

Prioritas: **P1**. Status: OPEN pada snapshot source audit; remediasi belum dijalankan.

**Bukti:** [internal/api/middleware.go:41](/mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go:41), [internal/api/middleware.go:82](/mnt/code/projects/jobs/pulang/current-booking/internal/api/middleware.go:82), [cmd/server/main.go:177](/mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go:177), [migrasi policy](/mnt/code/projects/jobs/pulang/current-booking/migrations/00003_casbin_rbac.sql:47), [internal/api/router.go:38](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:38), [internal/api/router.go:134](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:134), [internal/api/router.go:299](/mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go:299).

**Gap dan dampak:** Tidak ada request rate limit/body size/strict JSON/field errors. Decode unknown fields mengabaikan rate_plan atau special_request; FE dapat mengira pilihan tersimpan. Search rentang besar membuat quote slice besar karena tidak punya batas dan rates mengabaikan context. Error berbentuk string campuran; rate error seluruhnya 404. CORS/BFF/session policy belum diputuskan.

**Tindakan:** Rate limit search/create/status, bounded body/LOS/room counts, field validation dan reject unknown fields; machine-readable codes/field map/request ID. Pilih same-origin Nuxt BFF atau explicit CORS allowlist. CSRF perlu bila memakai cookie auth, bukan dianggap sudah terbukti vulnerability pada API sekarang.

**Kriteria verifikasi:** Oversized payload 413, invalid fields 400/422, limiter 429+retry guidance, DB outage 503/internal bukan unknown room; browser contract end-to-end dengan session/CORS pilihan final.
