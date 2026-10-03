# Technical Architecture & Design — Proposed 03: Stay Modification & Room Move
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 2026-10-03
- **Status:** Approved / Technical Design
- **Dokumen PRD:** [PRD Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/stay-modification-room-move-and-extension-2026-10-03.md)
- **Dokumen SRS:** [SRS Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/stay-modification-room-move-and-extension-2026-10-03.md)

---

## 1. Arsitektur Domain & Pola Transaksi

Modul Stay Modification ditempatkan sebagai sub-domain operasional pada `internal/stay/` atau terintegrasi dengan port `booking.Store`, `inventory.Store`, dan `housekeeping.Store`:

```mermaid
flowchart TD
    subgraph Transport_Layer [HTTP Transport Layer]
        API[API Router /api/v1/bookings/:id]
        Handler[Stay Handler: MoveRoom, ExtendStay, ListMoves]
    end

    subgraph Service_Layer [Stay Domain Service]
        StaySvc[Stay Service]
    end

    subgraph Port_Interfaces [Driven Ports]
        StorePort[Stay Store Interface]
        InvPort[Inventory Store Interface]
        RatePort[Rate Engine Interface]
        HKPort[Housekeeping Store Interface]
    end

    subgraph PostgreSQL_Database [PostgreSQL Database]
        PG_Tx[Atomic PostgreSQL Transaction]
        T_RA[room_assignments - GiST Exclude]
        T_Rooms[rooms - Cleanliness Status]
        T_Logs[room_move_logs]
        T_Inv[inventory]
        T_Bookings[bookings & reservation_room_nights]
    end

    API --> Handler
    Handler --> StaySvc
    StaySvc --> StorePort
    StaySvc --> InvPort
    StaySvc --> RatePort
    StaySvc --> HKPort

    StorePort --> PG_Tx
    InvPort --> PG_Tx
    PG_Tx --> T_RA
    PG_Tx --> T_Rooms
    PG_Tx --> T_Logs
    PG_Tx --> T_Inv
    PG_Tx --> T_Bookings
```

---

## 2. Diagram Sekuens Alur Room Move (Atomisitas Transaksional)

```mermaid
sequenceDiagram
    autonumber
    actor Staf as Resepsionis Meja Depan
    participant API as Stay HTTP Handler
    participant Svc as Stay Domain Service
    participant PG as PostgreSQL Tx Runner
    participant RoomDB as rooms Table
    participant AssignDB as room_assignments (GiST)
    participant LogDB as room_move_logs Table

    Staf->>API: POST /api/v1/bookings/{id}/room-move<br/>{target_room: "305", reason: "maintenance_defect"}
    API->>Svc: MoveRoom(ctx, input)
    Svc->>PG: InTx(ctx, func(tx) error)
    
    Note over PG,RoomDB: 1. Verifikasi Status Kamar Target
    PG->>RoomDB: SELECT cleanliness_status FROM rooms WHERE room_number = '305' FOR UPDATE
    alt Kamar Target != 'inspected'
        PG-->>Svc: ErrTargetRoomNotReady (HTTP 409)
        Svc-->>API: 409 Conflict (TARGET_ROOM_NOT_READY)
        API-->>Staf: Ditolak: Kamar belum diinspeksi
    else Kamar Target == 'inspected'
        Note over PG,AssignDB: 2. Potong Masa Menginap Kamar Asal (101)
        alt move_date > check_in
            PG->>AssignDB: UPDATE room_assignments<br/>SET stay_dates = daterange(lower(stay_dates), CURRENT_DATE, '[)')<br/>WHERE booking_id = $id AND room_number = '101'
        else move_date == check_in
            PG->>AssignDB: DELETE FROM room_assignments<br/>WHERE booking_id = $id AND room_number = '101'
        end

        Note over PG,AssignDB: 3. Pasang Kamar Baru (305)
        PG->>AssignDB: INSERT INTO room_assignments (booking_id, room_number, stay_dates)<br/>VALUES ($id, '305', daterange(CURRENT_DATE, check_out, '[)'))
        Note right of AssignDB: GiST constraint memastikan tidak ada overlap

        Note over PG,RoomDB: 4. Transisi Status Kebersihan Fisik
        PG->>RoomDB: UPDATE rooms SET cleanliness_status = 'vacant_dirty'<br/>WHERE room_number = '101'
        PG->>RoomDB: UPDATE rooms SET cleanliness_status = 'occupied'<br/>WHERE room_number = '305'

        Note over PG,LogDB: 5. Catat Log Audit Pemindahan
        PG->>LogDB: INSERT INTO room_move_logs (...)

        PG-->>Svc: Commit Transaction Sukses
        Svc-->>API: Result (Previous: 101, New: 305)
        API-->>Staf: 200 OK (Room Move Selesai)
    end
```

