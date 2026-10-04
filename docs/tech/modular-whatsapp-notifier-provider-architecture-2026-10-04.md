# Technical Architecture: Modular WhatsApp Notifier & Multi-Provider Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Fitur ID:** `FEAT-MODULAR-WHATSAPP-NOTIFIER`
- **Tanggal:** 4 Oktober 2026
- **Status:** APPROVED / IN IMPLEMENTATION

---

## 1. Diagram Arsitektur Hexagonal & Adapter Pattern

```mermaid
flowchart TD
    subgraph Domain ["Core Domain & Outbox Workers"]
        BOOKING["internal/booking\nService"]
        OUTBOX["internal/workers\nOutbox / NATS Consumer"]
    end

    subgraph Port ["Notifier Port (Interface)"]
        SENDER["notifier.WhatsAppSender\n(SendBookingConfirmation)"]
    end

    subgraph Factory ["Provider Factory"]
        FACTORY["NewWhatsAppSender(cfg, log)"]
    end

    subgraph Adapters ["Pluggable Notifier Adapters (Standard Library net/http)"]
        LOG_ADAPTER["LogWhatsAppSender\n(slog JSON & History)"]
        GATEWAY_ADAPTER["GenericGatewaySender\n(Fonnte / Wablas / Waha)"]
        TWILIO_ADAPTER["TwilioSender\n(Basic Auth + Form Urlencoded)"]
        META_ADAPTER["MetaCloudSender\n(Bearer Token + Graph API JSON)"]
    end

    subgraph External ["External WhatsApp Platforms"]
        EXT_LOG["Local / CI Terminal Output"]
        EXT_GW["Fonnte / Wablas / Waha API"]
        EXT_TWILIO["Twilio API (api.twilio.com)"]
        EXT_META["Meta Graph API (graph.facebook.com)"]
    end

    BOOKING -.-> SENDER
    OUTBOX -.-> SENDER
    FACTORY --> SENDER
    FACTORY --> LOG_ADAPTER
    FACTORY --> GATEWAY_ADAPTER
    FACTORY --> TWILIO_ADAPTER
    FACTORY --> META_ADAPTER

    LOG_ADAPTER --> EXT_LOG
    GATEWAY_ADAPTER --> EXT_GW
    TWILIO_ADAPTER --> EXT_TWILIO
    META_ADAPTER --> EXT_META
```

---

## 2. Struktur Data Konfigurasi & Factory

```go
type WhatsAppConfig struct {
    Provider      string        // "log", "generic_http", "twilio", "meta_cloud"
    BaseURL       string        // Custom URL jika ada
    APIKey        string        // Auth Token / Secret Key / Bearer Token
    AccountSID    string        // Twilio Account SID
    PhoneNumberID string        // Meta Cloud Phone Number ID
    FromPhone     string        // Twilio Sender Phone (e.g., +14155238886)
    Timeout       time.Duration // HTTP client timeout (default 10s)
}
```

### Factory Logic:
```go
func NewWhatsAppSender(cfg WhatsAppConfig, log *slog.Logger) (WhatsAppSender, error) {
    if log == nil {
        log = slog.Default()
    }
    if cfg.Timeout <= 0 {
        cfg.Timeout = 10 * time.Second
    }

    switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
    case "", "log", "mock":
        return NewLogWhatsApp(log), nil
    case "generic_http", "gateway", "fonnte", "wablas":
        return NewGenericGatewaySender(cfg, log)
    case "twilio":
        return NewTwilioSender(cfg, log)
    case "meta_cloud", "waba", "meta":
        return NewMetaCloudSender(cfg, log)
    default:
        return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, cfg.Provider)
    }
}
```

---

## 3. Detail Implementasi Adapter Provider

### 3.1 Normalisasi Nomor Telepon (`NormalizePhone`)
```go
// Menghilangkan spasi, tanda kurung, strip.
// Menjamin awalan kode negara Indonesia 62 (atau format E.164 +62).
func NormalizePhoneE164(phone string) string {
    digits := extractDigits(phone)
    if strings.HasPrefix(digits, "0") {
        digits = "62" + digits[1:]
    }
    return "+" + digits
}

func NormalizePhoneDigitsOnly(phone string) string {
    digits := extractDigits(phone)
    if strings.HasPrefix(digits, "0") {
        digits = "62" + digits[1:]
    }
    return digits
}
```

### 3.2 Twilio Adapter (`TwilioSender`)
- **Protokol:** POST form-urlencoded
- **Header:**
  - `Authorization: Basic base64(AccountSID + ":" + APIKey)`
  - `Content-Type: application/x-www-form-urlencoded`
- **Body Data:**
  - `From=whatsapp:<FromPhone>`
  - `To=whatsapp:<ToPhoneE164>`
  - `Body=<Text>`
- **HTTP Client:** Dialokasikan dengan timeout `cfg.Timeout`.

### 3.3 Meta WhatsApp Business Cloud API (`MetaCloudSender`)
- **Protokol:** POST JSON ke `https://graph.facebook.com/v20.0/{PhoneNumberID}/messages`
- **Header:**
  - `Authorization: Bearer <APIKey>`
  - `Content-Type: application/json`
- **Body Data:**
  ```json
  {
    "messaging_product": "whatsapp",
    "recipient_type": "individual",
    "to": "6281234567890",
    "type": "text",
    "text": {
      "preview_url": true,
      "body": "..."
    }
  }
  ```

### 3.4 Generic External Gateway (`GenericGatewaySender`)
- **Protokol:** POST JSON ke `BaseURL` (misal Fonnte / Wablas)
- **Header:**
  - `Authorization: <APIKey>`
  - `Content-Type: application/json`
- **Body Data:**
  ```json
  {
    "target": "081234567890",
    "message": "..."
  }
  ```

---

## 4. Analisis Anti-Overengineering (Ponytail Verification)

1. **Zero External SDK Dependencies:** Tidak memerlukan library pihak ketiga seperti `twilio-go` atau `facebook-go-sdk` yang berbobot puluhan megabyte dan ratusan transitive dependencies. Seluruh protokol REST HTTP diselesaikan secara ringkas dengan standard library `net/http` dan `encoding/json`.
2. **Backward Compatibility:** `LogWhatsAppSender` dan `HTTPWhatsAppSender` tetap dapat dipanggil secara langsung oleh kode eksisting tanpa *breaking change*.
3. **Fail-Fast Configuration:** Jika provider `"twilio"` dipilih namun `WHATSAPP_ACCOUNT_SID` kosong, server menolak menyala saat startup (*fail-fast*), mencegah error tersembunyi di masa produksi.
