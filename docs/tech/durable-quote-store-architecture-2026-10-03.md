# Technical Architecture: Penyimpanan Kuotasi Harga Terdistribusi (Durable Quote Store) (BE-R11)

**Nomor Dokumen:** ARCH-PULANG-BE-R11  
**Tanggal Efektif:** 3 Oktober 2026  
**Status:** Approved  
**Author:** AI Engineering Agent  
**Terkait Audit & PRD:** `BE-R11`, `PRD-PULANG-BE-R11`, `SRS-PULANG-BE-R11`

---

## 1. Arsitektur Komponen & Diagram Alur

### 1.1 Diagram Interaksi Multi-Instance Terdistribusi
```mermaid
flowchart TD
    Client["Browser / Guest Client"]
    LB["Load Balancer / Ingress"]
    
    subgraph AppCluster["Aplikasi Backend (Multi-Instance Replicas)"]
        InstanceA["Instance A (:28080)\nrates.Engine\nrates.ValkeyQuoteStore"]
        InstanceB["Instance B (:28081)\nrates.Engine\nrates.ValkeyQuoteStore"]
    end
    
    subgraph DataTier["Data Tier"]
        Valkey["Valkey 8 / Redis Cluster\nKey: hotel:quote:{id}\nTTL: 15 Menit (EX 900)"]
        Postgres["PostgreSQL 18 Database\nbookings & inventory"]
    end

    Client -- "1. POST /api/v1/quotes" --> LB
    LB -- "Di-route ke" --> InstanceA
    InstanceA -- "2. SET hotel:quote:{id} EX 900" --> Valkey
    InstanceA -- "3. Return 200 OK (quote_id)" --> Client

    Client -- "4. POST /api/v1/bookings (quote_id)" --> LB
    LB -- "Di-route ke" --> InstanceB
    InstanceB -- "5. GET hotel:quote:{id}" --> Valkey
    Valkey -- "6. Return JSON LockedQuote" --> InstanceB
    InstanceB -- "7. BEGIN TX: Insert booking & Hold room" --> Postgres
    InstanceB -- "8. Return 201 Created (Booking confirmed/held)" --> Client
```

### 1.2 Sequence Diagram: Penanganan Fail-Closed saat Persistensi Gagal
```mermaid
sequenceDiagram
    autonumber
    actor Guest as Guest Client
    participant Router as API Router
    participant Engine as rates.Engine
    participant Store as rates.ValkeyQuoteStore
    participant Valkey as Valkey / Redis Service

    Guest->>Router: POST /api/v1/quotes
    Router->>Engine: CalculateLockedQuote(ctx, req)
    Engine->>Engine: Hitung tarif kamar, sarapan, diskon, pajak
    Engine->>Store: SaveQuote(ctx, lq)
    Store->>Valkey: SET hotel:quote:{id} <payload> EX 900
    alt Valkey Terhubung & Berhasil
        Valkey-->>Store: OK
        Store-->>Engine: nil
        Engine-->>Router: lq, nil
        Router-->>Guest: 200 OK (LockedQuote JSON)
    else Valkey Gagal / Jaringan Putus
        Valkey-->>Store: Connection Refused / Timeout
        Store-->>Engine: error
        Engine-->>Router: ErrSaveQuoteFailed
        Router-->>Guest: 500 Internal Server Error (INTERNAL_ERROR)
        Note over Guest, Router: Mencegah penerbitan 'phantom quote' yang tidak bisa di-checkout
    end
```

---

## 2. Struktur Data & Pola Desain (Hexagonal Pattern)

### 2.1 Antarmuka Port
```go
package rates

type QuoteStore interface {
    SaveQuote(ctx context.Context, q LockedQuote) error
    GetQuote(ctx context.Context, id string) (LockedQuote, error)
}
```

### 2.2 Implementasi Adapter Valkey (`internal/rates/valkey_store.go`)
```go
package rates

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

const (
    defaultQuotePrefix = "hotel:quote:"
    defaultQuoteTTL    = 15 * time.Minute
)

var (
    ErrSaveQuoteFailed = errors.New("rates: failed to save quote")
)

type ValkeyQuoteStore struct {
    client redis.Cmdable
    ttl    time.Duration
    prefix string
}

func NewValkeyQuoteStore(client redis.Cmdable, ttl time.Duration) *ValkeyQuoteStore {
    if ttl <= 0 {
        ttl = defaultQuoteTTL
    }
    return &ValkeyQuoteStore{
        client: client,
        ttl:    ttl,
        prefix: defaultQuotePrefix,
    }
}
```

### 2.3 Fail-Closed Propagation di `Engine.CalculateLockedQuote`
```go
    if e.quoteStore != nil {
        if err := e.quoteStore.SaveQuote(ctx, lq); err != nil {
            return LockedQuote{}, fmt.Errorf("%w: %v", ErrSaveQuoteFailed, err)
        }
    }
```

---

## 3. Analisis Anti-Overengineering (Ponytail Principles)

1. **YAGNI (You Aren't Gonna Need It):**
   - Tidak perlu membuat background database worker atau cron cleanup untuk membersihkan kuotasi kedaluwarsa. Valkey/Redis memiliki native key expiration (`EX 900`) yang membersihkan memori secara otomatis.
   - Tidak menambahkan layer caching bertingkat (misal: L1 memory + L2 redis) yang memicu *cache invalidation drift* antar instansi. Cukup delegasikan single-source-of-truth quote TTL ke Valkey.
2. **Ketergantungan Minimal:**
   - Memanfaatkan library yang sudah ada di repositori: `github.com/redis/go-redis/v9` dan `encoding/json` bawaan Go standard library. Tanpa dependensi pihak ketiga baru.
3. **Komposisi Tanpa Breaking Changes:**
   - Antarmuka `QuoteStore` tetap identik sehingga seluruh unit test yang menggunakan `MemoryQuoteStore` tetap berfungsi normal.
   - `ValkeyQuoteStore` diintegrasikan di `cmd/server/main.go` memanfaatkan `redisClient` yang telah terkonfigurasi.
