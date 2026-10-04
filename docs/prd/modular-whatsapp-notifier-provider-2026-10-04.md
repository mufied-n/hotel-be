# PRD: Modular WhatsApp Notifier Architecture & Multi-Provider Engine
**Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)**

- **Fitur ID:** `FEAT-MODULAR-WHATSAPP-NOTIFIER`
- **Tanggal:** 4 Oktober 2026
- **Status:** APPROVED / IN IMPLEMENTATION
- **Target Pengguna:** Tamu Publik (`guest`), Tim Reservasi / Meja Depan (`receptionist`), Tim IT / DevOps Hotel

---

## 1. Latar Belakang & Konteks Bisnis

Pulang ke Uttara adalah hotel butik bintang 4 (95 kamar) di Sleman, Yogyakarta. Mayoritas tamu domestik di Indonesia sangat mengandalkan aplikasi WhatsApp untuk menerima bukti reservasi instan, petunjuk check-in, dan e-voucher digital.

Dalam operasional hotel modern, kebutuhan integrasi WhatsApp sering kali berevolusi seiring skala dan biaya:
1. **Fase Awal / Low-Cost Gateway:** Hotel menggunakan penyedia gateway pihak ketiga lokal/regional (seperti Fonnte, Wablas, Waha, atau Qontak) yang berbiaya terjangkau dan mudah dikonfigurasi.
2. **Fase Skala Enterprise / Korporasi:** Hotel berpindah ke Twilio Programmable Messaging API untuk jaminan SLA tinggi, deliverability global, dan enkripsi perbankan.
3. **Fase WhatsApp Official Business Platform (Meta Cloud API / WABA):** Hotel mendaftarkan nomor resmi bercentang hijau (*Official Business Account*) langsung ke Meta Graph API dengan biaya per percakapan (*conversation-based pricing*).
4. **Fase Local Dev & Automated Testing:** Diperlukan in-memory / structured log mock agar pengembang dan pipeline CI/CD tidak mengirim pesan nyata atau mengeluarkan biaya API eksternal.

Jika kode aplikasi mengikat (*hardcode*) salah satu SDK atau format HTTP API vendor tertentu, setiap pergantian penyedia akan menuntut *refactoring* kode domain pemesanan (*tight coupling*). Oleh karena itu, diperlukan **arsitektur notifikasi modular** yang mengisolasi detail vendor di lapisan *adapter*, mendukung peralihan penyedia secara mulus (*plug-and-play*) murni melalui konfigurasi *environment variables*.

---

## 2. Persona & Kebutuhan Pengguna

| Persona | Peran & Tanggung Jawab | Kebutuhan WhatsApp Modular |
| :--- | :--- | :--- |
| **`guest` (Tamu)** | Pelanggan yang memesan kamar online | Menerima pesan konfirmasi WhatsApp instan ($\le 3$ detik) dalam Bahasa Indonesia yang ramah, memuat detail booking dan link e-voucher resmi. |
| **`receptionist` (Meja Depan)** | Staf front office hotel | Memastikan tamu yang datang ke lobi sudah memegang e-voucher via nomor WhatsApp yang didaftarkan saat booking. |
| **`gm_admin` / IT Ops** | Pengelola infrastruktur hotel | Fleksibilitas memilih dan mengganti provider WhatsApp (Twilio vs Meta Cloud API vs Generic Gateway vs Log) tanpa merusak logika pemesanan hotel. |

---

## 3. Matriks Penyedia WhatsApp yang Didukung

Sistem wajib mendukung 4 jenis penyedia (*provider*) WhatsApp tanpa ketergantungan *third-party* SDK yang berat (murni Go standard library `net/http`):

| Provider Code | Nama Penyedia | Protokol / Format | Target Penggunaan | Autentikasi |
| :--- | :--- | :--- | :--- | :--- |
| `log` | **Structured Log Mock** | Memory & `slog` JSON | Unit testing, CI/CD, local dev | None |
| `generic_http` | **External Generic Gateway** | HTTP POST JSON (Fonnte/Wablas/Waha) | Operasional hemat biaya | Token / API Key di Header |
| `twilio` | **Twilio Programmable Messaging** | HTTP POST Form-Urlencoded | Enterprise SLA & global delivery | Basic Auth (`AccountSID` : `AuthToken`) |
| `meta_cloud` | **WhatsApp Business Cloud API** | HTTP POST JSON (Meta Graph API v20.0) | Official WABA bercentang hijau | Bearer Token & Phone Number ID |

---

## 4. Kriteria Keberhasilan & Acceptance Criteria

1. **AC-1 (Zero Domain Coupling):** Modul `internal/booking`, `internal/channel`, dan `cmd/server` hanya bergantung pada interface `notifier.WhatsAppSender`. Tidak ada satupun kode domain yang mengetahui jenis provider yang aktif.
2. **AC-2 (Standard Library Anti-Overengineering):** Seluruh adapter provider diimplementasikan menggunakan paket bawaan Go `net/http` tanpa mengimpor external vendor SDK (Ponytail principle).
3. **AC-3 (Config-Driven Provider Switching):** Pergantian provider dilakukan hanya dengan mengubah variabel lingkungan `WHATSAPP_PROVIDER` (`log`, `generic_http`, `twilio`, `meta_cloud`).
4. **AC-4 (Phone Number Normalization):** Sistem secara otomatis menormalkan format nomor telepon Indonesia (misal: `0812...`, `+62812...`, `62812...`) sesuai kebutuhan spesifik masing-masing API provider (E.164 untuk Twilio, digits-only untuk Meta).
5. **AC-5 (Graceful Error & Timeout Handling):** Kegagalan jaringan atau timeout (default 10 detik) menghasilkan error terdefinisi `ErrWhatsAppDeliveryFailed` yang aman dicoba ulang (*retryable*) oleh NATS consumer atau outbox relay.
