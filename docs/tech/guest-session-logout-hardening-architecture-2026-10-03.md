# Technical Architecture — Guest Session Logout Hardening & Cookie Security (BE-R04)

**Feature:** Guest Session Logout Hardening, Secure Cookies, and Private Cache Headers  
**Gap ID:** BE-R04 (P1)  
**Date:** 2026-10-03  
**Status:** Approved for Implementation  
**Target Module:** `internal/api` (`guest_auth.go`), `internal/guest` (`service.go`)  

---

## 1. Sequence Diagram: Honest Session Revocation (Logout)

```mermaid
sequenceDiagram
    autonumber
    actor Guest as Tamu / Browser
    participant API as API Handler (handleGuestLogout)
    participant Svc as Guest Service (RevokeSession)
    participant DB as PostgreSQL (guest_sessions)

    Guest->>API: POST /api/v1/auth/guest/logout<br/>(Cookie: guest_session=gst_sess_... / Bearer)
    API->>API: Extract token from Bearer / Cookie
    API->>Svc: RevokeSession(ctx, rawToken)
    Svc->>DB: DELETE FROM guest_sessions WHERE token_hash = sha256(token)

    alt Database Error (e.g. Connection Lost)
        DB-->>Svc: Error (DB connection broken)
        Svc-->>API: Error (failed to delete session)
        API-->>Guest: HTTP 503 Service Unavailable<br/>Cache-Control: no-store, private<br/>{"error":"LOGOUT_FAILED"}<br/>(Cookie DIPERTAHANKAN untuk retry)
    else Revocation Succeeded
        DB-->>Svc: RowsAffected = 1 (or 0 if already removed)
        Svc-->>API: nil (Success)
        API->>API: Construct Expired Cookie (MaxAge: -1, HttpOnly: true, Secure: env/TLS)
        API-->>Guest: HTTP 200 OK<br/>Set-Cookie: guest_session=; Max-Age=0; HttpOnly<br/>Cache-Control: no-store, private<br/>{"status":"ok"}
    end
```

---

## 2. Cookie Security Architecture

Cookie sesi tamu dikonfigurasi mengikuti standar keamanan modern:

| Parameter Cookie | Nilai yang Ditetapkan | Alasan Keamanan |
|---|---|---|
| `Name` | `guest_session` | Nama cookie standar untuk portal tamu |
| `Path` | `/` | Berlaku untuk semua rute `/api/v1` |
| `HttpOnly` | `true` | Mencegah pembacaan token melalui script JavaScript (Mitigasi XSS) |
| `SameSite` | `http.SameSiteLaxMode` | Mencegah pengiriman cookie pada request lintas situs ilegal (Mitigasi CSRF) |
| `Secure` | `isSecureRequest(c, d.IsDevelopment)` | Mencegah transmisi cookie dalam plaintext pada koneksi non-HTTPS |
| `Expires` | `sess.ExpiresAt` (saat login) / `Unix(0, 0)` (saat logout) | Mengikuti sliding window sesi server |

### Algoritma Deteksi TLS/HTTPS
```go
func isSecureCookie(c *gin.Context, isDev bool) bool {
    if c.Request.TLS != nil {
        return true
    }
    if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
        return true
    }
    return !isDev
}
```

---

## 3. Anti-Overengineering Analysis (Ponytail)

- **Tidak Menambah Tabel atau Dependensi Baru**: Semua perubahan memanfaatkan tabel `guest_sessions` dan struktur `gin.Context` yang sudah ada.
- **Kompak & Deterministik**: Penanganan error `RevokeSession` dan `TouchSession` hanya melibatkan penambahan pengecekan error dan logging, tanpa abstraksi tambahan.
