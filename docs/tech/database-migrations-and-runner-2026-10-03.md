# Database Migrations & Runner Technical Documentation

> **Tanggal:** 3 Oktober 2026  
> **Status:** Active / Production-Ready  
> **Target Database:** PostgreSQL 18  
> **Tooling:** Goose v3 (`github.com/pressly/goose/v3`) & pgx/v5 (`github.com/jackc/pgx/v5`)  
> **Lokasi File:** `migrations/` & `cmd/migrate/`

---

## 1. Latar Belakang & Motivasi Arsitektur

Pada iterasi awal sistem booking, migrasi database di-embed langsung ke dalam binary server Go menggunakan `//go:embed` dan dijalankan secara otomatis saat server HTTP melakukan *bootstrapping*.

Meskipun sederhana, pendekatan embedded migration memiliki beberapa kelemahan mendasar:
1. **Risiko Race Condition pada Multi-Instance / Scaled App:** Jika lebih dari satu instance aplikasi menyala secara bersamaan di balik load balancer, instance-instance tersebut akan berebut mengunci tabel migrasi dan menjalankan DDL yang berpotensi menyebabkan *lock contention*, timeout, atau kegagalan startup.
2. **Pelanggaran Prinsip Least Privilege:** Binary server yang bertugas melayani trafik HTTP/API membutuhkan hak akses DDL penuh (`CREATE TABLE`, `ALTER TABLE`, `DROP TABLE`). Dalam standar keamanan modern, user database untuk web app sebaiknya hanya memiliki hak DML (`SELECT`, `INSERT`, `UPDATE`, `DELETE`), sedangkan migrasi schema dieksekusi oleh service khusus berhak DDL.
3. **Duplikasi File & Kompleksitas Build:** Karena batasan compiler Go di mana `//go:embed` tidak dapat mengakses path direktori di atas package induknya (`..`), file SQL sebelumnya harus di-copy secara manual dari `migrations/` ke `internal/migrations/` melalui Makefile dan Dockerfile sebelum build, menimbulkan risiko inkonsistensi (*drift*).

### Solusi Baru
Sistem migrasi dialihkan sepenuhnya menggunakan **Goose v3** dengan pola **Dedicated Standalone Runner CLI** (`cmd/migrate`):
- **Tanpa Embed (`no-embed`):** File SQL migrasi dibaca langsung dari filesystem (`migrations/`).
- **Terpisah dari Lifecycle Server:** Binary `cmd/server` tidak lagi menyentuh DDL atau menjalankan migrasi saat startup.
- **Dukungan Container Orchestration:** Pada Docker Compose / Kubernetes, migrasi dijalankan sebagai init-step atau pre-deploy job terpisah sebelum container aplikasi dimulai.
- **DX (Developer Experience) Ringkas:** Tersedia command Makefile standar untuk eksekusi, rollback, inspeksi status, dan pembuatan file migrasi baru.

---

## 2. Struktur Direktori & Pola File

```
hotel-booking/
├── cmd/
│   ├── migrate/
│   │   └── main.go          # Standalone Goose runner CLI
│   └── server/
│       └── main.go          # HTTP server & background workers (tanpa migrasi)
├── migrations/              # Sumber tunggal (single source of truth) file migrasi
│   ├── 00001_init.sql       # DDL schema awal sistem
│   └── 00002_seed.sql       # Seed data tipe kamar, inventory, dan kamar fisik
├── Makefile                 # Target helper: make migrate-up, migrate-down, dsb.
├── Dockerfile               # Multi-binary build (server + migrate)
└── docker-compose.yml       # Service 'migration' dengan condition dependency
```

### Konvensi Format File Goose
File migrasi menggunakan format SQL berpasangan dalam satu file dengan penomoran urut (`00001_xxx.sql`, `00002_xxx.sql`):
```sql
-- +goose Up
-- Query DDL / DML untuk memajukan schema database
...

-- +goose Down
-- Query DDL / DML untuk rollback ke versi sebelumnya
...
```

---

## 3. Rincian Migrasi SQL Saat Ini

### 3.1 `migrations/00001_init.sql` (Initial Schema)
Membangun fondasi database PostgreSQL 18 yang dirancang untuk mendukung concurrency booking tinggi, penanganan inventory fungible, jaminan kamar fisik via GiST, dan transactional outbox.

#### Fitur & Tabel Utama:
1. **Ekstensi `btree_gist`:**
   ```sql
   CREATE EXTENSION IF NOT EXISTS btree_gist;
   ```
   Dibutuhkan agar operator btree (`=`) untuk tipe data `text` dan `uuid` dapat digabungkan dengan operator spatial/range (`&&`) dalam exclusion constraint GiST.

2. **Katalog Tipe Kamar (`room_types`):**
   - Menggunakan primary key `UUID` dengan default fungsi native Postgres 18 `uuidv7()`.
   - Menyimpan `name` dan `max_capacity`.

