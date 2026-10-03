# Cakupan roadmap dan verification gates untuk frontend

Tanggal: 3 Oktober 2026. Status: evidence mapping dan usulan acceptance, bukan deklarasi implementasi/deployment selesai.

## API yang terdaftar pada snapshot

Registrasi utama: [router.go](../../internal/api/router.go), NewRouter. Health/ready dan webhook berada di luar grup Casbin. Guest challenge/verify publik, guest me/logout/list/detail memakai requireGuestSession. Katalog dan booking memakai IdentifySubject + Authorize.

| Area | Method/path aktual | Keterangan kontrak |
|---|---|---|
| Katalog | GET/POST `/api/v1/catalog/rooms`; GET/PUT/DELETE `/api/v1/catalog/rooms/{id}` | CRUD ada; harga katalog tidak otomatis menjadi tarif engine |
| Pencarian | GET `/api/v1/search`; GET `/api/v1/availability` | check_in/check_out, rooms/adults/children/child_ages; search price belum total paket/pajak |
| Quote | POST `/api/v1/quotes` | satu room_type_id/rate_plan, num_rooms/num_guests; TTL15m, IDR exponent0 |
| Booking | POST `/api/v1/bookings`; GET `/api/v1/bookings/{id}` | flat guest fields, quote_id, terms/privacy, X-Guest-Token untuk read privat |
| Operasi booking | POST `/api/v1/bookings/{id}/cancel`, `/check-in`, `/check-out`, `/no-show` | Token ownership guest cancel; operasi staf bergantung trusted identity yang masih bermasalah |
| Payment | POST `/api/v1/webhooks/xendit`; POST `/fake-pay/{ref}` hanya development | Redirect FE bukan confirmation; dev payment bukan live checkout |
| Guest auth | POST `/api/v1/auth/guest/challenge`, `/verify`; GET `/me`; POST `/logout` | prefix me/logout tetap `/api/v1/auth/guest`; OTP dan hashed session persistence tersedia |
| Booking Saya | GET `/api/v1/guest/bookings`; GET `/api/v1/guest/bookings/{id}` | Session ownership berdasarkan canonical email; koleksi data/total dibatasi limit, belum cursor |
| Artifacts | GET `/api/v1/guest/bookings/{id}/receipt`; GET `/api/v1/guest/bookings/{id}/calendar.ics` | Session-protected receipt JSON dan file ICS dengan status guard tersedia |

Berbeda dengan kondisi awal pembacaan, route guest auth/list/detail/receipt/calendar dan wiring GuestSvc sudah ada pada snapshot akhir. Jangan mempertahankan blocker “login endpoint tidak ada” ketika memakai source ini. Service localhost yang diprobe belum menunjukkan katalog baru; rebuild/migrate/deploy serta environment provider tetap perlu verifikasi terpisah.

## Matriks F01–F15

“Partial” mengakui implementasi yang ada tanpa menyamakan kapabilitas pendukung dengan semua FR. Tidak adanya route berarti kontrak HTTP itu belum ditemukan dalam router snapshot, bukan bukti tidak pernah dikerjakan pada checkout/branch lain. Source terus berubah ketika audit; tambahan setelah waktu manifest harus dinilai lewat delta review, bukan diasumsikan belum ada.

| Fitur | Implementasi yang ditemukan | Sisa kontrak/gate untuk FE |
|---|---|---|
| F01 journey | Search, quote, create/status/cancel | Satu tipe/paket per booking; mixed cart atomic belum ada. Idempotency/capacity/consent diperbaiki sesuai BE-R06–R11 |
| F02 guest access | OTP challenge/verify, hashed sessions, me/logout | OTP atomicity, delivery, session revocation/cookie hardening (BE-R03/R04/R17) |
| F03 Booking Saya | Email-owned list/detail dan allowed_actions | Canonical email, pagination, policy-derived actions, invoice recovery (BE-R05/R15) |
| F04 artifacts | Email confirmed, private receipt JSON dan kalender ICS tersedia | Rendering/export PDF tidak ditemukan dalam router snapshot; timezone/metadata/policy artifacts perlu acceptance. Receipt JSON tidak otomatis berarti PDF atau invoice pajak telah diterbitkan |
| F05 payment recovery | Provider create/callback, ledger, hold/expiry | Resume/lookup/reconcile contract dan ambiguous outcome belum lengkap (BE-R13–R16) |
| F06 requests | Cancellation use case ada | Request bantuan/perubahan, approval dan history belum ditemukan; cancel langsung berbeda dari request/refund workflow |
| F07 front desk | Check-in/out/no-show, assignment | Daftar/filter reservation kerja staf, trusted session, audit/version conflict perlu kontrak. Jangan membuat login role-string |
| F08 inventory | Katalog CRUD, inventory read/decrement/horizon | Mutasi maintenance/block/stock adjustment dengan audit dan concurrency contract belum ditemukan |
| F09 revenue | Dua plan dan promo OCTOBREAK dihitung engine | CRUD/version rate/promo, validity/quota dan ownership konfigurasi belum ditemukan; map hardcoded bukan management API |
| F10 channel | Inventory lokal dan outbox dapat menjadi fondasi | Adapter channel, import/export mapping, cursor/version replay, reconciliation dan stop-sell API belum ditemukan |
| F11 notification | Resend confirmed dan OTP/log adapters | Delivery ledger/status/retry admin dan event lengkap belum ditemukan; logged/accepted tidak berarti delivered |
| F12 staff identity | Casbin model/policy adapter tersedia | Trusted login/session/MFA/role administration/audit lifecycle belum ditemukan; impersonation BE-R01 tetap gate |
| F13 hotel config | Metadata/default config/description di source | Read/write/version config API dan immutable policy snapshots untuk seluruh kebutuhan belum ditemukan |
| F14 finance | PaymentAttempt repository serta tambahan model/store finance dan adapter CreateRefund terlihat | Jalur HTTP finance, approval/execution/reconciliation dan guest refund recovery belum terdaftar dalam router snapshot; module baru belum mendapat audit mendalam pada review ini |
| F15 pilot | Unit/HTTP harness, real-DB suite/report | Deployment-current contract smoke, provider sandbox, browser/device/accessibility, rollback/backup/pilot acceptance masih perlu bukti |

