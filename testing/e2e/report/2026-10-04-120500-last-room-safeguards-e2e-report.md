# E2E Test Report: Last-Room Operational Safeguards & 1-Click Complimentary Upgrade Resolution Engine

- **Tanggal / Waktu:** 2026-10-04 12:05:00 WIB
- **Target Fitur:** Last-Room Operational Safeguards, LRDA Safety Buffer & 1-Click Complimentary Upgrade Resolution Engine (F12, F13, F14)
- **Komponen Pengujian:**
  - [`migrations/00022_last_room_safeguards_and_upgrade.sql`](file:///mnt/code/projects/jobs/pulang/current-booking/migrations/00022_last_room_safeguards_and_upgrade.sql) (Kolom `safety_buffer` pada tabel `channel_partners`, kolom `upgrade_room_type_id` dan `notes` pada `channel_sync_issues`, serta perluasan hak akses Casbin RBAC untuk `receptionist`, `revenue_mgr`, dan `gm_admin`)
  - [`internal/inventory/availability.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/inventory/availability.go) & [`internal/inventory/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/inventory/postgres.go) (Logika evaluasi ketersediaan stok kamar berpenyangga `CheckWithBuffer` dan kueri PostgreSQL `CheckAvailabilityWithBuffer`)
  - [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go) (Dynamic Last-Room Hold Timeout: penyingkatan durasi booking hold dari 30 menit menjadi 15 menit ketika sisa kamar $\le 1$ untuk mencegah *ghost holds*)
  - [`internal/channel/model.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/channel/model.go), [`internal/channel/store.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/channel/store.go), [`internal/channel/postgres.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/channel/postgres.go), [`internal/channel/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/channel/service.go) (Validasi buffer kanal saat webhook masuk, karantina overbooking otomatis, dan transaksi dua arah atomik `ResolveWithUpgrade`)
  - [`internal/api/http/handler/channel.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/channel.go) (`HandleResolveChannelSyncIssue`, validasi input JSON, pemetaan error HTTP)
  - [`internal/api/http/routes.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go) & [`internal/api/http/testdata/routes.golden`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/testdata/routes.golden) (Pendaftaran endpoint `POST /api/v1/staff/channel-sync-issues/:id/resolve` di bawah proteksi Casbin RBAC)
  - [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) (`E2E-79` dan `E2E-80`)
  - [`testing/e2e/script/last_room_safeguards_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/last_room_safeguards_e2e.sh)
- **Lingkungan Pengujian:**
  - In-Process Go E2E Runner: HTTP Memory Transport (`E2E-01` s.d. `E2E-80`)
  - Ephemeral Live HTTP Server: Port 28094 (`127.0.0.1:28094`) via skrip otomasi Bash
  - Database: PostgreSQL 18 container (`current-booking-postgres-1`) dengan Goose migration versi 22
- **Skrip Eksekusi:** [`testing/e2e/script/last_room_safeguards_e2e.sh`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/last_room_safeguards_e2e.sh)

---

## 1. Ringkasan Eksekusi

| Metrik | Target | Nilai Realisasi | Status |
| :--- | :--- | :--- | :--- |
| **Total Asersi Otomatis Bash** | - | 20 asersi | 100% PASS |
| **Asersi Go In-Process** | - | 2 sub-test (`E2E-79` & `E2E-80`) | 100% PASS |
| **Total Suite Go E2E** | 80 | 80 sub-tests | 100% PASS |
| **Gagal / Error** | 0 | 0 | CLEAN |
| **Statement Coverage (`internal/channel`)** | $\ge 80\%$ | **82.5%** | MEMENUHI SYARAT |
| **Statement Coverage (`internal/inventory`)** | $\ge 80\%$ | **82.4%** | MEMENUHI SYARAT |
| **Data Race Detection (`-race`)** | 0 races | 0 data races | CLEAN |
| **Static Analysis (`go vet ./...`)** | 0 issues | 0 issues | CLEAN |

---

## 2. Rincian Skenario Pengujian

### Skenario 1: Go In-Process E2E Suite (E2E-79 & E2E-80)
- **Komponen Diuji:** [`testing/e2e/script/e2e_runner_test.go`](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go)
- **Eksekusi:**
  - `E2E-79: Dynamic Last-Room Hold Timeout & LRDA Safety Buffer Protection` (`PASS`):
    - Webhook OTA Agoda mengirimkan permintaan booking untuk kamar dengan sisa 1 unit saat `safety_buffer = 1`.
    - Engine langsung menolak dengan HTTP 409 Conflict (`ALLOTMENT_EXHAUSTED`) dan mencatat rekor karantina ke `channel_sync_issues`.
    - Di sisi web direct tamu publik, kamar terakhir tersebut tetap dapat diperoleh penawarannya (`POST /api/v1/quotes` $\rightarrow$ 200 OK).
    - Tamu publik melakukan reservasi langsung (`POST /api/v1/bookings` $\rightarrow$ 201 Created). Engine mendeteksi ketersediaan kamar $\le 1$, sehingga batas kedaluwarsa hold (`expires_at`) dipersingkat menjadi 15 menit (dibandingkan durasi standar 30 menit).
  - `E2E-80: 1-Click Complimentary Upgrade Resolution Engine & RBAC Verification` (`PASS`):
    - Staf Resepsionis membaca daftar isu karantina via `GET /api/v1/staff/channel-sync-issues` $\rightarrow$ HTTP 200 OK.
    - Staf Housekeeping mencoba mengeksekusi resolusi via `POST /api/v1/staff/channel-sync-issues/:id/resolve` $\rightarrow$ Ditolak HTTP 403 Forbidden.
    - Staf Resepsionis mengeksekusi aksi `COMPLIMENTARY_UPGRADE` dengan menyertakan `target_room_type_id` $\rightarrow$ HTTP 200 OK (`status: RESOLVED`).
    - Staf mencoba menyelesaikan kembali isu yang sama $\rightarrow$ Ditolak HTTP 409 Conflict (`ALREADY_RESOLVED`).

---

### Skenario 2: Penolakan Webhook OTA Berdasarkan Safety Buffer (LRDA Protection)
- **Deskripsi:** Mitra OTA (misal: Agoda) mengirimkan event reservasi saat sisa kamar fisik berada di batas penyangga keamanan (`safety_buffer`).
- **Request:**
  ```http
  POST /api/v1/channel-events
  X-Channel-Provider: AGODA
  X-Channel-Signature: c81b3ce818cbb31b637bbbb43315df5dd8b6b0dffbe14b13a7df2b85fa19bc5a
  Content-Type: application/json

  {
    "event_id": "EVT-AGD-OVR-01",
    "event_type": "BOOKING_CREATED",
    "provider_booking_id": "AGD-OVR-999",
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-11-20",
    "check_out": "2026-11-22",
    "rooms_count": 1,
    "guest_name": "Agoda Overbooked Guest",
    "guest_email": "agoda.guest@example.com",
    "guest_phone": "+628119999000"
  }
  ```
- **Respons Sistem:** HTTP 409 Conflict:
  ```json
  {
    "code": "ALLOTMENT_EXHAUSTED",
    "event_id": "EVT-AGD-OVR-01",
    "message": "Kuota kamar penuh untuk rentang tanggal yang diminta. Event telah dikarantina."
  }
  ```
- **Verifikasi Database:**
  - Baris baru tercipta pada `channel_sync_issues` dengan status `QUARANTINED_CONFLICT`.
  - Catatan log insiden memuat informasi lengkap: `provider_code: AGODA`, `external_reference: AGD-OVR-999`, `rooms_count: 1`.

---

### Skenario 3: Front Desk Inspection & Casbin RBAC Least-Privilege Guard
- **Deskripsi:** Verifikasi bahwa front desk staff (`receptionist`) dapat melihat dan menangani isu overbooking, sementara peran tanpa wewenang (`housekeeping` & `guest`) ditolak.
- **Pengujian Meja Depan:**
  - `GET /api/v1/staff/channel-sync-issues?partner=AGODA` oleh `receptionist` $\rightarrow$ HTTP 200 OK.
  - Memverifikasi keberadaan isu dengan `external_reference: AGD-OVR-999`.
- **Pengujian Keamanan RBAC:**
  - `POST /api/v1/staff/channel-sync-issues/:id/resolve` oleh `housekeeping`:
    - HTTP 403 Forbidden:
    ```json
    {
      "code": "FORBIDDEN",
      "error": "Akses ditolak: role Anda tidak memiliki izin untuk resource ini",
      "status": 403
    }
    ```
  - `POST /api/v1/staff/channel-sync-issues/:id/resolve` oleh `guest` (atau unauthenticated) $\rightarrow$ HTTP 401 / 403 Forbidden.

---

### Skenario 4: Validasi Input Resolusi Isu Sinkronisasi
- **Deskripsi:** Pengujian *edge-cases* dan input invalid pada *payload* resolusi.
- **Kasus 1: Upgrade tanpa Target Room Type:**
  - Payload: `{"action": "COMPLIMENTARY_UPGRADE", "notes": "upgrade to suite"}`
  - Respons: HTTP 400 Bad Request:
    ```json
    {
      "code": "MISSING_TARGET_ROOM_TYPE",
      "message": "target_room_type_id wajib diisi untuk aksi COMPLIMENTARY_UPGRADE"
    }
    ```
- **Kasus 2: Aksi Tidak Valid:**
  - Payload: `{"action": "INVALID_ACTION"}`
  - Respons: HTTP 400 Bad Request:
    ```json
    {
      "code": "INVALID_ACTION",
      "message": "Aksi resolusi tidak valid. Pilihan: COMPLIMENTARY_UPGRADE, REJECT_AND_CANCEL, FORCE_OVERBOOK_CONFIRMED"
    }
    ```

---

### Skenario 5: Eksekusi Atomik 1-Click Complimentary Upgrade
- **Deskripsi:** Resepsionis meja depan menyelesaikan konflik overbooking tamu walk-in / arrived dengan meng-upgrade tamu ke tipe kamar yang lebih tinggi tanpa biaya tambahan secara atomik.
- **Request:**
  ```http
  POST /api/v1/staff/channel-sync-issues/c0a80101-0000-7000-8000-000000000001/resolve
  Authorization: Bearer receptionist
  Content-Type: application/json

  {
    "action": "COMPLIMENTARY_UPGRADE",
    "target_room_type_id": "01900000-0000-7000-8000-000000000002",
    "notes": "Complimentary upgrade to Executive Suite due to Agoda last-room contention"
  }
  ```
- **Respons Sistem:** HTTP 200 OK:
  ```json
  {
    "id": "c0a80101-0000-7000-8000-000000000001",
    "status": "RESOLVED",
    "resolution_action": "COMPLIMENTARY_UPGRADE",
    "upgrade_room_type_id": "01900000-0000-7000-8000-000000000002",
    "resolved_by": "01900000-0000-7000-8000-000000000002",
    "notes": "Complimentary upgrade to Executive Suite due to Agoda last-room contention",
    "message": "Isu sinkronisasi berhasil diselesaikan."
  }
  ```
- **Verifikasi Atomisitas Database:**
  - Stok kamar pada tabel inventori untuk tipe kamar target (`01900000-0000-7000-8000-000000000002`) otomatis berkurang sejumlah malam menginap tamu (`ORDER BY date ASC FOR UPDATE`).
  - Rekor `channel_sync_issues` berubah status menjadi `RESOLVED`, mencatat `resolved_at`, `resolved_by`, dan `upgrade_room_type_id`.
- **Pencegahan Double-Resolution (Idempoten / State Protection):**
  - Staf mengulang request yang sama ke isu yang sudah berstatus `RESOLVED`.
  - Respons: HTTP 409 Conflict:
    ```json
    {
      "code": "ALREADY_RESOLVED",
      "message": "Isu sinkronisasi ini sudah diselesaikan sebelumnya"
    }
    ```

---

### Skenario 6: Aksi Alternatif Operasional (Reject & Force Overbook)
- **Kasus 1: REJECT_AND_CANCEL**
  - Digunakan saat hotel benar-benar penuh di seluruh tipe kamar atau tamu OTA memilih pembatalan.
  - Request: `POST .../resolve` dengan aksi `REJECT_AND_CANCEL` dan alasan *no rooms available*.
  - Respons: HTTP 200 OK (`status: REJECTED`). Tidak ada pengurangan inventori kamar fisik.
- **Kasus 2: FORCE_OVERBOOK_CONFIRMED**
  - Digunakan oleh General Manager / Revenue Manager atas diskresi bisnis khusus (misal: tamu VIP).
  - Request: `POST .../resolve` oleh `gm_admin` dengan aksi `FORCE_OVERBOOK_CONFIRMED`.
  - Respons: HTTP 200 OK (`status: OVERBOOKED_OVERRIDDEN`). Tercatat di audit trail staf hotel.

---

## 3. Kesimpulan Verifikasi

Seluruh 20 asersi skenario fungsional dan keamanan berhasil diverifikasi secara otomatis tanpa kegagalan:
1. **LRDA Safety Buffer Terbukti Efektif:** Mitra pihak ketiga (OTA) otomatis ditolak dan diarahkan ke karantina saat sisa stok kamar menyentuh ambang batas penyangga, menjaga kuota kamar tersisa secara eksklusif bagi pemesanan langsung (*direct booking*) ber-margin penuh.
2. **Dynamic Hold Timeout Mencegah Kamar Mati:** Pengurangan otomatis durasi penahanan kamar dari 30 menit menjadi 15 menit ketika sisa kamar $\le 1$ meminimalkan risiko *ghost holds* saat *peak season*.
3. **Resolusi Atomik Tanpa Resiko Human-Error:** Fitur 1-click complimentary upgrade mengunci baris inventori kamar target dengan `FOR UPDATE`, memotong stok secara atomik bersamaan dengan pembaruan status isu, menghilangkan risiko salah hitung atau *double booking* manual di meja depan.
4. **Keamanan RBAC Matang:** Matriks Casbin RBAC memastikan staf meja depan memiliki otonomi cepat menyelesaikan krisis overbooking tamu di lobi, sementara staf housekeeping dan publik diblokir secara mutlak.