3. **Schema A — Inventory Fungible Harian (`inventory`):**
   - Composite Primary Key: `(room_type_id, date)`.
   - Menggunakan kolom `total_rooms`, `available_rooms`, dan `version` (optimistic locking counter).
   - Check constraint: `available_rooms >= 0 AND available_rooms <= total_rooms`.
   - **Partial Index:**
     ```sql
     CREATE INDEX idx_inv_available ON inventory (room_type_id, date)
         WHERE available_rooms > 0;
     ```
     Mengoptimalkan query ketersediaan kamar yang hanya membutuhkan tanggal dengan sisa kamar > 0.

4. **Agregat Reservasi (`bookings`):**
   - Status terpusat dengan Check Constraint yang membatasi state yang sah:
     `status IN ('pending', 'confirmed', 'checked_in', 'checked_out', 'cancelled', 'expired', 'failed', 'no_show')`.
   - Check constraint tanggal valid: `check_out > check_in` (konvensi interval half-open `[check_in, check_out)`).
   - Mata uang ISO `CHAR(3)` dan harga dalam unit moneter minor integer (`BIGINT`).
   - Partial indexes:
     - `idx_bookings_room_type_dates`: Indeks cepat untuk booking yang aktif.
     - `idx_bookings_status`: Indeks untuk sweep booking berstatus `pending`.

5. **Detail Harga Per Malam (`reservation_room_nights`):**
   - Menyimpan breakdown audit harga per malam per booking (`booking_id`, `night_date`, `rate_minor`).

6. **Hold Sementara Kamar (`holds`):**
   - Menyimpan referensi booking berstatus pending beserta `expires_at`.
   - Dilengkapi indeks `idx_holds_expiry ON holds (expires_at)` untuk percepatan sweep background job hold kedaluwarsa.

7. **Schema B — Kamar Fisik & Assignment Check-in (`rooms` & `room_assignments`):**
   - `rooms`: Menyimpan inventori fisik kamar nyata per tipe kamar (misal: 101, 102, dst.).
   - `room_assignments`: Dipasangkan saat tamu check-in.
   - **Jaminan Database EXCLUDE USING GIST:**
     ```sql
     EXCLUDE USING GIST (room_number WITH =, stay_dates WITH &&)
     ```
     Menjamin di level database engine bahwa dua tamu tidak akan pernah bisa di-assign ke nomor kamar fisik yang sama pada rentang tanggal (`DATERANGE`) yang tumpang tindih (`&&`), sekalipun terjadi race condition di layer aplikasi.

8. **Transactional Outbox (`outbox`):**
   - Menampung domain events (`booking.created`, `booking.confirmed`, dsb.) dalam transaksi database yang sama dengan perubahan status agregat.
   - Status: `pending`, `done`, `failed`.
   - Partial Index: `CREATE INDEX idx_outbox_pending ON outbox (next_retry_at) WHERE status = 'pending';` untuk polling efisien dengan `FOR UPDATE SKIP LOCKED`.

---

### 3.2 `migrations/00002_seed.sql` (Seed Data Properti)
Mengisi data awal untuk simulasi 1 hotel independen (kapasitas 100 kamar) sesuai spesifikasi sistem desain:

1. **5 Tipe Kamar Standar:**
   - Standard (Kapasitas 2): ID `01900000-0000-7000-8000-000000000001`
   - Superior (Kapasitas 2): ID `01900000-0000-7000-8000-000000000002`
   - Deluxe (Kapasitas 3): ID `01900000-0000-7000-8000-000000000003`
   - Family (Kapasitas 4): ID `01900000-0000-7000-8000-000000000004`
   - Suite (Kapasitas 2): ID `01900000-0000-7000-8000-000000000005`

2. **Inventory Otomatis 365 Hari:**
   - Dihasilkan dinamis menggunakan `generate_series(date_trunc('day', now())::date, date_trunc('day', now())::date + 364, interval '1 day')`.
   - Alokasi kamar per tipe:
     - Standard: 30 kamar
     - Superior: 25 kamar
     - Deluxe: 20 kamar
     - Family: 15 kamar
     - Suite: 10 kamar
     - **Total = 100 kamar / hari**.

3. **100 Kamar Fisik (`rooms`):**
   - Format nomor kamar konsisten:
     - Standard: `101` s/d `130`
     - Superior: `201` s/d `225`
     - Deluxe: `301` s/d `320`
     - Family: `401` s/d `415`
     - Suite: `501` s/d `510`

---

## 4. Implementasi Runner CLI (`cmd/migrate/main.go`)

Runner ditulis menggunakan library Goose v3 dan driver standard library `pgx/v5/stdlib`:

```go
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)
...
```

### Karakteristik & Fitur Runner:
- **Konfigurasi Fleksibel:**
  - `-dir <path>`: Direktori tempat file SQL berada (default: `migrations`).
  - `-dsn <connection_string>`: Mengambil string koneksi dari argumen CLI atau environment variable `DATABASE_URL`. Default fallback ke setting dev lokal: `postgres://postgres:dev@localhost:5432/booking?sslmode=disable`.
