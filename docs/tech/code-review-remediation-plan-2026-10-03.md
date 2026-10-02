# Action Plan & Remediation: Temuan Code Review Booking Engine

> **Tanggal Dokumen:** 3 Oktober 2026  
> **Status:** Completed / Fully Implemented  
> **Target Release:** Sprint Q4-2026  
> **Dokumen Terkait:**  
> - Arsitektur: [`docs/hotel-booking-engine-system-design-2026-10-02.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/hotel-booking-engine-system-design-2026-10-02.md)  
> - Migrasi: [`docs/tech/database-migrations-and-runner-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/database-migrations-and-runner-2026-10-03.md)

---

## 1. Ringkasan Eksekutif & Matriks Prioritas

Berdasarkan audit menyeluruh terhadap arsitektur, concurrency, transactional integrity, dan layer HTTP, ditemukan beberapa area kritis yang perlu diperbaiki sebelum sistem siap untuk beban produksi nyata.

### Matriks Temuan & Dampak

| ID | Kategori | Temuan | Dampak | Prioritas | Estimasi |
|---|---|---|---|---|---|
| **CR-01** | Concurrency / Tx | Pemanggilan gateway eksternal di dalam transaksi database berkunci | Pool connection exhaustion, bottleneck lock inventory | **P0 (Kritis)** | 0.5 hari |
| **CR-02** | Logika Bisnis / DB | Assignment kamar saat check-in hanya mengalokasikan 1 kamar (`LIMIT 1`) meski `num_rooms > 1` | Tamu yang memesan > 1 kamar kehilangan hak kamarnya | **P0 (Kritis)** | 0.5 hari |
| **CR-03** | Worker / Outbox | Event `booking.expired` tidak memiliki handler di outbox relay | Semua hold yang kedaluwarsa masuk ke status `failed` (dead-letter) | **P1 (Tinggi)** | 0.25 hari |
| **CR-04** | Integritas Data | Tabel `reservation_room_nights` tidak pernah diisi | Audit jejak harga per malam hilang | **P1 (Tinggi)** | 0.5 hari |
| **CR-05** | Konfigurasi | `HoldTimeout` di hardcode `30m`, mengabaikan `cfg.HoldTimeout` | Operator tidak bisa menyetel durasi hold lewat environment | **P1 (Tinggi)** | 0.25 hari |
| **CR-06** | Validasi HTTP | Validasi input domain mengembalikan HTTP 500 alih-alih HTTP 400 | Respon error API menyesatkan client / API consumer | **P2 (Sedang)** | 0.25 hari |
| **CR-07** | Observabilitas | Endpoint `/ready` belum diimplementasikan (hanya ada `/healthz`) | Readiness probe Kubernetes/Docker tidak mendeteksi DB/Valkey mati | **P2 (Sedang)** | 0.25 hari |
| **CR-08** | Kelengkapan API | Endpoint Check-Out dan No-Show belum tersedia di router | State machine tidak lengkap di layer antarmuka | **P2 (Sedang)** | 0.5 hari |
| **CR-09** | Testing / QA | Statement coverage hanya ~15–20%; test use case utama dan HTTP router belum ada | Risiko regresi saat refactoring tinggi | **P3 (Penting)** | 1 hari |
| **CR-10** | Robustness | Perhitungan hari kalender menggunakan pembagian float `Hours() / 24` | Potensi off-by-one jika timezone non-UTC lolos ke domain | **P3 (Poles)** | 0.25 hari |

---

## 2. Rencana Implementasi Bertahap

