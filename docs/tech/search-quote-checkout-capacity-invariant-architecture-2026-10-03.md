# Desain Arsitektur Teknis: Invariant Kapasitas Kamar Search, Quote, dan Checkout (BE-R07)

**Nomor Dokumen:** ARCH-PULANG-BE-R07  
**Tanggal:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait:** BE-R07, BE-G03, F01, F08  

---

## 1. Ikhtisar Arsitektur

Fitur ini menyelaraskan penegakan invariant kapasitas fisik kamar hotel di seluruh lapisan boundary arsitektur hexagonal (Transport API, Rate Engine, dan Core Booking Domain):

```mermaid
flowchart TD
    subgraph Client["Client (Guest / Third-Party / Direct API)"]
        ReqSearch["GET /api/v1/search"]
        ReqQuote["POST /api/v1/quotes"]
        ReqBook["POST /api/v1/bookings"]
    end

    subgraph API["Transport Layer (internal/api)"]
        HSearch["searchRooms Handler\n- adults >= rooms\n- child_ages count == children\n- children <= MaxChildren*rooms\n- adults <= MaxAdults*rooms"]
        HQuote["calculateQuote Handler\n- validate num_rooms [1..8]\n- num_guests >= rooms\n- num_guests <= MaxCapacity*rooms\n- validate against CatalogStore"]
        HBook["createBooking Handler\n- validate num_guests <= MaxCapacity*rooms\n- num_guests >= rooms\n- quote match verification"]
    end

    subgraph Domain["Core Domain & Ports"]
        CatStore[("Catalog Store\n(internal/catalog)")]
        RateSvc["Rate Engine\n(internal/rates)"]
        BookSvc["booking.Service\n(internal/booking)"]
        CatPort["CatalogReader Port\nGetVariant(ctx, id)"]
    end

    ReqSearch --> HSearch
    ReqQuote --> HQuote
    ReqBook --> HBook

    HSearch --> CatStore
    HSearch --> RateSvc
    HQuote --> CatStore
    HQuote --> RateSvc
    HBook --> BookSvc
    BookSvc -.-> CatPort
    CatPort --> CatStore
```

---

## 2. Diagram Alur Validasi Kapasitas

```mermaid
sequenceDiagram
    autonumber
    actor Caller as Klien API
    participant API as Transport (router.go)
    participant Cat as CatalogStore
    participant Svc as booking.Service
    participant Inv as Inventory DB

    Caller->>API: POST /api/v1/quotes (room_type_id, rooms=1, guests=5)
    API->>Cat: GetVariant(room_type_id)
    Cat-->>API: Superior King (MaxCapacity=3)
    Note over API: 5 > 3 * 1 -> EXCEEDS_CAPACITY
    API-->>Caller: 400 Bad Request ("EXCEEDS_CAPACITY")

    Caller->>API: POST /api/v1/bookings (Direct bypass, guests=6)
    API->>Cat: GetVariant(room_type_id)
    Note over API: Direct API guard fails
    API-->>Caller: 400 Bad Request ("EXCEEDS_CAPACITY")

    Note over Svc: Domain Guard (Defence in Depth):
    Caller->>Svc: Create(in) [if bypassed API layer]
    Svc->>Cat: GetVariant(in.RoomTypeID)
    Note over Svc: num_guests > MaxCapacity*rooms
    Svc-->>Caller: ErrExceedsCapacity
```

---

## 3. Desain Komponen & Interface (Hexagonal Port)

### 3.1 Port `CatalogReader` pada `internal/booking/service.go`
Sesuai prinsip Hexagonal Architecture (§8.2), domain `booking` mendefinisikan kontrak interface yang dibutuhkannya sendiri:

```go
// CatalogReader mendefinisikan port untuk membaca spesifikasi varian kamar (BE-R07).
type CatalogReader interface {
    GetVariant(ctx context.Context, idOrCode string) (catalog.RoomVariant, error)
}
```

Service disematkan method injeksi dependency:
```go
func (s *Service) SetCatalogStore(cs CatalogReader) {
    s.catalogStore = cs
}
```

### 3.2 Penegakan Invariant pada `booking.Service.Create`
```go
// Validasi kapasitas dasar
if in.NumRooms < 1 || in.NumRooms > 8 || in.NumGuests < 1 {
    return Booking{}, ChargeResult{}, ErrInvalidCapacity
}
if in.NumGuests < in.NumRooms {
    return Booking{}, ChargeResult{}, ErrInvalidCapacity
}

// Validasi terhadap batas fisik katalog (BE-R07)
if s.catalogStore != nil {
    variant, err := s.catalogStore.GetVariant(ctx, in.RoomTypeID)
    if err != nil {
        if errors.Is(err, catalog.ErrVariantNotFound) {
            return Booking{}, ChargeResult{}, ErrUnknownRoomType
        }
        return Booking{}, ChargeResult{}, fmt.Errorf("booking: catalog lookup: %w", err)
    }
    if in.NumGuests > variant.MaxCapacity*in.NumRooms {
        return Booking{}, ChargeResult{}, ErrExceedsCapacity
    }
}
```

---

## 4. Analisis Anti-Overengineering (Prinsip Ponytail)

1. **YAGNI (You Aren't Gonna Need It):**
   - Tidak menambahkan tabel database baru atau migration skema baru, karena kapasitas fisik (`max_capacity`, `max_adults`, `max_children`) sudah ada pada tabel `room_types` dan struct `RoomVariant`.
   - Tidak membuat microservice validator terpisah; validasi dijalankan secara in-process dengan sub-microsecond latency.
2. **Reuse Existing Ports:**
   - Memanfaatkan interface `catalog.Store` yang sudah ada dan mengikatnya sebagai `CatalogReader` pada booking service.
3. **Fail-Fast Early:**
   - Error overcapacity dicegah sebelum menyentuh transaksi database PostgreSQL `InTx` dan sebelum mengunci baris inventori `FOR UPDATE`, menghemat resource DB connection pool.
