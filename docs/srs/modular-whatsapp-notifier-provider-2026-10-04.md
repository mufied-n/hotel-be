# SRS: Modular WhatsApp Notifier Architecture & Multi-Provider Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Fitur ID:** `FEAT-MODULAR-WHATSAPP-NOTIFIER`
- **Tanggal:** 4 Oktober 2026
- **Status:** APPROVED / IN IMPLEMENTATION

---

## 1. Kebutuhan Fungsional (Functional Requirements)

* **FR-WA-01 (Unified Sender Interface):** Sistem wajib menyediakan interface tunggal:
  ```go
  type WhatsAppSender interface {
      SendBookingConfirmation(ctx context.Context, msg WhatsAppBookingMessage) error
  }
  ```
* **FR-WA-02 (Provider Factory & Dynamic Selection):** Sistem wajib menginisialisasi provider WhatsApp melalui fungsi factory `NewWhatsAppSender(cfg WhatsAppConfig, log *slog.Logger) (WhatsAppSender, error)` berdasarkan nilai konfigurasi `cfg.Provider`:
  - `"log"` atau `""` $\rightarrow$ `LogWhatsAppSender`
  - `"generic_http"` $\rightarrow$ `GenericGatewaySender`
  - `"twilio"` $\rightarrow$ `TwilioSender`
  - `"meta_cloud"` atau `"waba"` $\rightarrow$ `MetaCloudSender`
  - Nilai selain itu mengembalikan error `ErrUnsupportedProvider`.
* **FR-WA-03 (Twilio WhatsApp Protocol Adapter):**
  - Endpoint: `https://api.twilio.com/2010-04-01/Accounts/{AccountSID}/Messages.json` (atau kustom via `BaseURL`).
  - Autentikasi: HTTP Basic Auth dengan Username `AccountSID` dan Password `APIKey` (Auth Token).
  - Format Body: `application/x-www-form-urlencoded` memuat:
    - `To`: `whatsapp:<E.164-phone>` (misal `whatsapp:+6281234567890`)
    - `From`: `whatsapp:<FromPhone>`
    - `Body`: Teks pesan konfirmasi.
  - Validasi Status: HTTP 200 s.d. 299 dianggap berhasil; kode lainnya menghasilkan `ErrWhatsAppDeliveryFailed`.
* **FR-WA-04 (Meta WhatsApp Business Cloud API Protocol Adapter):**
  - Endpoint: `https://graph.facebook.com/v20.0/{PhoneNumberID}/messages` (atau kustom via `BaseURL`).
  - Autentikasi: HTTP Header `Authorization: Bearer <APIKey>`.
  - Format Body: `application/json` memuat struktur Meta Cloud Graph API:
    ```json
    {
      "messaging_product": "whatsapp",
      "recipient_type": "individual",
      "to": "6281234567890",
      "type": "text",
      "text": {
        "preview_url": true,
        "body": "<formatted-message-text>"
      }
    }
    ```
  - Validasi Status: HTTP 200 s.d. 299 dianggap berhasil.
* **FR-WA-05 (Generic External Gateway Protocol Adapter):**
  - Mendukung gateway populer Indonesia (Fonnte, Wablas, Waha, Qontak).
  - Endpoint: Diarahkan ke `BaseURL` (default: `https://api.fonnte.com/send`).
  - Header: `Authorization: <APIKey>`.
  - Body: `{"target": "<phone>", "message": "<formatted-message-text>"}`.
* **FR-WA-06 (Phone Normalization Engine):**
  - Menghapus karakter spasi, strip, tanda kurung.
  - Jika diawali `08...`, diubah menjadi `628...` (dan `+628...` untuk Twilio).
  - Mengembalikan `ErrInvalidPhoneNumber` jika nomor penerima kosong atau tidak valid.

---

## 2. Definisi Parameter Konfigurasi Lingkungan (*Environment Variables*)

| Variabel Lingkungan | Deskripsi | Contoh Nilai | Wajib Untuk |
| :--- | :--- | :--- | :--- |
| `WHATSAPP_PROVIDER` | Pilihan engine penyedia WhatsApp | `log`, `generic_http`, `twilio`, `meta_cloud` | Opsional (default: `log`) |
| `WHATSAPP_API_KEY` | Token API / Auth Token / Secret | `WA_TOKEN_xyz123...` | `generic_http`, `twilio`, `meta_cloud` |
| `WHATSAPP_BASE_URL` | Override Base URL endpoint HTTP | `https://api.twilio.com` atau mock server | Opsional |
| `WHATSAPP_ACCOUNT_SID` | Twilio Account SID | `AC1234567890abcdef...` | `twilio` |
| `WHATSAPP_PHONE_NUMBER_ID` | Meta WhatsApp Cloud Phone Number ID | `1002938481239` | `meta_cloud` |
| `WHATSAPP_FROM_PHONE` | Nomor pengirim resmi hotel | `+14155238886` atau `+62811234567` | `twilio` |

---

## 3. Spesifikasi Error

| Kode Error / Objek | Kondisi Pemicu | Sifat Penanganan |
| :--- | :--- | :--- |
| `ErrInvalidPhoneNumber` | Nomor telepon kosong atau format salah | Non-retryable (`msg.Term()`) |
| `ErrUnsupportedProvider` | Nilai `WHATSAPP_PROVIDER` tidak dikenal | Startup fail-fast |
| `ErrMissingConfiguration` | Parameter wajib untuk provider terpilih belum diisi | Startup fail-fast |
| `ErrWhatsAppDeliveryFailed` | HTTP response $\ge 400$, network timeout, gateway offline | Retryable (`msg.NakWithDelay()`) |