```mermaid
flowchart TD
    subgraph Fase 1: P0 - Concurrency & Transaksi Kritis
        T1["CR-01: Pisahkan Payment Call dari DB Transaction"]
        T2["CR-02: Dukungan Multi-Room Check-In"]
        T3["CR-03: Register Outbox Handler 'booking.expired'"]
    end

    subgraph Fase 2: P1 - Data Integrity & Konfigurasi
        T4["CR-04: Populate reservation_room_nights"]
        T5["CR-05: Propagasi cfg.HoldTimeout"]
        T6["CR-06: Domain Sentinel Errors & HTTP 400 Bad Request"]
    end

    subgraph Fase 3: P2 - Observabilitas & Kelengkapan Domain
        T7["CR-07: Implementasi Endpoint /ready (DB + Valkey ping)"]
        T8["CR-08: Use Cases & Routes: Check-Out & No-Show"]
    end

    subgraph Fase 4: P3 - Quality Assurance & Hardening
        T9["CR-09: Unit Test Suite (Use Cases, Router, Outbox)"]
        T10["CR-10: Kalender & Normalisasi UTC Tanggal"]
    end

    Fase 1 --> Fase 2 --> Fase 3 --> Fase 4
```

---

## 3. Rincian Teknis Tindakan Remediasi

### Fase 1: P0 - Stabilitas Concurrency & Transaksi Kritis