Rujukan requirement per fitur tersedia pada [SRS register](../srs/README.md), [PRD register](../prd/README.md), dan [TECH register](../tech/README.md). Detail F01–F15 roadmap masih harus dibaca bersama owner-approved Batch A–F/guest/payment docs; proposal tidak otomatis mengganti kontrak aktif. Conflict register [shared contracts](../srs/00-shared-contracts-and-decisions-2026-10-03.md) tetap menjadi tempat keputusan product/owner, bukan laporan gap ini.

## Batas bukti laporan pengujian

Laporan [Batch F](../../testing/e2e/report/2026-10-03-real-db-concurrency-batch-f-e2e-report.md) memuat lima skenario PostgreSQL: last-room race, multi-night rollback, parallel assignment, GiST overlap dan late-payment/expiry. Source suite memang mempunyai pgx connection dan SQL assertions. Review ini tidak mengulang suite atau meniadakan bukti historis tersebut. Lima skenario itu tidak menguji atomic checkout idempotency, OTP races, staff-token validity atau semua provider recovery.

Laporan [Xendit/Resend](../../testing/e2e/report/2026-10-03-xendit-resend-e2e-report.md) harus dibaca sesuai harness-nya. [e2e_runner_test.go](../../testing/e2e/script/e2e_runner_test.go) memakai e2eTxMock/e2ePayMock/e2eReaderMock; skenario Resend membentuk httptest.NewServer lalu memanggil SendBookingConfirmed secara langsung. Ini memberi bukti HTTP payload/adapter dan handler logic, belum membuktikan pengiriman email nyata atau outbox PostgreSQL→worker→provider secara penuh. Header callback valid pada mock booking juga belum menguji ledger amount/reference matching.

[integration/db_helper.go](../../testing/integration/db_helper.go) memakai default alamat container tetap, dapat t.Skip ketika DB tidak bisa dijangkau, dan ResetTestData melakukan TRUNCATE CASCADE. [guest/postgres_test.go](../../internal/guest/postgres_test.go) juga memiliki DSN default dan skip-on-ping. Karena itu jangan menjalankan suite terhadap DB pengguna tanpa isolasi, serta jangan menyebut command exit0 sebagai bukti real-DB PASS jika skenario SKIP. Log kegagalan helper juga perlu meredaksi DSN, bukan mencetak kredensial koneksi.

Pernyataan compliance/certification menyeluruh pada laporan terdahulu tidak divalidasi oleh review ini. Sumber ini memberikan evidence rekayasa terbatas; tidak ada penilaian hukum atau sertifikasi baru.

## Urutan penutupan dan handoff

1. **BE/security:** tutup BE-R01 dan BE-R16 sebelum exposure live. Uji kredensial palsu dan production misconfiguration memakai router/wiring deployment yang sama. FE dapat membangun layout staf tetapi tidak menjadikan string role sebagai login.
2. **BE/checkout:** BE-R06–R12. Bekukan occupancy semantics, scalar/mixed room boundary, Money exponent0, raw-body retry contract, durable quote, consent dan deadline. QA membuktikan DB/provider side effects, bukan response saja.
3. **BE/guest:** BE-R02–R05/R17. Verify OTP atomic, revoke session durable, email ownership dan action eligibility. FE login/Booking Saya mengikuti API guest terbaru; BFF menyimpan token HttpOnly, tidak menerima identity email dari browser untuk private lookup.
4. **BE/payment:** BE-R13–R15. Siapkan payment view yang dapat dipulihkan, event/amount matching, ambiguity dan refund exception. FE hanya navigasi ke URL server yang tervalidasi dan mengambil status server setelah redirect.
5. **QA/ops:** verifikasi instance/port/build/migration aktual, isolated real-DB fault/concurrency suite, provider sandbox dan browser flow. Rekam command, SHA/hash, environment, PASS/FAIL/SKIP per skenario, expected/actual DB effects, cleanup serta artefak hasil.

Untuk semua temuan, status CLOSED membutuhkan source fix yang sesuai, contract update bila perlu, dan evidence acceptance pada snapshot yang sama. Jangan menutup hanya karena dokumen approved, test mocked hijau, atau UI tombol sudah tersedia.

## Data handoff minimal

Backend menyediakan base URL lingkungan uji, build/snapshot identity, migration version, provider mode, contoh DTO success/error, auth/session transport, ownership, allowed_actions, server deadline, idempotency window dan capability yang aktif. Frontend menyediakan adapter contract, state/error mapping, retry key/body, session refresh/logout, polling/visibility behavior dan empty/expired paths. QA memisahkan mock contract tests, connected backend smoke, provider sandbox dan physical-device results.

Dalam review ini hanya source/doc inspection serta probe read-only dilakukan. Tidak ada unit/E2E suite yang dijalankan ulang, DB writes, provider charges, email sends, migration ataupun restart service. Frontend implementasi/integrasi tidak diklaim selesai oleh dokumen ini.
