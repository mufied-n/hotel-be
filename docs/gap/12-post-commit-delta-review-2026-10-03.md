# Delta review backend setelah commit operasional

Tanggal review: 3 Oktober 2026. Snapshot source: `09bc727515fdfc7ce8b4e7244f780559d93d2dc9` (`master`). Review ini membandingkan commit F02/F03, F04, F14, housekeeping, front desk, dan stay modification dengan audit sebelumnya. Dokumen ini mencatat implementasi yang ditemukan, batas integrasi frontend, serta gap yang masih terbuka. Review tidak mengubah kode backend, schema, atau status approval produk.

## Ringkasan delta

| Commit | Kapabilitas yang ditemukan | Status untuk frontend |
|---|---|---|
| `4b7355d` | OTP guest, session, profil, Booking Saya | Adapter FE dan BFF dapat memakai kontrak source; hardening sesi dan OTP tetap mengikuti BE-R03–BE-R05 |
| `7d4b8af` | Receipt privat dan kalender RFC 5545 | Dapat diintegrasikan melalui guest session |
| `2372fc9` | Refund, payment cases, reconciliation, guest refund status | Guest refund read dapat diintegrasikan; seluruh UI staff tetap diblokir trusted staff identity |
| `b7b348c` | Housekeeping room board, transisi status, out-of-order, readiness check-in | UI, DTO, fixture, dan contract adapter dapat dibuat; mutation live belum aman |
| `5bbdb96` | Daily roster dan shift handover | UI, DTO, fixture, dan contract adapter dapat dibuat; live PII dan mutation belum aman |
| `09bc727` | Room move, extend stay, dan room-move history | UI, DTO, fixture, dan contract adapter dapat dibuat; mutation live belum aman |

Route baru terdaftar pada `internal/api/router.go`: finance (`/api/v1/finance/*`), guest refund status, housekeeping (`/api/v1/housekeeping/*`), front desk (`/api/v1/front-desk/*`), dan stay modification (`/api/v1/bookings/{id}/room-move`, `/extend-stay`, `/room-moves`). Migrasi `00009` sampai `00012` menambahkan ledger finance, status kamar, handover notes, room move logs, serta policy Casbin terkait.

## Gate yang masih memblokir integrasi staff live

BE-R01 tetap OPEN dan sekarang berdampak pada semua route finance, housekeeping, front desk, dan stay modification. `IdentifySubject` masih menerima nama role langsung sebagai Bearer token. Cabang `X-User-Role` juga memakai kondisi `rawRole != ""`, sehingga setiap role staff yang valid diterima tanpa secret internal. Casbin memeriksa role setelah role tersebut sudah dapat dipalsukan; policy yang lengkap tidak membentuk autentikasi.

Skenario reproduksi source: caller tanpa akun mengirim `Authorization: Bearer gm_admin`, `Authorization: Bearer finance`, atau `X-User-Role: receptionist`. Middleware membentuk principal staff dan route dapat melewati policy sesuai role. Karena roster, booking detail, handover, refund, status kamar, room move, dan extend-stay memuat PII atau side effect operasional/finansial, frontend tidak boleh memakai mekanisme role-string tersebut sebagai login.

Kriteria penutupan: staff login/session atau token bertanda tangan diverifikasi issuer, audience, expiry, revocation, role, dan property scope; header publik dibersihkan pada trust boundary; seluruh credential palsu ditolak pada middleware deployment yang sama; audit actor berasal dari principal server.

## Deployment drift

Instance lokal yang teramati di `http://127.0.0.1:18080` menjawab `200` untuk `/healthz`, tetapi mengembalikan `404` untuk `/api/v1/auth/guest/me`, `/api/v1/finance/reconciliations`, `/api/v1/housekeeping/rooms`, `/api/v1/front-desk/daily-roster`, dan `/api/v1/bookings/demo/room-moves`. Ini menunjukkan proses yang hidup belum memakai snapshot source terbaru atau migrasi/wiring terbaru belum diterapkan.

Frontend belum dapat menjalankan connected smoke terhadap fitur baru sampai backend dibangun ulang, migrasi `00008`–`00012` diterapkan pada database target, proses direstart, dan endpoint identity/build atau deployment record memastikan SHA yang sedang berjalan.

## Verification gap: suite database tidak terisolasi saat paralel

`go vet ./...` lulus. `go test -race ./...` gagal ketika paket database berjalan paralel: finance ownership, stay extension, last-room race, dan rollback test saling melihat atau membersihkan fixture pada database bersama. Setiap skenario yang sama lulus saat dijalankan terisolasi, dan `go test -race -p 1 ./...` lulus seluruh paket.

Implikasinya, perilaku fitur yang diuji tidak terbukti gagal secara deterministik, tetapi perintah verifikasi standar repository belum stabil pada concurrency default. Helper integration memakai database bersama dan operasi reset/truncate lintas paket, sehingga paket dapat saling mengganggu. CI perlu database/schema unik per package atau serialisasi eksplisit yang terdokumentasi. Kriteria penerimaan: tiga kali `go test -race ./...` pada lingkungan bersih lulus tanpa mengandalkan `-p 1`, atau workflow resmi menetapkan isolasi/serialisasi yang mencegah collision dan menjelaskan trade-off.

## Pekerjaan frontend yang sekarang dapat dimulai

1. Guest refund status dapat disambungkan ke halaman detail Booking Saya melalui BFF guest session.
2. Housekeeping board dapat dibuat dengan filter floor/status/type, summary cards, status transition dialog, dan out-of-order form dalam mock/contract mode.
3. Front desk workspace dapat dibuat untuk daily roster, arrivals, departures, occupancy metrics, handover history, dan handover form dalam mock/contract mode.
4. Stay operations dapat dibuat untuk room move, extend stay, conflict/error mapping, dan history dalam mock/contract mode.
5. Finance workspace dapat dibuat untuk reconciliation, cases, resolve flow, dan refund form dalam mock/contract mode.

Semua staff request live tetap dinonaktifkan sampai trusted staff identity tersedia. Connected guest flow juga menunggu deployment instance yang sesuai snapshot source.

## Verifikasi yang dijalankan pada review ini

- `go vet ./...`: PASS.
- `go test -race ./...`: FAIL karena collision database lintas package pada concurrency default.
- Targeted rerun untuk finance, stay, dan dua integration scenario yang gagal: PASS.
- `go test -race -p 1 ./...`: PASS seluruh package.
- Probe read-only instance `127.0.0.1:18080`: health PASS, route fitur baru 404.

Hasil di atas adalah bukti lokal pada snapshot dan environment review ini. Tidak ada provider charge, email nyata, migration execution, atau deployment/restart yang dilakukan.