- **Dialect Terisolasi:** Mengatur `goose.SetDialect("postgres")`.
- **Optimalisasi Koneksi:** Perintah `create` tidak memerlukan koneksi aktif ke database PostgreSQL, sehingga developer dapat membuat file migrasi baru secara offline tanpa database aktif.
- **Context-Aware:** Menggunakan `goose.RunContext` untuk kompatibilitas timeout dan graceful cancellation.

---

## 5. Cara Menjalankan Migrasi

### 5.1 Menggunakan Makefile (Rekomendasi Lokal)

Tersedia target Makefile yang membungkus pemanggilan runner:

| Perintah | Deskripsi |
|---|---|
| `make migrate-up` | Menjalankan seluruh file migrasi pending yang belum diaplikasikan ke database |
| `make migrate-down` | Melakukan rollback 1 step migrasi terakhir |
| `make migrate-status` | Memeriksa tabel `goose_db_version` dan menampilkan status applied/pending tiap file |
| `make migrate-reset` | Melakukan rollback seluruh migrasi hingga database kosong |
| `make migrate-create name=<nama>` | Membuat template file migrasi SQL baru di folder `migrations/` |

**Contoh Penggunaan:**
```bash
# Cek status migrasi terhadap database lokal
make migrate-status

# Buat migrasi baru untuk menambahkan tabel voucher
make migrate-create name=create_promos_table

# Jalankan migrasi ke database remote/staging
DATABASE_URL="postgres://user:pass@staging-db:5432/booking" make migrate-up
```

### 5.2 Menggunakan Go CLI Langsung

Runner dapat dipanggil langsung tanpa Makefile:
```bash
# Menampilkan help & daftar perintah yang didukung
go run ./cmd/migrate -h

# Menjalankan migrasi dengan flag direktori dan DSN kustom
go run ./cmd/migrate -dir=./migrations -dsn="postgres://postgres:dev@localhost:5432/booking?sslmode=disable" up

# Rollback spesifik ke versi tertentu
go run ./cmd/migrate -dir=./migrations down-to 1
```

### 5.3 Eksekusi dalam Docker Compose

Dalam file [`docker-compose.yml`](file:///mnt/code/projects/jobs/pulang/current-booking/docker-compose.yml), ditambahkan service khusus `migration`:

```yaml
services:
  postgres:
    image: postgres:18
    ...
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d booking"]
      interval: 3s
      timeout: 3s
      retries: 10

  migration:
    build: .
    entrypoint: ["migrate", "-dir=/migrations", "up"]
    environment:
      DATABASE_URL: postgres://postgres:dev@postgres:5432/booking?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: "no"

  app:
    build: .
    depends_on:
      postgres:
        condition: service_healthy
      valkey:
        condition: service_healthy
      migration:
        condition: service_completed_successfully
```

#### Alur Orkestrasi:
1. Container `postgres` menyala dan diverifikasi kesehatannya via healthcheck `pg_isready`.
2. Container `migration` menyala, menjalankan binary `migrate -dir=/migrations up`, dan keluar dengan kode `0` (*exit 0*).
3. Container `app` baru akan menyala setelah `migration` berstatus `service_completed_successfully`.
4. Jika terjadi kegagalan DDL pada migrasi, container `app` tidak akan pernah dijalankan, melindungi aplikasi dari inkonsistensi schema.

---

## 6. Panduan Menambahkan Migrasi Baru

Ikuti langkah-langkah berikut ketika membutuhkan perubahan schema database:

1. **Generate File Migrasi Baru:**
   ```bash
   make migrate-create name=add_promotions_table
   ```
   Goose akan membuat file baru dengan format penomoran timestamp/sequential di direktori `migrations/`, misal `migrations/00003_add_promotions_table.sql`.

2. **Tulis Script SQL `Up` dan `Down`:**
   Buka file yang baru dibuat dan isi blok yang bersangkutan:
   ```sql
   -- +goose Up
   CREATE TABLE promotions (
       id UUID PRIMARY KEY DEFAULT uuidv7(),
       code TEXT NOT NULL UNIQUE,
       discount_percent INT NOT NULL CHECK (discount_percent > 0 AND discount_percent <= 100),
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );

   -- +goose Down
   DROP TABLE IF EXISTS promotions;
   ```

3. **Uji Migrasi Secara Lokal:**
   ```bash
   # Jalankan migrasi
   make migrate-up

   # Periksa status
   make migrate-status

   # Uji rollback untuk memastikan script Down bekerja sempurna
   make migrate-down

   # Jalankan kembali ke versi terbaru
   make migrate-up
   ```

4. **Verifikasi Build dan Test:**
   ```bash
   make build
   make test
   ```

5. **Commit File Migrasi:**
   File di `migrations/` wajib di-commit ke source control bersamaan dengan perubahan kode domain Go terkait.
