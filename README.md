# Hotel Booking Engine

Implementasi dari dokumen desain `docs/hotel-booking-engine-system-design-2026-10-02.md` — booking engine untuk 1 hotel.

## Stack

| Komponen | Pilihan |
|---|---|
| Bahasa | Go 1.27 (modular monolith + hexagonal ports & adapters) |
| Database | PostgreSQL 18 (uuidv7, async I/O, partial index, GiST exclusion) |
| Job queue | asynq + Valkey 8 (AOF on, noeviction) |
| HTTP | chi |
| Observability | slog JSON, asynqmon, /healthz |

## Struktur

cmd/
  server/              # composition root: wiring + HTTP + workers + relay
  migrate/             # standalone migration runner (goose CLI)
internal/
  booking/             # domain core: state machine, use case, port, tx Postgres
  inventory/           # domain inventory + port AvailabilityStore
  rates/               # rate engine (base + weekend)
  adapter/             # adapter vendor: payment (fake), notifier (log)
  api/                 # transport HTTP (chi)
  workers/             # asynq tasks + outbox relay
  platform/            # config, pgx pool, valkey
migrations/            # SQL migrasi + seed (Goose format)
```

Aturan hexagonal: domain mendefinisikan port; vendor hanya di `internal/adapter`;
semua keputusan vendor di `cmd/server/main.go`.

## Menjalankan

```bash
docker compose up --build -d     # postgres 18 + valkey 8 + app + asynqmon
# Migrasi + seed jalan otomatis saat app start.
# Catatan: API di-map ke host port 18080 (host 8080/5432/6379 sering terpakai).
```

- API: http://localhost:18080
- asynqmon (monitoring task): http://localhost:18081

## Contoh Pemakaian API

```bash
RT="01900000-0000-7000-8000-000000000001"   # Standard

# 1. Cek availability + harga (weekend +25%)
curl "localhost:18080/api/v1/availability?room_type_id=$RT&check_in=2026-10-09&check_out=2026-10-12"

# 2. Buat booking (hold 30 menit + payment URL)
curl -s -X POST localhost:18080/api/v1/bookings -H 'Content-Type: application/json' -d '{
  "room_type_id": "'$RT'",
  "check_in": "2026-10-09", "check_out": "2026-10-12",
  "num_rooms": 1, "num_guests": 2,
  "guest_name": "Budi", "guest_email": "budi@example.com"
}'

# → catat booking.id dari respon

# 3. Simulasi bayar (dev): memicu path webhook yang sama dengan gateway nyata
curl -X POST "localhost:18080/fake-pay/x?booking_id=<BOOKING_ID>"

# 4. Lihat status booking (confirmed; email terkirim via outbox → log app)
curl localhost:18080/api/v1/bookings/<BOOKING_ID>
```

## Jaminan Desain yang Aktif di Kode

- **Anti double-booking**: transaksi `create` mengunci baris inventory `FOR UPDATE`
  terurut ASC (anti-deadlock), verifikasi atomik `available_rooms >= n`, decrement,
  insert booking+hold+outbox dalam SATU transaksi (desain §12.3).
- **Idempotent webhook**: transisi status bersyarat — duplikat → no-op 200 (§12.4).
- **Release-hold**: asynq scheduled task t+30m + sweep cadangan tiap menit (§12.1).
- **Outbox pattern**: event domain tersimpan dalam transaksi yang sama dengan
  perubahan state; relay `FOR UPDATE SKIP LOCKED` + retry backoff eksponensial +
  dead-letter (§5.3).
- **State machine**: transisi ilegal ditolak di domain + CHECK constraint di DB (§12.2).
- **GiST exclusion**: dua tamu tidak mungkin di kamar fisik yang sama untuk tanggal
  yang overlap — ditolak DATABASE saat check-in (§13).

## Perintah

```bash
make build                 # compile semua binary (server & migrate)
make test                  # unit test (state machine, inventory check, rate engine)
make vet
make run                   # jalankan server lokal (butuh postgres & valkey di localhost)

# Manajemen migrasi (Goose):
make migrate-up            # jalankan semua migrasi pending
make migrate-down          # rollback 1 migrasi terakhir
make migrate-status        # cek status migrasi
make migrate-reset         # rollback seluruh migrasi
make migrate-create name=tambah_tabel   # buat file migrasi baru
```

## Mengganti Vendor

| Port | Adapter sekarang | Ganti dengan |
|---|---|---|
| `booking.PaymentGateway` | `adapter/payment.FakeGateway` | tulis `adapter/payment/midtrans` lalu ubah 1 baris di main.go |
| `booking.Notifier` | `adapter/notifier.LogNotifier` | SMTP/Resend/SES adapter |
| Event relay | outbox → log/handler | tambah handler topic baru (PMS sync, dsb.) |
