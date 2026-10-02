# Research & System Design: Hotel Booking Engine (1 Hotel)

> **Tanggal:** 2 Oktober 2026
> **Status:** Design terkunci — siap untuk implementasi
> **Konteks:** Dokumen ini merangkum seluruh proses research dan analisis system design untuk pembangunan booking engine untuk **1 hotel**, berdasarkan serangkaian diskusi teknis (pemilihan model arsitektur, evaluasi message broker, pemilihan stack, arsitektur hexagonal, hingga deep-dive concurrency dan schema database).

---

## Daftar Isi

1. [Latar Belakang & Konteks Domain](#1-latar-belakang--konteks-domain)
2. [Tujuan, Scope, dan Non-Goals](#2-tujuan-scope-dan-non-goals)
3. [Analisis Arsitektur Industri Booking Engine](#3-analisis-arsitektur-industri-booking-engine)
4. [Keputusan Arsitektural #1: Monolith vs Microservices](#4-keputusan-arsitektural-1-monolith-vs-microservices)
5. [Keputusan Arsitektural #2: Strategi Async Processing & Message Broker](#5-keputusan-arsitektural-2-strategi-async-processing--message-broker)
6. [Keputusan Arsitektural #3: Redis vs Valkey](#6-keputusan-arsitektural-3-redis-vs-valkey)
7. [Keputusan Arsitektural #4: Real-Time — Pub/Sub, SSE, WebSocket](#7-keputusan-arsitektural-4-real-time--pubsub-sse-websocket)
8. [Keputusan Arsitektural #5: Hexagonal Architecture (Ports & Adapters)](#8-keputusan-arsitektural-5-hexagonal-architecture-ports--adapters)
9. [Final Tech Stack](#9-final-tech-stack)
10. [System Design: Kapasitas & Estimasi (Back-of-Envelope)](#10-system-design-kapasitas--estimasi-back-of-envelope)
11. [System Design: High-Level Architecture](#11-system-design-high-level-architecture)
12. [System Design: Deep Dive — Masalah Double-Booking](#12-system-design-deep-dive--masalah-double-booking)
13. [System Design: Deep Dive — Schema Database](#13-system-design-deep-dive--schema-database)
14. [System Design: Data Flow Lengkap](#14-system-design-data-flow-lengkap)
15. [System Design: Non-Functional Requirements](#15-system-design-non-functional-requirements)
16. [Tradeoffs & Skor Desain](#16-tradeoffs--skor-desain)
17. [Roadmap Implementasi](#17-roadmap-implementasi)
18. [Sumber & Referensi](#18-sumber--referensi)

---

## 1. Latar Belakang & Konteks Domain

### 1.1 Hasil research: apa itu `book-secure.com`

Sebagai titik awal, dilakukan research terhadap `www.book-secure.com` — sebuah domain yang sering muncul di website resmi hotel. Temuan:

- **book-secure.com adalah platform *booking engine* pihak ketiga (white-label) untuk hotel.** Bukan OTA (Online Travel Agent) seperti Booking.com/Traveloka, melainkan perusahaan **B2B software** yang menjual sistem reservasi ke hotel.
- Hotel yang berlangganan akan mengarahkan tombol *Book Now* di website resminya ke domain vendor, dengan pola URL khas:

  ```
  https://www.book-secure.com/index.php?s=results&property=mykua31641&rate=NRF-Promo—Breakfast-Inclusive
  ```

  - `property=...` → kode ID hotel pada platform vendor.
  - `rate=...` → kode paket/kamar yang dipromosikan.
- Dipakai banyak jaringan hotel resmi di Asia: Dorsett (MY), Impiana KLCC (MY), KIP Hotel (MY), Tanza Oasis (PH), Hotel ICON (HK), dan lainnya.
- Kompetitor yang teridentifikasi via Similarweb: Sabre, Travelport, Vertical Booking, OnePageBooking, secure-hotel-booking.com.
- **Tidak ada API publik terdokumentasi** untuk developer umum. Akses hanya untuk hotel/partner resminya. ScamAdviser menilai domain ini *legit*.

### 1.2 Distingsi penting: OTA vs Booking Engine Vendor

| Aspek | OTA (Booking.com, Agoda) | Booking Engine Vendor (book-secure.com) |
|---|---|---|
| Model bisnis | Marketplace; komisi per booking | SaaS B2B; langganan per hotel |
| Relasi pelanggan | Dimiliki OTA | **Tetap milik hotel** (direct booking) |
| Harga & pembayaran | Ditentukan/lewat OTA | Harga hotel langsung, bayar ke hotel |
| Branding | Milik OTA | *White-label* — tampilan menyesuaikan brand hotel |

**Analogi:** seperti toko online kecil yang memakai Shopify — pembeli tetap berbelanja di toko tersebut, tetapi mesin di baliknya milik Shopify.

### 1.3 Implikasi untuk proyek ini

Kesimpulan dari konteks domain: kita akan **membangun sendiri booking engine** (peran yang setara dengan book-secure.com) untuk **satu hotel**. Artinya sistem yang harus dibangun mencakup: pencarian ketersediaan, perhitungan harga, checkout, pembayaran, sinkronisasi inventory, dan notifikasi — namun pada skala satu properti.

---

## 2. Tujuan, Scope, dan Non-Goals

### 2.1 Tujuan (Goals)

- Sistem reservasi langsung (direct booking) end-to-end untuk 1 hotel: search → pilih rate → checkout → bayar → konfirmasi.
- Jaminan kuat **tidak terjadi double-booking** — dijamin di level database, bukan hanya kode aplikasi.
- Ekosistem vendor/3rd party yang mudah diganti (payment gateway, email, PMS) tanpa menyentuh domain core.
- Operasional sederhana: satu binary Go, dua dependensi infra (Postgres, Valkey).
- Observability minimum yang sehat sejak hari pertama: structured log, UI monitoring task, health check.

### 2.2 Non-Goals (sengaja TIDAK dilakukan sekarang)

| Non-Goal | Alasan |
|---|---|
| Multi-hotel / SaaS | Scope 1 hotel; arsitektur modular tetap menyiapkan jalur jika suatu hari berubah |
| API publik untuk developer | Tidak ada kebutuhan; menghindari beban keamanan & versioning |
| Sharding / multi-region database | Trafik tidak membutuhkan; kompleksitas tidak sebanding |
| Kafka/RabbitMQ/event sourcing | Volume async ratusan pesan/hari — outbox + asynq lebih dari cukup |
| Implementasi PCI DSS in-house | Data kartu ditangani penuh oleh payment gateway (hosted/redirect) |
| Search engine (Elasticsearch) untuk discovery | Inventory 1 hotel dapat di-query langsung dari Postgres sub-milidetik |

### 2.3 Functional Requirements inti

1. Tamu mencari ketersediaan per rentang tanggal + jumlah tamu.
2. Sistem menghitung harga final per malam (base rate, promo, weekday/weekend).
3. Tamu melakukan checkout → kamar **di-hold** sementara (TTL).
4. Pembayaran via payment gateway (redirect/hosted page) → webhook konfirmasi.
5. Booking confirmed → email konfirmasi + notifikasi front desk.
6. Front desk: check-in (assign kamar fisik), check-out, cancel, no-show.
7. Admin: kelola room type, rate, promo, blokir kamar (maintenance).
8. Hold kedaluwarsa → kamar otomatis kembali tersedia.

---

## 3. Analisis Arsitektur Industri Booking Engine

Sebelum merancang, dipetakan dulu bagaimana industri membangun sistem sejenis, agar keputusan kita berpijak pada pola yang sudah teruji.

### 3.1 Posisi dalam ekosistem

```
[Tamu] ──► [Website Hotel] ──► [Booking Engine] ──► [PMS Hotel]
                    │                  │
                    │                  ├──► [Payment Gateway]
                    │                  └──► [Channel Manager] ──► OTA (Booking.com, Agoda, dst)
                    └──► [Email/CRM tools]
```

Konsep kunci: **inventory kamar itu satu, tetapi dijual di banyak kanal** (website hotel + OTA). Sistem harus menjaga sinkronisasi antar kanal, idealnya dalam hitungan detik, untuk mencegah overbooking antar kanal.

### 3.2 Komponen khas booking engine komersial

| Komponen | Tanggung jawab | Catatan desain |
|---|---|---|
| Multi-tenant app tier | 1 deployment melayani ribuan hotel | Di proyek kita: single-tenant (1 hotel) — lebih sederhana |
| Rate Engine | Harga final = base rate + promo + musim + durasi | Logika pricing diisolasi sebagai modul |
| Inventory/Availability | Hitung sisa kamar per tanggal | *Hot path* — hampir semua request menyentuhnya |
| Cache | Availability & rate cache (TTL pendek) | **Lihat analisis §12.5: di skala 1 hotel, cache ini justru tidak diperlukan** |
| Booking State Machine | pending → confirmed → checked_in → dst. | Booking bukan transaksi instan; ada jeda menunggu pembayaran |
| Message Queue | Email, sync PMS, invoice (async) | Pemisahan antara *request path* dan *background work* |
| Payment Gateway | 3D-Secure, tokenization | Vendor engine **tidak menyentuh** data kartu (PCI scope) |

### 3.3 Insight arsitektural utama dari industri

1. **Kesulitan utama bukan traffic, melainkan korektness state** — hold, pembayaran, dan sinkronisasi inventory antar kanal.
2. **Read-heavy ekstrem** (orang mencari jauh lebih banyak daripada memesan) — tetapi pada skala 1 hotel, Postgres saja sudah sangat cepat untuk read path.
3. **Async processing adalah wajib** — email konfirmasi tidak boleh diblokir di request path tamu.
4. **Channel manager / PMS adalah integrasi yang rapuh** — sistem PMS hotel sering legacy; sinkronisasi harus *resilient* (retry, queue), bukan blocking.

---

## 4. Keputusan Arsitektural #1: Monolith vs Microservices

### 4.1 Analisis skala (back-of-envelope)

Hotel 100 kamar, musim ramai:

- Booking per hari: **20–50**
- Query availability: **~10–50 QPS** (puncak ekstrem < 500 QPS)

**Kesimpulan: satu instance Go + satu Postgres tidak akan kewalahan.** Masalah sebenarnya adalah korektness (hold, pembayaran, sinkronisasi) — dan itu tidak membutuhkan microservices untuk diselesaikan dengan benar.

### 4.2 Biaya microservices pada skala ini

| Microservices berarti harus menangani | Manfaat nyata untuk 1 hotel |
|---|---|
| Service discovery, inter-service auth, network calls | ≈ 0 |
| Distributed tracing, debugging antar service | Debugging justru lebih sulit |
| Idempotency antar layanan, saga/compensating transaction | Masalah yang justru kita hindari |
| CI/CD per service, banyak repositori | Maintenance ≫ benefit |

### 4.3 Keputusan: **Modular Monolith + Hexagonal** (bukan microservices)

Prinsip: *split when it hurts* — jangan split sebelum ada yang sakit. Namun modularitas internal tetap dipertahankan ketat agar pemisahan di masa depan (misal berubah menjadi SaaS multi-hotel) relatif bersih.

```
my-hotel-booking/
├── cmd/
│   └── server/
│       └── main.go            # wiring + HTTP server (composition root)
├── internal/
│   ├── booking/               # state machine: pending → confirmed → cancel
│   ├── inventory/             # availability, hold kamar, release timeout
│   ├── rates/                 # pricing: base rate, promo, season
│   ├── payment/               # port PaymentGateway
│   ├── notification/          # port Notifier (email)
│   ├── hub/                   # SSE hub + Valkey bridge
│   ├── adapter/               # implementasi vendor: midtrans/, resend/, dst.
│   └── platform/              # db (pgx), valkey, config, middleware
├── migrations/                # schema SQL versioned
└── go.mod
```

Aturan main modul: **`booking` tidak boleh query SQL langsung ke tabel milik `inventory`** — harus lewat service/interface milik paket tersebut. Satu aturan ini cukup untuk menjaga boundary.

---

## 5. Keputusan Arsitektural #2: Strategi Async Processing & Message Broker

### 5.1 Estimasi volume async

Per hari (hotel 100 kamar):

- Email konfirmasi: ~50–100
- Sync ke PMS: ~50
- Webhook/invoice: ~100–200

**Total: ratusan pesan per hari.** Ini angka kunci yang menyaring semua pilihan broker.

### 5.2 Kandidat yang dievaluasi

| Tool | Throughput realistis | Kebutuhan infra | Verdict |
|---|---|---|---|
| **Kafka** | Jutaan msg/detik | Cluster, KRaft/ZooKeeper, tuning | ❌ Overkill ekstrem |
| **RabbitMQ** | Puluhan ribu msg/detik | 1 server + monitoring | ❌ Overkill |
| **NATS JetStream** | Jutaan msg/detik | 1 binary ~15MB, embeddable | ⚠️ Valid, tapi belum ada kebutuhan yang mewajibkannya |
| **Asynq + Redis/Valkey** | Ribuan job/detik | 1 instance Valkey | ⚠️ DX sangat bagus, tapi ada celah atomicity |
| **Postgres outbox (DIY)** | Ribuan job/detik | **0 infra baru** | ✅ Paling tepat sebagai fondasi |

> Analogi: memakai Kafka untuk 200 email/hari adalah seperti memakai truk kontainer untuk mengantar satu surat.

### 5.3 Pola terpilih: **Transactional Outbox di Postgres**

```sql
CREATE TABLE outbox (
  id            BIGSERIAL PRIMARY KEY,
  topic         TEXT NOT NULL,          -- 'email.confirmation', 'pms.sync'
  payload       JSONB NOT NULL,
  status        TEXT NOT NULL DEFAULT 'pending',  -- pending | done | failed
  attempts      INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbox_pending ON outbox (next_retry_at) WHERE status = 'pending';
```

Inti pola — **booking dan outbox masuk dalam satu transaksi database**:

```go
// dalam SATU transaksi:
// 1. INSERT INTO bookings (...)           -- booking dibuat
// 2. INSERT INTO outbox (topic, payload)  -- job email terdaftar
// COMMIT → tamu mendapat respon sukses
```

Worker (goroutine dalam proses Go yang sama):

```go
for {
    job := db.PollNextPendingOutbox() // FOR UPDATE SKIP LOCKED → aman multi-worker
    handle(job)                       // kirim email, sync PMS, dst.
    db.MarkDone(job.ID)               // atau retry dengan backoff eksponensial
}
```

#### Mengapa outbox lebih baik daripada broker pada skala ini

1. **Atomicity terkuat** — booking tersimpan tetapi email hilang adalah bug klasik jika email dikirim langsung di handler (crash antara commit dan enqueue). Dengan outbox, keduanya satu transaksi → tidak ada gap.
2. **Nol infra baru** — tidak ada server tambahan untuk di-install, di-monitor, di-backup, di-upgrade.
3. **Durability gratis** — pesan ikut ter-backup bersama database.
4. **Retry + dead-letter DIY ~50 baris** — `attempts`, `next_retry_at`, `status='failed'`.

### 5.4 Evaluasi spesifik: "NATS + asynq?" dan "Valkey + asynq?"

**Klarifikasi fundamental:** asynq adalah job queue **berbasis Redis** (bukan NATS). "NATS + asynq" berarti menjalankan dua infrastruktur (NATS + Redis) untuk pekerjaan yang tumpang tindih → ditolak. Keduanya adalah *kompetitor* untuk kebutuhan job-async, bukan pasangan pelengkap. Kombinasi NATS (event antar service) + asynq (job internal) baru bermakna pada sistem multi-service besar.

**"Valkey + asynq"** — diterima sebagai kombo yang valid dan modern (lihat §6): Valkey *menggantikan* Redis yang memang sudah dibutuhkan asynq, sehingga jumlah komponen tidak bertambah. Kompatibilitas terverifikasi via research:

- Valkey memakai protokol **RESP** (wire-compatible dengan Redis); asynq memakai `go-redis` di bawah kap → tanpa perubahan kode.
- asynq aktif dirawat: **v0.26.0 (Feb 2026)**, min. Go 1.24, status "relatively stable, moderate development".
- Valkey: Linux Foundation (didukung AWS/GCP), lisensi BSD-3-Clause — bebas dari keraguan lisensi Redis (v8 AGPLv3).
- Praktik komunitas "Asynq + Valkey" (2026) berjalan tanpa masalah.

**Keputusan akhir async:** fondasi = **transactional outbox Postgres**; eksekusi job = **asynq + Valkey** (dengan AOF). Sinyal untuk mengadopsi broker penuh (NATS/JetStream) di kemudian hari: dashboard staf real-time berskala besar, multi-hotel SaaS, atau kebutuhan event sourcing. Interfacenya dibuat tipis (`Publisher` 1 method) agar penukaran implementasi tidak menyentuh pemanggil.

---

## 6. Keputusan Arsitektural #3: Redis vs Valkey

### 6.1 Temuan research

| Aspek | Valkey 8 |
|---|---|
| Protokol | RESP — wire-compatible dengan Redis (semua client Go, termasuk asynq, langsung jalan) |
| Pengembangan | Linux Foundation; dukungan AWS, Google Cloud, Oracle, EQT |
| Lisensi | BSD-3-Clause (Redis 8: AGPLv3) |
| Performa | Setara atau lebih baik dari Redis 7.x pada benchmark publik |
| Ekosistem | asynq, asynqmon, go-redis terbukti dipakai bersama Valkey di produksi |

### 6.2 Peran ganda Valkey di stack ini (satu instance, dua tugas)

| Peran | Untuk apa | Contoh |
|---|---|---|
| Task queue (asynq) | Job async + retry + scheduled | Email konfirmasi, release-hold kamar |
| Cache (cache-aside) | Data read-heavy | Availability, hasil rate engine *(lihat §12.5 — diaktifkan kondisional)* |
| Rate limiter | Token bucket / sliding window | Proteksi endpoint search & cek promo |
| Pub/Sub (fase ≥2 instance) | Bus event antar instance | Live update dashboard |

### 6.3 Konfigurasi yang wajib (keamanan operasional)

```bash
valkey-server --appendonly yes
# TANPA --maxmemory-policy allkeys-lru  (biarkan default: noeviction)
```

Dua jebakan yang teridentifikasi saat satu instance dipakai ganda:

1. **Eviction policy dapat "memakan" task asynq.** Jika `maxmemory` + `allkeys-lru` diaktifkan, Valkey boleh membuang key apa pun saat penuh — termasuk **task asynq yang belum diproses = job hilang**. Catatan penting: memisahkan logical DB (`SELECT 0` vs `SELECT 1`) **tidak melindungi**, karena eviction berlaku per-instance, bukan per-DB. Solusi: jaga `noeviction` — memori terpakai hanya beberapa MB di skala ini.
2. **AOF menyala untuk semua key** termasuk cache yang sebenarnya rebuildable. Di volume hotel tunggal dampaknya negligible; AOF tetap wajib untuk melindungi task asynq.

Alternatif paranoid (dua instance: cache-only bebas-evict vs queue-only AOF) dievaluasi dan **ditunda** — satu instance + monitoring memori sudah memadai.

### 6.4 Docker Compose (dev)

```yaml
services:
  postgres:
    image: postgres:18
    environment:
      POSTGRES_DB: booking
      POSTGRES_PASSWORD: dev
    volumes: [pgdata:/var/lib/postgresql/data]
    ports: ["5432:5432"]

  valkey:
    image: valkey/valkey:8
    command: valkey-server --appendonly yes   # persistence WAJIB untuk task asynq
    volumes: [valkeydata:/data]
    ports: ["6379:6379"]

  app:
    build: .
    depends_on: [postgres, valkey]
    ports: ["8080:8080"]

  asynqmon:
    image: hibiken/asynqmon
    ports: ["8081:8080"]
    environment:
      REDIS_ADDR: valkey:6379

volumes: { pgdata: {}, valkeydata: {} }
```

---

## 7. Keputusan Arsitektural #4: Real-Time — Pub/Sub, SSE, WebSocket

### 7.1 Luruskan konsep (sering tertukar)

- **SSE/WebSocket** = *transport ke browser* (bagaimana data sampai ke client).
- **Pub/Sub** = *distribusi di backend* (bagaimana event dari satu komponen sampai ke semua koneksi client).

Keduanya saling melengkapi, bukan alternatif:

```
Worker outbox ──publish──► [Valkey Pub/Sub] ──subscribe──► [Hub in-memory di Go]
                                                                │
                                          ┌─────────────────────┼──────────────┐
                                          ▼                     ▼              ▼
                                     [SSE client 1]      [SSE client 2]  [SSE client N]
                                     (dashboard front desk, dst.)
```

### 7.2 Analisis kebutuhan

Kasus nyata: dashboard front desk yang otomatis update saat booking baru/kamar terjual. Skala: **5–50 koneksi concurrent** — satu proses Go menangani dengan sangat santai (tiap koneksi = 1 goroutine; Go kuat ratusan ribu koneksi).

| Fase | Kondisi | Solusi |
|---|---|---|
| Sekarang (1 instance) | Dashboard kecil | **In-memory hub + SSE**, nol infra baru |
| Nanti (2 instance di balik LB) | Client di instance A, event dipublish dari instance B | **Valkey Pub/Sub** sebagai bus antar instance — infra tetap sama |
| Jauh nanti (multi-service) | — | Baru pertimbangkan NATS; kemungkinan tidak pernah |

### 7.3 Implementasi SSE (pola dasar)

```go
func (s *Server) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // penting di belakang nginx

	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
```

Bridge Valkey → hub (aktif saat multi-instance):

```go
sub := s.rdb.Subscribe(ctx, "events:booking", "events:availability")
go func() {
	for msg := range sub.Channel() {
		s.hub.Publish([]byte(msg.Payload))
	}
}()
```

Publish dari relay outbox (setelah job sukses diproses) — outbox tetap *source of truth*:

```go
s.rdb.Publish(ctx, "events:availability", payload)
```

Browser: `new EventSource("/api/events")` — **auto-reconnect bawaan**.

### 7.4 Dua keputusan penting real-time

1. **Valkey Pub/Sub itu fire-and-forget** (tanpa persistence/replay). Pesan yang terlewat saat client disconnect *hilang*. Solusi bukan ganti broker, melainkan pola **snapshot-on-connect**:
   > Saat dashboard dibuka → client fetch availability via REST (snapshot); SSE hanya mengirim *delta* berikutnya. Reconnect = refetch snapshot. Data yang wajib sampai (email, sync PMS) tetap lewat outbox; Pub/Sub hanya "bunyi bel", bukan kurir.
2. **SSE vs WebSocket:** untuk kebutuhan saat ini (dashboard, notifikasi satu arah), **SSE hampir pasti cukup** — reconnect bawaan, setup proxy sederhana. WebSocket (`coder/websocket` / `gorilla/websocket`) baru dipertimbangkan jika muncul fitur dua arah (mis. chat tamu ↔ resepsionis). Transport bisa ditukar tanpa mengubah arsitektur hub/pub-sub.

---

## 8. Keputusan Arsitektural #5: Hexagonal Architecture (Ports & Adapters)

### 8.1 Mengapa cocok

Semakin banyak integrasi 3rd party (payment gateway, email, PMS, channel manager), semakin terasa manfaatnya. Hexagonal menjawab persis masalah ini dengan satu aturan:

> **Panah dependensi selalu mengarah ke dalam (domain core).** Core tidak pernah tahu Midtrans/Stripe/Resend/opera. Core hanya mengenal *port* — interface yang menggambarkan **kebutuhannya sendiri**, bukan meniru bentuk API vendor.

```
                    ┌───────────────────────────────┐
  driving (masuk)   │        DOMAIN CORE            │   driven (keluar)
                    │  booking · inventory · rates  │
  HTTP handlers ──► │                               │ ──► repos Postgres
  asynq workers ──► │   mendefinisikan PORT-nya:    │ ──► PaymentGateway
  CLI              │   PaymentGateway, Notifier,   │ ──► Notifier (email)
                    │   AvailabilityStore, ...      │ ──► PMS sync
                    └───────────────────────────────┘ ──► Valkey cache
```

Keputusan vendor dipusatkan di **composition root** (`main.go`). Ganti Midtrans → Stripe = tulis 1 adapter baru + ubah 1 baris wiring; core tidak tersentuh.

### 8.2 Penerapan idiomatic Go

Prinsip kunci Go: **definisikan interface di sisi consumer, bukan implementor.**

```go
// internal/booking/service.go — PORT didefinisikan oleh domain yang butuh
package booking

type PaymentGateway interface {
	CreateCharge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
	VerifyWebhook(payload []byte, sig string) (PaymentEvent, error)
}

type Notifier interface {
	SendBookingConfirmed(ctx context.Context, b Booking) error
}

type Service struct {
	repo    Repository
	payment PaymentGateway // tergantung interface, bukan struct vendor
	notify  Notifier
	events  EventPublisher // ← outbox, juga port
}
```

Adapter berperan sebagai **anti-corruption layer** — bentuk API vendor diterjemahkan penuh di dalam file adapter dan **tidak boleh bocor keluar**:

```go
// internal/adapter/payment/midtrans/gateway.go
func (g *Gateway) CreateCharge(ctx context.Context, req booking.ChargeRequest) (booking.ChargeResult, error) {
	body := g.toMidtransPayload(req) // translate: domain shape → API Midtrans
	resp, err := g.client.Charge(ctx, body)
	if err != nil {
		return booking.ChargeResult{}, fmt.Errorf("midtrans charge: %w", err)
	}
	return booking.ChargeResult{Token: resp.Token, RedirectURL: resp.RedirectURL}, nil
}

var _ booking.PaymentGateway = (*Gateway)(nil) // compile-time check
```

Wiring di composition root:

```go
// cmd/server/main.go
var gw booking.PaymentGateway
switch cfg.PaymentProvider {
case "midtrans":
	gw = midtrans.New(cfg.Midtrans)
case "xendit":
	gw = xendit.New(cfg.Xendit)
}
svc := booking.NewService(repo, gw, resend.New(cfg), outbox)
```

Go structural typing membuat semua ini tanpa framework DI — wiring manual di `main.go` sudah elegan pada skala ini.

### 8.3 Pemetaan port → adapter (sekarang vs masa depan)

| Port | Adapter sekarang | Ganti nanti menjadi |
|---|---|---|
| `EventPublisher` | outbox Postgres (+ asynq relay) | outbox → NATS relay |
| `PaymentGateway` | Midtrans (atau Xendit/Stripe) | Vendor lain — tulis 1 adapter |
| `Notifier` | SMTP / Resend | SES / vendor lain |
| `PMSClient` | stub / sync manual | Integrasi PMS hotel |
| `CacheStore` / `RateLimiter` | Valkey | Apa pun yang RESP-compatible |

### 8.4 Dua jebakan yang dihindari

1. **Jangan over-abstract "agar siap semua kemungkinan".** Port menggambarkan kebutuhan *yang ada sekarang*; interface 20 method = tanda desain salah. 1–3 method per port itu sehat.
2. **Port ≠ wrapper API vendor.** Jika `PaymentGateway` memiliki method `SnapCreateTransactionWithQris()` (nama fitur Midtrans), abstraction bocor. Method harus berbahasa domain: `CreateCharge()`.

### 8.5 Bonus yang langsung terasa

- **Testing:** domain di-unit-test dengan *fake* sederhana (`fakeGateway{}`, `spyNotifier`) — cepat, deterministik, tanpa network, tanpa mock framework berat.
- **Contract test per adapter:** satu test suite yang menguji kontrak `PaymentGateway` dijalankan terhadap *setiap* adapter → ganti vendor tidak menurunkan kualitas.
- **Onboarding:** dev baru memahami batas sistem cukup dengan membaca definisi port.

---

## 9. Final Tech Stack

Divalidasi dengan research terhadap rilis terkini (per Oktober 2026):

| Layer | Pilihan | Alasan & bukti research |
|---|---|---|
| Bahasa | **Go 1.27** (min 1.27.1, rilis Agu 2026) | Generic methods; `encoding/json/v2` (JSON API jauh lebih cepat); package `uuid` bawaan stdlib; `go fix` modernizer |
| Arsitektur | Modular monolith + hexagonal | §4 & §8 |
| Database | **PostgreSQL 18** | Source of truth + outbox + hold. **UUIDv7 native** (`uuidv7()`) = ID time-sortable & index-friendly; **async I/O** (2–3× sequential scan) mempercepat query range tanggal; **virtual generated columns** (mis. `total_price`); statistik planner survive restart |
| Driver DB | `pgx/v5` | Standar de-facto Go + Postgres |
| Migrasi | `golang-migrate` / `goose` | Versioned schema, jalan di CI |
| Job queue | **asynq v0.26 + Valkey 8 (AOF on)** | §5–6; asynqmon untuk UI monitoring |
| Cache | Valkey (instance sama, `noeviction`) | Kondisional — lihat §12.5 |
| Event bus | Valkey Pub/Sub (aktif saat ≥2 instance) | §7 |
| Real-time | SSE + in-memory hub | §7; WebSocket hanya jika ada fitur dua arah |
| HTTP | `chi` atau stdlib `net/http` | Ringan, idiomatic |
| Payment | Midtrans/Stripe/Xendit (hosted/redirect) | PCI tetap di gateway; webhook idempotent |
| Deploy (awal) | 1 VPS: app + Postgres + Valkey | Naik ke 2 instance + LB hanya saat perlu |

**Prinsip final:** stack modern penuh, tetapi tidak ada satu pun komponen "hiasan" — semuanya punya tugas konkret.

---

## 10. System Design: Kapasitas & Estimasi (Back-of-Envelope)

Asumsi: hotel 100 kamar, 5 tipe kamar, booking window 365 hari, musim ramai.

| Metrik | Perhitungan | Hasil |
|---|---|---|
| Booking/hari | — | 20–50 (puncak musiman) |
| Availability QPS | puncak ekstrem | ~50 QPS (< 500 QPS) |
| Baris `inventory` | 5 tipe × 365 hari | **~1.825 baris** |
| Baris `bookings` | 50/hari × 365 | ~18.000/tahun |
| Job async/hari | email + PMS + webhook | ratusan |
| Koneksi SSE concurrent | dashboard staf | 5–50 |

**Kesimpulan fundamental:** skala ini **tidak memaksa keputusan arsitektur mana pun berdasarkan throughput.** Semua keputusan desain dibangun di atas *korektness* (konsistensi, idempotency, durability), bukan performa. Ini menjelaskan mengapa banyak komponen "skala besar" (Kafka, sharding, cache layer, multi-region) sengaja ditolak.

---

## 11. System Design: High-Level Architecture

```
                              ┌─────────────────────────────────────────┐
  Tamu / Dashboard ──► DNS/LB │            Go App (1 binary)            │
                              │                                         │
                              │  HTTP: search · checkout · webhook      │
                              │  SSE hub (dashboard front desk)         │
                              │  asynq server (workers)                 │
                              │  outbox relay (goroutine)               │
                              └───────┬──────────────────────┬──────────┘
                                      │                      │
                        ┌─────────────▼────────┐   ┌─────────▼─────────┐
                        │   PostgreSQL 18      │   │   Valkey 8 (AOF)  │
                        │  - bookings          │   │  - asynq tasks    │
                        │  - inventory         │   │  - cache (opt.)   │
                        │  - holds             │   │  - rate limit     │
                        │  - room_assignments  │   │  - pub/sub        │
                        │  - rates/promos      │   └───────────────────┘
                        │  - outbox            │
                        └──────────┬───────────┘
                                   │ (adapter, via ports)
              ┌────────────────────┼─────────────────────┐
              ▼                    ▼                     ▼
      [Payment Gateway]     [Email provider]      [PMS / Channel Mgr]
      (Midtrans/Stripe)     (SMTP/Resend)         (stub → integrasi)
```

Karakteristik:

- **Satu binary Go** berisi HTTP server, SSE hub, asynq worker, dan outbox relay — dibagi paket modular dengan boundary tegas.
- **Postgres = satu-satunya source of truth.** Valkey hanya menyimpan data yang boleh hilang *kecuali* task asynq (dilindungi AOF).
- Semua interaksi vendor lewat **port/adapter** (§8).

---

## 12. System Design: Deep Dive — Masalah Double-Booking

Ini masalah inti seluruh booking engine. Research mengidentifikasi enam pendekatan umum; berikut analisis masing-masing terhadap konteks 1 hotel.

### 12.1 Enam pendekatan & evaluasinya

| # | Pendekatan | Cara kerja | Verdict | Alasan |
|---|---|---|---|---|
| 1 | **Row lock + status flag** (`SELECT ... FOR UPDATE`) | Tx: lock baris → cek status → update → commit | ✅ **Pakai** | Single DB + trafik rendah = biaya lock trivial; konsistensi terkuat; tanpa dependensi baru |
| 2 | **Lock dengan expiry (hold + TTL)** | Status `HELD` dengan TTL; konfirmasi sebelum timeout; kadaluarsa = lepas otomatis | ✅ **Pakai** | Wajib karena flow multi-step (checkout → bayar); mencegah deadlock akibat crash/sesi ditinggal |
| 3 | **Atomic conditional UPDATE** | `UPDATE ... SET status='BOOKED' WHERE ... AND status='AVAILABLE'` | ✅ Alternatif | Cepat & sederhana; cocok untuk inventori berbasis counter |
| 4 | Distributed lock (Redis SETNX) | Lock terdistribusi lintas instance | ❌ Tolak | Untuk multi-instance/multi-DB; di sini hanya menambah *mode gagal* baru tanpa manfaat |
| 5 | Queue-based serialization | Semua request booking diantri & diproses berurutan | ❌ Tolak | Menambah latency di user path; tidak perlu pada 50 QPS |
| 6 | **Payment-as-truth + auto-release** | Booking *tentative* sampai pembayaran sukses; gagal/timeout → kamar dilepas otomatis | ✅ **Adopsi prinsipnya** | Mengurangi *ghost booking*; menyelaraskan desain dengan realitas bisnis |

**Keputusan: kombinasi 1 + 2 + 6** — satu transaksi database: lock baris inventory rentang tanggal (terurut deterministik) → verifikasi → insert booking `pending` + hold + outbox event → commit. Release otomatis via asynq scheduled task. Pola ini sejalan dengan praktik industri (sistem tiket & travel skala besar) dan bab *hotel reservation system* pada literatur system design standar (Alex Xu).

### 12.2 Booking state machine (harus eksplisit di kode)

```
pending ──(webhook sukses)──► confirmed ──(check-in)──► checked_in ──► checked_out
   │                              │
   ├──(timeout 30m)──► expired    ├──(cancel)──► cancelled
   └──(payment fail)─► failed     └──(no-show)─► no_show
```

Transisi hanya boleh terjadi melalui fungsi state machine terpusat (bukan ad-hoc `UPDATE` di handler).

### 12.3 Critical path: transaksi create-booking

```
POST /bookings
  BEGIN
    SELECT ... FROM inventory
      WHERE room_type_id = $1 AND date >= $2 AND date < $3
      ORDER BY date ASC            -- urutan deterministik = anti-deadlock
      FOR UPDATE                    -- lock baris rentang malam
    -- verifikasi semua available_rooms >= jumlah_kamar
    UPDATE inventory SET available_rooms = available_rooms - n, version = version + 1
    INSERT bookings (status='pending') + holds (expires_at = now() + 30m)
    INSERT outbox (event: booking.created)
  COMMIT
  → enqueue asynq: release-hold pada t+30m (scheduled task)
```

Catatan kritis: **lock baris selalu terurut ASC** untuk mencegah deadlock ketika dua transaksi mengunci rentang yang saling overlap dari urutan berbeda.

### 12.4 Idempotency webhook pembayaran

```sql
UPDATE bookings
SET status = 'confirmed'
WHERE id = $1 AND status = 'pending';   -- affected rows = 0 → duplikat/terlambat → balas 200
```

Gateway dapat mengirim webhook dua kali atau lebih; transisi state yang bersyarat membuat duplikat *harmless*.

### 12.5 Analisis cache: keputusan TIDAK memakai cache availability (untuk sekarang)

Ini temuan kontraintuitif yang penting. Dengan **1.825 baris inventory** yang seluruhnya muat di RAM Postgres (§10), query availability berjalan **sub-milidetik**. Memasang cache-aside + invalidation di path ini justru:

- Menambah *state yang bisa stale* di titik yang paling tidak boleh stale (availability),
- Menambah kode invalidation yang wajib benar pada setiap mutasi (booking, hold, blokir kamar, cancel),
- Memberi manfaat performa yang tak terukur pada skala ini.

**Keputusan:** cache availability **tidak diaktifkan** di fase awal; hanya rate-limiter (Valkey) yang aktif di path publik. Cache diaktifkan bila: (a) muncul fitur pencarian multi-hotel, atau (b) trafik naik ~100×. Pola cache-aside + invalidasi manual tetap didokumentasikan (§6.2) agar siap saat dibutuhkan. Ini penerapan konsisten dari prinsip *avoid premature optimization* yang dipakai sejak awal diskusi.

### 12.6 Matriks kegagalan (failure mode → penanganan)

| Kegagalan | Penanganan |
|---|---|
| App crash setelah commit DB, sebelum apa pun | Aman — seluruh state ada di DB; worker memproses outbox saat restart |
| Webhook pembayaran datang dua kali | Idempotency via transisi state bersyarat (§12.4) |
| Tamu membayar setelah hold expired | Refund manual via admin flow; jarang — hold 30 menit memadai |
| Email gagal terkirim | Outbox retry + backoff eksponensial; tidak mempengaruhi booking |
| Deadlock lock rentang tanggal | Urutan lock deterministik ASC (§12.3) |
| Valkey restart | Task asynq selamat berkat AOF; data cache boleh hilang (rebuildable) |
| Postgres down | Seluruh sistem down (diterima untuk 1 hotel); mitigasi: backup + prosedur restore terlatih |

---

## 13. System Design: Deep Dive — Schema Database

Research membandingkan dua kubu pemodelan inventory di industri:

- **Schema A — row-per-day, kamar *fungible*:** satu baris per `(room_type, date)` berisi counter; kamar 101 & 102 tipe sama dianggap identik; B-tree index; optimistic locking (`version`).
- **Schema B — per-room, kamar *non-fungible*:** satu baris per kamar fisik dengan `DATERANGE` + **exclusion constraint** (`EXCLUDE USING GIST`) yang memblok overlap di level database.

### 13.1 Keputusan: **hybrid** — Schema A untuk booking, Schema B untuk check-in

```sql
-- ============ SCHEMA A: inventory per tipe per malam (booking di sini) ============
CREATE TABLE room_types (
  id           UUID PRIMARY KEY DEFAULT uuidv7(),
  name         TEXT NOT NULL,
  max_capacity INT  NOT NULL
);

CREATE TABLE inventory (
  room_type_id    UUID NOT NULL REFERENCES room_types(id),
  date            DATE NOT NULL,
  total_rooms     INT  NOT NULL,
  available_rooms INT  NOT NULL,
  version         INT  NOT NULL DEFAULT 0,
  PRIMARY KEY (room_type_id, date),
  CHECK (available_rooms >= 0 AND available_rooms <= total_rooms)
);
-- partial index: query tersering hanya peduli baris yang masih tersisa
CREATE INDEX idx_inv_available ON inventory (room_type_id, date)
  WHERE available_rooms > 0;

CREATE TABLE bookings (
  id           UUID PRIMARY KEY DEFAULT uuidv7(),
  room_type_id UUID NOT NULL REFERENCES room_types(id),
  check_in     DATE NOT NULL,
  check_out    DATE NOT NULL,          -- half-open [check_in, check_out): malam terakhir tidak dijual dua kali
  num_rooms    INT  NOT NULL,
  num_guests   INT  NOT NULL,
  status       TEXT NOT NULL DEFAULT 'pending',  -- pending|confirmed|checked_in|checked_out|cancelled|expired|failed|no_show
  total_price_minor BIGINT NOT NULL,
  currency     CHAR(3) NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (check_out > check_in)
);

CREATE TABLE reservation_room_nights (   -- pricing per malam (weekday/weekend/promo berbeda)
  booking_id  UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
  night_date  DATE NOT NULL,
  rate_minor  BIGINT NOT NULL,
  PRIMARY KEY (booking_id, night_date)
);

CREATE TABLE holds (                     -- hold kamar dengan TTL
  id         UUID PRIMARY KEY DEFAULT uuidv7(),
  booking_id UUID NOT NULL REFERENCES bookings(id),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_holds_expiry ON holds (expires_at);

-- ============ SCHEMA B: assignment kamar fisik saat check-in (jaminan DB) ============
CREATE TABLE rooms (
  room_number  TEXT PRIMARY KEY,
  room_type_id UUID NOT NULL REFERENCES room_types(id)
);

CREATE TABLE room_assignments (
  assignment_id UUID PRIMARY KEY DEFAULT uuidv7(),
  booking_id    UUID NOT NULL REFERENCES bookings(id),
  room_number   TEXT NOT NULL REFERENCES rooms(room_number),
  stay_dates    DATERANGE NOT NULL,      -- [check_in, check_out)
  assigned_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- GiST exclusion constraint: DATABASE menolak dua tamu di kamar yang sama
  -- untuk tanggal yang tumpang tindih — bukan mengandalkan kode aplikasi
  EXCLUDE USING GIST (room_number WITH =, stay_dates WITH &&)
);

-- ============ OUTBOX ============
CREATE TABLE outbox (
  id            BIGSERIAL PRIMARY KEY,
  topic         TEXT NOT NULL,
  payload       JSONB NOT NULL,
  status        TEXT NOT NULL DEFAULT 'pending',
  attempts      INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbox_pending ON outbox (next_retry_at) WHERE status = 'pending';
```

### 13.2 Justifikasi hybrid

1. **Booking butuh cepat & sederhana** → tamu tidak peduli nomor kamar sebelum check-in; row-per-day memberi point-query O(log n) dan update per-malam yang independen. Dua booking yang overlap sebagian tanggal (mis. 15–17 vs 16–18) terhandle natural oleh counter per malam.
2. **Check-in butuh jaminan permanen** → setelah tamu menginap di 101, *tidak boleh ada* tamu lain di 101 pada malam yang sama. `EXCLUDE USING GIST` membuat **database menolak** pelanggaran — bukan mengandalkan kebenaran kode aplikasi.
3. **Pricing fleksibel per malam** via `reservation_room_nights`.
4. **Bonus Postgres 18:** `uuidv7()` memberi ID time-sortable (index-friendly, tanpa bloat seperti UUIDv4); async I/O mempercepat scan rentang tanggal; `version` untuk optimistic locking pada jalur tanpa lock eksplisit.

### 13.3 Konvensi tanggal: half-open interval

`[check_in, check_out)` — malam checkout **tidak** dihitung sebagai ketersediaan yang terpakai. Konsistensi konvensi ini krusial di seluruh query (`date >= check_in AND date < check_out`, `daterange(..., '[)')`).

---

## 14. System Design: Data Flow Lengkap

### 14.1 Flow booking sukses (happy path)

1. **Search** — `GET /availability?check_in=...&check_out=...&guests=2` → query `inventory` langsung (tanpa cache, §12.5) + rate engine menghitung harga per malam.
2. **Checkout** — `POST /bookings` → transaksi kritis §12.3 → respon berisi token pembayaran dari `PaymentGateway.CreateCharge` (port).
3. **Bayar** — tamu diarahkan ke hosted page gateway.
4. **Webhook** — `POST /webhooks/payment` → verifikasi signature (`VerifyWebhook`) → transisi state bersyarat (idempotent, §12.4) → insert outbox (`booking.confirmed`).
5. **Async (via outbox relay → asynq)** — email konfirmasi; live ping ke dashboard via Valkey Pub/Sub (jika diaktifkan); sync ke PMS.

### 14.2 Flow kegagalan

- Hold expired → asynq scheduled task menjalankan `release-hold` → `available_rooms += n`, status booking → `expired`.
- Payment gagal → status `failed`, hold dirilis, email "pembayaran tidak berhasil" via outbox.
- Cancel → transisi state + rilis inventory + email cancel.

### 14.3 Admin flow

- Blokir kamar (maintenance) → kurangi `total_rooms`/`available_rooms` pada rentang tanggal + invalidasi event.
- Check-in → transisi `confirmed → checked_in` + insert `room_assignments` (GiST menjamin tidak konflik).
- Check-out / no-show → transisi state terkait.

---

## 15. System Design: Non-Functional Requirements

| Aspek | Keputusan | Dasar |
|---|---|---|
| Konsistensi | Strong (single Postgres, transaksi lokal) | Tidak ada distributed transaction sama sekali |
| Availability target | 99.9% (≈ 8.77 jam downtime/tahun) | Downtime 1 jam di musim ramai = pendapatan hilang nyata |
| Durability | Backup Postgres harian (RPO ≤ 24 jam) + AOF Valkey | Data booking = uang; prosedur restore wajib diuji |
| Idempotency | Semua webhook & outbox consumer | *At-least-once delivery* di mana-mana → consumer harus idempotent |
| Keamanan | PCI di gateway; PII tamu terenkripsi at rest; TLS penuh; rate limit endpoint publik | Data kartu tidak pernah menyentuh sistem |
| Observability | `slog` structured logging; asynqmon; `/healthz` (liveness) + `/ready` (DB+Valkey) | Tiga sinyal minimum; tracing ditunda |
| Waktu & mata uang | Simpan UTC + `timestamptz`; ISO currency code | Tamu internasional tetap ada pada 1 hotel |
| Deployment awal | 1 VPS: app + Postgres + Valkey di balik reverse proxy | Skala naik: 2 instance + LB; Valkey Pub/Sub baru aktif di fase ini |

---

## 16. Tradeoffs & Skor Desain

### 16.1 Tradeoff yang diakui secara sadar

| Tradeoff | Diterima karena | Mitigasi |
|---|---|---|
| Row-per-day: nomor kamar tidak diketahui saat booking | Standar industri; booking lebih sederhana & cepat | Assignment kamar fisik saat check-in dengan jaminan GiST (§13) |
| Hold 30 menit: tamu bisa "kehilangan" harga setelah timeout | Mencegah ghost booking menahan inventory | Komunikasi jelas di UI + auto-release yang konsisten |
| Single Postgres = single point of failure | Skala 1 hotel; multi-region tidak proporsional | Backup harian + restore drill terlatih |
| Tidak ada cache availability | Query sudah sub-milidetik; cache menambah risiko stale | Aktifkan kondisional (§12.5) |
| Outbox DIY tanpa UI seperti asynqmon | Volume rendah; pola sederhana | Monitoring via query + metrik sederhana |

### 16.2 Skor desain: **8.5 / 10**

Gap menuju 10/10 (terdokumentasi sebagai backlog):

1. **Read replica / read scaling** — tidak dibutuhkan sekarang; tambahkan saat analytics mulai membebani primary.
2. **Metrik bisnis (observability level 2)** — booking conversion, abandonment rate checkout; murah untuk disiapkan sejak hari pertama.
3. **Rate limiting yang formal** — sudah didesain (token bucket Valkey), tinggal implementasi + threshold.

---

## 17. Roadmap Implementasi

| Fase | Deliverable |
|---|---|
| **0. Fondasi** | Scaffold repo modular monolith + hexagonal; docker-compose (PG 18 + Valkey 8 + asynqmon); migrasi SQL §13; CI (test + `go vet`) |
| **1. Inventory & Rates** | CRUD room type/rate; query availability; rate engine (base + promo) |
| **2. Booking core** | Transaksi create-booking (§12.3); state machine; holds + release-hold (asynq scheduled); outbox + relay |
| **3. Payment** | Port `PaymentGateway` + adapter vendor terpilih; webhook idempotent; contract test suite |
| **4. Notification** | Port `Notifier` + adapter email; template konfirmasi/cancel |
| **5. Front desk** | Check-in + `room_assignments` (GiST); check-out; cancel; dashboard admin |
| **6. Real-time & polish** | SSE hub (+ Valkey bridge bila ≥2 instance); rate limiter; metrik bisnis; restore drill |

---

## 18. Sumber & Referensi

**Research domain:**
- Identifikasi book-secure.com sebagai booking engine white-label: halaman resmi `m.book-secure.com`, praktik URL booking hotel resmi (Dorsett, Impiana KLCC, KIP Hotel, Tanza Oasis, Hotel ICON/WCFS 2024, Visa Offers Dorsett Hartamas), pembanding Similarweb (Sabre, Travelport, Vertical Booking, OnePageBooking), penilaian legitimasi ScamAdviser.
- Konsep Connectivity API industri: `developers.booking.com/connectivity/docs`.

**Research double-booking & concurrency:**
- *System Design Interview: The Double Booking Problem* — B. Dickman (Des 2025): enam pendekatan (row lock + status flag; lock with expiry/TTL; atomic conditional update; distributed lock Redis; queue-based serialization; payment-as-truth) beserta trade-off-nya. → menjadi dasar analisis §12.1.
- *Race Conditions in Hotel Booking Systems* — A. Roy (Feb 2026): pentingnya proteksi di level DB dan aplikasi.
- Analisis lock/overlap: O. Potapov (*exclusive row lock untuk mencegah overlapping reservation*), diskusi StackOverflow/DBA tentang pencegahan double booking pada schema hotel, *Solving Double Booking at Scale* (Reddit r/programming, 2025).

**Research schema database:**
- *Hotel Booking: Schema Design Comparison* — S. Bala (dev.to, Nov 2025): perbandingan lengkap **row-per-day fungible inventory (B-tree + optimistic locking)** vs **per-room GiST exclusion constraint**, termasuk pola *hybrid* (inventory fungible saat booking + room assignment saat check-in), tabel `reservation_room_nights` untuk pricing per malam, dan *check constraint* keseimbangan inventory. → menjadi dasar keputusan §13.
- *Hotel Reservation System* — Alex Xu, ByteByteGo: karakteristik read-heavy dan pemodelan inventory per (hotel, room type, date).

**Research tooling & rilis:**
- *Go 1.27 is released* — go.dev/blog (19 Agu 2026): generic methods, `encoding/json/v2`, package `uuid` stdlib, alokasi memori lebih cepat, goroutine leak profiles. Go 1.27.1 (1 Sep 2026).
- *PostgreSQL 18.0 Release Notes* — postgresql.org (25 Sep 2025): async I/O (sequential scan hingga 2–3× lebih cepat), UUIDv7 native, OAuth 2.0, virtual generated columns, statistik planner survive restart; analisis Crunchy Data, Xata, Severalnines.
- *hibiken/asynq* — GitHub/pkg.go.dev: v0.26.0 (3 Feb 2026), min. Go 1.24, status "relatively stable, moderate development"; fitur retry/backoff, scheduled/cron, prioritas, archive (dead-letter), asynqmon.
- Valkey: `valkey.io/topics/protocol` (RESP, kompatibilitas wire dengan Redis); blog Valkey Glide Go client (Mar 2025); praktik komunitas *Background Jobs in Go with Asynq and Valkey* (J. Goksu, Mar 2026).

---

*Dokumen ini menjadi acuan tunggal (source of truth) desain sebelum implementasi dimulai. Perubahan keputusan arsitektural setelah dokumen ini harus dicatat sebagai addendum dengan tanggal dan alasan.*