---

## 3. Diagram Alur Transaksi Perpanjangan Menginap (Stay Extension)

```mermaid
flowchart TD
    Start[Mulai Perpanjangan Menginap] --> Input[Input additional_nights >= 1]
    Input --> CalcDates[Hitung new_check_out = old_check_out + additional_nights]
    CalcDates --> CheckInv[Cek & Lock Inventaris Rentang Tambahan]
    
    CheckInv -->|Ada Tanggal Habis| ErrInv[409 NO_AVAILABILITY_FOR_EXTENSION]
    CheckInv -->|Stok Tersedia| DecrementInv[Kurangi Kuota Inventaris Secara Atomik]
    
    DecrementInv --> CalcRates[Hitung Tarif Malam Tambahan via Pricing Engine]
    CalcRates --> CheckPhysicalRoom{Apakah Kamar Fisik Bebas pada Rentang Tambahan?}
    
    CheckPhysicalRoom -->|Ada Benturan GiST| ErrOverlap[409 ROOM_PHYSICAL_OVERLAP]
    CheckPhysicalRoom -->|Bebas| ExtendPhysical[Perpanjang stay_dates pada room_assignments]
    
    ExtendPhysical --> UpdateBooking[Update check_out & total_price_minor pada bookings]
    UpdateBooking --> InsertNights[Insert malam baru ke reservation_room_nights]
    InsertNights --> Finish[Commit Tx & Response 200 OK]
```

---

## 4. Analisis Anti-Overengineering (Ponytail Standard)

1. **YAGNI (You Aren't Gonna Need It):**
   * Tidak menambahkan sub-sistem event bus terpisah untuk room move log. Audit log disimpan langsung dalam transaksi database yang sama dengan eksekusi perpindahan fisik kamar.
2. **Minimal Abstraction:**
   * Memanfaatkan kemampuan native PostgreSQL:
     - `daterange` dan klausul GiST exclusion constraint (`EXCLUDE USING GIST`) menggantikan ratusan baris kode logika pendeteksi overlap di aplikasi.
     - `lower(stay_dates)` dan operator `&&` menjamin nol balapan data (*zero race condition*).
3. **Pemanfaatan Reusable Components:**
   * Perhitungan tarif malam tambahan menggunakan komponen `rates.Engine` yang sudah teruji.
   * Modifikasi status kamar memanfaatkan kolom `cleanliness_status` pada tabel `rooms` yang telah diinisialisasi pada Proposed 01.

---

## 5. Pertimbangan Konkurensi & Keamanan

- **Isolasi Transaksi:**
  Seluruh operasi pemindahan kamar dan perpanjangan menginap dibungkus dalam PostgreSQL `InTx` dengan query `FOR UPDATE` pada baris kamar target untuk mencegah dua resepsionis memindahkan tamu berbeda ke kamar target yang sama pada milidetik yang sama.
- **Fail-Closed RBAC:**
  Endpoint dilindungi oleh middleware Casbin enforcer. Tamu (`guest`) atau staf yang tidak memiliki wewenang langsung ditolak di lapisan HTTP transport (403 Forbidden).