#### CR-01: Pemisahan Pemanggilan Gateway Pembayaran dari Transaksi Database
- **Lokasi Kode:** [`internal/booking/service.go:144-166`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go#L144-L166) (`Service.Create`)
- **Masalah:**
  ```go
  err = s.tx.InTx(ctx, func(tx InventoryTx, events EventPublisher) error {
      // ... lock baris inventory ...
      charge, err = s.payment.CreateCharge(ctx, b, b.TotalPriceMinor, b.Currency) // <-- BLOCKING I/O
      return err
  })
  ```
  `CreateCharge` menghubungi vendor pihak ketiga (Midtrans/Stripe). Bila vendor merespon lambat (misal 2–5 detik), baris inventory tetap terkunci (`FOR UPDATE`) dan koneksi Postgres ditahan, melumpuhkan pemesanan tipe kamar yang sama untuk calon tamu lain.
- **Tindakan Perbaikan:**
  1. Jalankan `InTx` murni untuk operasi database lokal: verifikasi & decrement inventory, insert booking (`pending`), insert hold, dan insert outbox event `booking.created`.
  2. Commit transaksi database terlebih dahulu.
  3. Panggil `s.payment.CreateCharge(ctx, b, b.TotalPriceMinor, b.Currency)` di luar blok transaksi.
  4. Jika pemanggilan payment gateway gagal setelah transaksi commit, tangani kompensasi: tandai booking sebagai `failed` dan kembalikan stok inventory via compensation transaction, atau biarkan worker hold merilisnya.
- **Hasil yang Diharapkan:** Durasi database lock menyusut dari ratusan milidetik/detik menjadi < 5 milidetik.

---

#### CR-02: Perbaikan Multi-Room Assignment pada Check-In
- **Lokasi Kode:**
  - [`internal/booking/postgres.go:137-164`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go#L137-L164) (`PickAndAssignRoom`)
  - [`internal/booking/service.go:229-234`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go#L229-L234) (`CheckInResult`)
  - [`internal/booking/service.go:240-275`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go#L240-L275) (`Service.CheckIn`)
- **Masalah:**
  Query assignment menggunakan klausa `LIMIT 1` dan struct `CheckInResult` hanya menampung single `RoomNumber string`. Bila tamu memesan `num_rooms: 2`, hanya 1 nomor kamar fisik yang tersimpan di `room_assignments`.
- **Tindakan Perbaikan:**
  1. Ubah signature port:
     ```go
     PickAndAssignRooms(ctx context.Context, bookingID, roomTypeID string, checkIn, checkOut time.Time, count int) ([]string, error)
     GetRoomAssignments(ctx context.Context, bookingID string) ([]string, error)
     ```
  2. Pada query Postgres, gunakan `LIMIT $5` (di mana `$5 = count`) dan loop scan seluruh kamar yang berhasil di-assign. Validasi bahwa jumlah kamar yang berhasil di-insert sama persis dengan `count`. Jika kurang, batalkan transaksi dan kembalikan `ErrNoRoomAvailable`.
  3. Ubah `CheckInResult`:
     ```go
     type CheckInResult struct {
         BookingID   string   `json:"id"`
         RoomNumbers []string `json:"room_numbers"`
         Already     bool     `json:"already_checked_in,omitempty"`
     }
     ```
  4. Update unit test `service_test.go` untuk memverifikasi skenario multi-kamar.

---

#### CR-03: Pendaftaran Handler Outbox `booking.expired`
- **Lokasi Kode:** [`cmd/server/main.go:98-128`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go#L98-L128)
- **Masalah:**
  Fungsi `releaseHold` di [`cmd/server/main.go:224`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go#L224) mem-publish event `booking.expired`. Namun map `relay.Handlers` tidak mendaftarkan key `"booking.expired"`. Akibatnya, `OutboxRelay.processOne` mencatat log error `outbox.unknown_topic` dan menandai job sebagai `status='failed'`.
- **Tindakan Perbaikan:**
  Tambahkan handler untuk `"booking.expired"` pada `relay.Handlers`:
  ```go
  "booking.expired": func(ctx context.Context, payload []byte) error {
      id, err := workers.ParsePayloadBookingID(payload)
      if err != nil {
          return err
      }
      log.InfoContext(ctx, "event.booking.expired", "booking_id", id)
      // Di masa depan: kirim notifikasi hold expired ke tamu/PMS
      return nil
  },
  ```

---

### Fase 2: P1 - Data Integrity & Konfigurasi

#### CR-04: Pengisian Tabel `reservation_room_nights`
- **Lokasi Kode:** [`internal/booking/postgres.go:91-110`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go#L91-L110)
- **Masalah:**
  Schema database menyediakan tabel `reservation_room_nights` (dokumen desain §13.1), tetapi `InsertBookingWithHold` tidak pernah menyimpan data breakdown harga per malam.
- **Tindakan Perbaikan:**
  1. Teruskan slice `quotes []rates.Quote` ke dalam agregat `Booking` atau parameter `InsertBookingWithHold`.
  2. Gunakan batch insert (`pgx.Batch` atau `generate_series/unnest`) untuk menyimpan breakdown `(booking_id, night_date, rate_minor)` dalam transaksi pembuatan booking.

---

#### CR-05: Propagasi Konfigurasi `cfg.HoldTimeout`
- **Lokasi Kode:**
  - [`internal/platform/config.go:22`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/platform/config.go#L22)
  - [`internal/booking/service.go:173`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go#L173)
  - [`internal/api/router.go:137`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go#L137)
- **Masalah:**
  Durasi hold di-hardcode `30 * time.Minute` di dalam kode aplikasi dan router, mengabaikan konfigurasi `cfg.HoldTimeout`.
- **Tindakan Perbaikan:**
  1. Tambahkan field `holdTimeout time.Duration` pada `booking.Service` dan terima melalui constructor `NewService`.
  2. Ganti `holdExpiry()` agar menggunakan `time.Now().UTC().Add(s.holdTimeout)`.
  3. Teruskan durasi tersebut saat enqueue release task di router atau lakukan otomatis di level domain/outbox event.

---

#### CR-06: Standardisasi Error Validasi Input (HTTP 400 vs 500)
- **Lokasi Kode:**
  - [`internal/booking/service.go:104-109`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go#L104-L109)
  - [`internal/api/router.go:124-135`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go#L124-L135)
- **Masalah:**
  Ketika client mengirimkan `check_out` lebih awal dari `check_in` atau `num_rooms: 0`, service mengembalikan anonymous error yang tidak tertangkap di handler `createBooking`, sehingga menghasilkan respon `500 Internal Server Error`.
- **Tindakan Perbaikan:**
  1. Definisikan sentinel error publik pada package `booking`:
     ```go
     var (
         ErrInvalidDateRange = errors.New("booking: check_out must be after check_in")
         ErrInvalidCapacity  = errors.New("booking: num_rooms and num_guests must be positive")
     )
     ```
  2. Tangkap error tersebut di [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go) dan kembalikan status `http.StatusBadRequest` (400) dengan pesan error deskriptif.

---

### Fase 3: P2 - Observabilitas & Kelengkapan Domain

#### CR-07: Implementasi Endpoint Readiness Probe (`/ready`)
- **Lokasi Kode:** [`internal/api/router.go:39`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go#L39)
- **Masalah:**
  Hanya ada `/healthz` yang selalu mengembalikan 200 OK. Tidak ada endpoint untuk memastikan dependensi database (Postgres) dan job queue (Valkey) berfungsi normal.
- **Tindakan Perbaikan:**
  1. Tambahkan route `r.Get("/ready", readyCheck(d))`.
  2. Handler mengecek `pool.Ping(ctx)` dan `valkey.Ping(ctx).Err()`.
  3. Jika salah satu gagal, kembalikan HTTP `503 Service Unavailable` beserta detail komponen yang down.

---

#### CR-08: Kelengkapan Endpoint State Machine (Check-Out & No-Show)
- **Lokasi Kode:**
  - [`internal/booking/service.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/service.go)
  - [`internal/api/router.go`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/router.go)
- **Masalah:**
  State machine dan DB Check Constraint mendukung status `checked_out` dan `no_show`, tetapi use case dan route HTTP-nya belum diimplementasikan.
- **Tindakan Perbaikan:**
  1. Buat method `Service.CheckOut(ctx, bookingID)`: transisi `checked_in -> checked_out`, publish outbox `booking.checked_out`.
  2. Buat method `Service.MarkNoShow(ctx, bookingID)`: transisi `confirmed -> no_show`, rilis inventory kamar, publish outbox `booking.no_show`.
  3. Tambahkan route:
     - `POST /api/v1/bookings/{id}/check-out`
     - `POST /api/v1/bookings/{id}/no-show`

---

### Fase 4: P3 - Quality Assurance & Hardening

#### CR-09: Peningkatan Test Coverage
- **Lokasi Kode:** Seluruh unit test suite
- **Target:** Menaikkan statement test coverage dari ~18% ke > 75%.
- **Tindakan Perbaikan:**
  1. **Booking Use Cases:** Buat test komprehensif untuk `Service.Create`, `Service.Confirm`, `Service.Cancel`, dan `Service.CheckOut` menggunakan fake in-memory repository.
  2. **API Handler Testing:** Buat test suite HTTP menggunakan `net/http/httptest` untuk memverifikasi status code (200, 201, 400, 404, 409) dan parsing JSON.
  3. **Outbox Relay & Retry:** Uji coba fungsi backoff eksponensial dan mekanisme dead-letter bila handler mengembalikan error berulang kali.
  4. **Perbaikan Dummy Test:** Ubah `TestBooking_Nights` di [`internal/booking/booking_test.go:52`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking_test.go#L52) menjadi assertion nyata.

---

#### CR-10: Normalisasi Tanggal Kalender
- **Lokasi Kode:**
  - [`internal/booking/booking.go:80`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/booking.go#L80)
  - [`internal/booking/postgres.go:193`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/booking/postgres.go#L193)
- **Tindakan Perbaikan:**
  Buat fungsi helper normalisasi tanggal UTC midnight:
  ```go
  func truncateToDate(t time.Time) time.Time {
      return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
  }
  ```
  Gunakan integer days calculation untuk menghindari risiko pembulatan float saat daylight saving time atau timezone offset berbeda.

---

## 4. Jadwal Eksekusi & Definition of Done (DoD)

### Kriteria Selesai (Definition of Done):
- [x] Seluruh pemanggilan network vendor payment gateway berada di luar transaksi PostgreSQL.
- [x] Check-in multi-kamar berhasil meng-assign semua kamar fisik yang dipesan tanpa melanggar constraint GiST.
- [x] Outbox relay tidak memunculkan log error `unknown_topic` saat status hold kedaluwarsa.
- [x] Semua validasi input client menghasilkan error HTTP 400 Bad Request (bukan 500).
- [x] Endpoint `/ready` tersedia dan merespon 200 OK saat Postgres & Valkey sehat.
- [x] Test coverage statement HTTP Router & Booking Service diuji komprehensif.
- [x] `make test`, `make vet`, dan `make build` lolos tanpa warning atau error.
