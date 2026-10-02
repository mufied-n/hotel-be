# Technical Architecture & Concurrency Design — Batch BE-E
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status:** **APPROVED / READY FOR IMPLEMENTATION**
- **Referensi:** [`docs/prd/operational-reliability-concurrency-batch-e-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/operational-reliability-concurrency-batch-e-2026-10-03.md) & [`docs/srs/operational-reliability-concurrency-batch-e-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/operational-reliability-concurrency-batch-e-2026-10-03.md)

---

## 1. Arsitektur Alokasi Kamar Bebas Konflik (`BE-G17`)

### Diagram Urutan Konkurensi Check-In Paralel

```mermaid
sequenceDiagram
    autonumber
    actor RecA as Resepsionis A (Kamar Std)
    actor RecB as Resepsionis B (Kamar Std)
    participant Engine as Booking Engine
    participant DB as PostgreSQL (ACID)

    par Check-In A
        RecA->>Engine: POST /bookings/bk-1/check-in
        Engine->>DB: BEGIN TX A
        Engine->>DB: WITH free_rooms AS (SELECT room_number FROM rooms r ... FOR UPDATE OF r SKIP LOCKED LIMIT 1)
        Note over DB: TX A mengunci baris kamar 101
        DB-->>Engine: Kamar 101 dialokasikan
        Engine->>DB: INSERT INTO room_assignments (bk-1, 101)
        Engine->>DB: UPDATE bookings SET status='checked_in'
        Engine->>DB: COMMIT TX A
        Engine-->>RecA: 200 OK (Room: 101)
    and Check-In B (Simultan)
        RecB->>Engine: POST /bookings/bk-2/check-in
        Engine->>DB: BEGIN TX B
        Engine->>DB: WITH free_rooms AS (SELECT room_number FROM rooms r ... FOR UPDATE OF r SKIP LOCKED LIMIT 1)
        Note over DB: Baris 101 terkunci TX A -> SKIP LOCKED lewati kamar 101, pilih kamar 102!
        DB-->>Engine: Kamar 102 dialokasikan
        Engine->>DB: INSERT INTO room_assignments (bk-2, 102)
        Engine->>DB: UPDATE bookings SET status='checked_in'
        Engine->>DB: COMMIT TX B
        Engine-->>RecB: 200 OK (Room: 102)
    end
```

### Penjelasan Mekanisme `SKIP LOCKED`
Pada implementasi sebelumnya, kedua query memeriksa keberadaan kamar fisik yang belum ter-assign tanpa row lock. Akibatnya, kedua transaksi memilih kamar dengan nomor urut terkecil yang sama (misal 101). Transaksi pertama berhasil memasang assignment, sedangkan transaksi kedua ditolak oleh exclusion constraint GiST (`room_number WITH =, stay_dates WITH &&`), memunculkan *false failure*.

Dengan menambahkan klausa `FOR UPDATE OF r SKIP LOCKED` pada CTE `free_rooms`, baris kamar pada tabel `rooms` dikunci oleh transaksi yang sedang memilihnya. Transaksi paralel yang berjalan pada waktu yang sama secara otomatis melompati (*skips*) baris kamar yang sedang dikunci dan langsung memilih kamar fisik bebas berikutnya.

---

## 2. Diagram Alur Restitusi Inventaris Early Check-Out (`BE-G22`)

```mermaid
flowchart TD
    StartCheckOut["Check-Out Dipicu\n(POST /bookings/:id/check-out)"] --> FetchBooking["GetForUpdate Booking"]
    FetchBooking --> CheckStatus{"Status Saat Ini?"}
    CheckStatus -- CheckedOut --> ReturnIdempotent["Return 200 OK\n(Idempotent)"]
    CheckStatus -- Bukan CheckedIn --> RejectIllegal["Return 409 Conflict\n(Illegal Transition)"]
    CheckStatus -- CheckedIn --> CalcDates["Hitung Rentang Tanggal:\nToday = trunc(now.UTC)\nScheduledOut = trunc(b.CheckOut)"]
    CalcDates --> IsEarly{"Today < ScheduledOut?"}
    IsEarly -- Ya (Early Departure) --> Restitute["Increment Inventory Calendar\n[Today, ScheduledOut) sejumlah NumRooms"]
    IsEarly -- Tidak (Normal Departure) --> SkipRestitution["Pertahankan Stok Terpakai\n(Half-Open [CheckIn, CheckOut))"]
    Restitute --> UpdateBooking["Update status='checked_out'"]
    SkipRestitution --> UpdateBooking
    UpdateBooking --> OutboxEvent["Publish Outbox: booking.checked_out\n(early_checkout: true/false)"]
    OutboxEvent --> CommitTx["COMMIT TX"]
    CommitTx --> Done["Selesai"]
```

---

## 3. Desain Graceful Shutdown & Reliability Observability (`BE-G21`)

1. **Koordinasi Penghentian Komponen:**
   - Saat sinyal OS (`SIGINT`, `SIGTERM`) diterima atau server HTTP gagal listen, context aplikasi dibatalkan.
   - Timeout shutdown dialokasikan 10 detik.
   - HTTP server memanggil `srv.Shutdown(shutdownCtx)`.
   - Asynq worker server memanggil `asynqSrv.Shutdown()`.
   - Outbox relay berhenti memproses polling baru melalui `<-ctx.Done()`.
2. **Observabilitas Sweep Expired Hold:**
   - Fungsi `workers.SweepExpiredHolds` mengevaluasi `rows.Err()` pasca iterasi.
   - Jika terjadi error pada jaringan atau pool saat streaming cursor, error terminal dicatat ke `log.ErrorContext` dan cursor ditutup secara bersih.
