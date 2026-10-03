# Technical Architecture — Production Provider Fail-Fast & Readiness Safety (BE-R16)

**Fitur:** Production Provider Fail-Fast, Safe Readiness Capability & Fake-Pay Hardening  
**Tanggal:** 2026-10-03  
**Status:** Approved for Implementation  
**Terkait:** [PRD](../prd/production-provider-failfast-r16-2026-10-03.md) · [SRS](../srs/production-provider-failfast-r16-2026-10-03.md) · BE-R16  

---

## 1. Diagram Alur Booting & Fail-Fast Guard

```mermaid
flowchart TD
    Start["Booting cmd/server"] --> LoadCfg["platform.LoadConfig()"]
    LoadCfg --> ValidateCfg{"cfg.Validate()"}
    
    ValidateCfg -- "Error (misal prod missing key)" --> Exit1["log.Error & os.Exit(1)"]
    ValidateCfg -- "Valid" --> CheckEnv{"cfg.IsProduction()"}
    
    CheckEnv -- "Yes (Production)" --> WireProd["Inisialisasi payment.NewXendit & notifier.NewResend"]
    WireProd --> GuardProd{"Validasi Instansiasi Provider"}
    GuardProd -- "Fake/Log terdeteksi" --> ExitPanic["log.Fatal & os.Exit(1)"]
    GuardProd -- "Provider Asli Aktif" --> AssembleRouter["NewRouter(d Deps)\nIsDevelopment=false\nFakePay=nil"]
    
    CheckEnv -- "No (Development/Test)" --> WireDev["Inisialisasi Xendit / Fake & Resend / Log"]
    WireDev --> AssembleRouterDev["NewRouter(d Deps)\nIsDevelopment=true\nFakePay=HardenedHandler"]
    
    AssembleRouter --> Listen["http.Server ListenAndServe"]
    AssembleRouterDev --> Listen
```

---

## 2. Diagram Alur Readiness & Capability Inquiry (`GET /ready`)

```mermaid
sequenceDiagram
    autonumber
    actor Client as Frontend / Monitoring Probe
    participant Router as Gin Engine (/ready)
    participant DepCheck as ReadyCheck (DB & Valkey Ping)

    Client->>Router: GET /ready
    Router->>DepCheck: Ping Postgres Pool & Valkey Client
    alt Database / Valkey Gagal
        DepCheck-->>Router: Error (Connection Refused)
        Router-->>Client: HTTP 503 Service Unavailable<br/>{"status":"unavailable","error":"..."}
    else Seluruh Dependensi Sehat
        DepCheck-->>Router: OK (Nil error)
        Router-->>Client: HTTP 200 OK<br/>{"status":"ready","environment":"production|development",<br/>"payment_gateway":"xendit|fake","notifier":"resend|log"}
    end
```

---

## 3. Komponen & Modifikasi Kode

### A. `internal/platform/config.go`
- Tambahkan validasi berbasis lingkungan pada `Validate()`:
  - Jika `c.IsProduction()`:
    - Cek `c.XenditSecretKey != ""`
    - Cek `c.XenditWebhookToken != ""`
    - Cek `c.ResendAPIKey != ""`
- Menambahkan method `PaymentGatewayType()` dan `NotifierType()` sebagai helper ringkas jika diperlukan.

### B. `internal/adapter/notifier/log.go`
- Tambahkan field `MaskOTP bool` pada `LogNotifier`.
- Pada `SendGuestOTP`:
  ```go
  otpText := otpCode
  if n.MaskOTP {
      otpText = "[REDACTED]"
  }
  ```

### C. `internal/api/router.go`
- Tambahkan field `NotifierMode string` pada struct `Deps`.
- Perbarui handler `ready(d Deps)` untuk menyertakan `environment`, `payment_gateway`, dan `notifier` pada response JSON 200 OK.

### D. `cmd/server/main.go`
- Penegakan proteksi fail-fast eksplisit saat runtime composition:
  - Jika `cfg.IsProduction()` dan key tidak ada, langsung batalkan proses.
  - Set `NotifierMode: "resend"` atau `"log"`.
- Hardening endpoint simulasi `FakePay`:
  - Validasi string `bookingID` menggunakan `uuid.Parse(bookingID)`.
  - Jika format tidak valid: kirimkan HTTP 400 Bad Request (`INVALID_BOOKING_ID`) dalam format JSON seragam (bukan 500 error Postgres mentah).
  - Tangani error domain secara terstruktur: `booking.ErrNotFound` $\rightarrow$ 404 `BOOKING_NOT_FOUND`, `booking.ErrHoldExpired` $\rightarrow$ 409 `HOLD_EXPIRED`.
  - Error lainnya dibungkus sebagai 500 internal error generik tanpa membocorkan SQL syntax error.

---

## 4. Evaluasi Anti-Overengineering (Ponytail Review)

- **Tidak ada library baru:** Menggunakan fungsi standar Go `uuid.Parse` yang sudah tersedia di proyek melalui Google UUID.
- **Fail-Fast di Titik Masuk:** Menggunakan `cfg.Validate()` yang sudah dipanggil saat `main()` pertama kali dieksekusi, tanpa menambahkan framework konfigurasi baru.
- **Minimal Abstractions:** Tidak membuat interface baru untuk health checking; cukup memperluas response JSON dari handler `ready()` yang sudah ada.
