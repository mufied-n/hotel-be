# Software Requirements Specification (SRS) — Guest Session Logout Hardening & Cookie Security (BE-R04)

**Feature:** Guest Session Logout Hardening, Secure Cookies, and Private Cache Headers  
**Gap ID:** BE-R04 (P1)  
**Date:** 2026-10-03  
**Status:** Approved for Implementation  
**Target Module:** `internal/api` (`guest_auth.go`), `internal/guest` (`service.go`)  

---

## 1. Functional Requirements

### FR-01: Honest Session Revocation on Logout
- Saat client memanggil `POST /api/v1/auth/guest/logout`:
  1. Handler mengekstrak token sesi dari header `Authorization: Bearer`, `X-Guest-Session`, atau cookie `guest_session`.
  2. Jika token ditemukan, panggil `d.GuestSvc.RevokeSession(ctx, token)`.
  3. Jika `RevokeSession` mengembalikan error, handler **WAJIB** merespons dengan:
     - HTTP Status: `503 Service Unavailable`
     - Header: `Cache-Control: no-store, private`
     - Body JSON:
       ```json
       {
         "error": "LOGOUT_FAILED",
         "message": "Gagal mencabut sesi pada server. Silakan coba lagi."
       }
       ```
     - Cookie `guest_session` **TIDAK DIHAPUS** agar client dapat mengirim ulang permintaan retry.
  4. Jika `RevokeSession` sukses (atau tidak ada token sesi yang aktif):
     - Handler menghapus cookie `guest_session` dari browser.
     - HTTP Status: `200 OK`
     - Header: `Cache-Control: no-store, private`
     - Body JSON:
       ```json
       {
         "status": "ok",
         "message": "Sesi Anda telah berhasil diakhiri."
       }
       ```

### FR-02: Secure Cookie Attribute Determination
- Cookie `guest_session` yang disetel pada `handleGuestVerify` dan dihapus pada `handleGuestLogout` harus menghitung nilai atribut `Secure`:
  ```go
  func isSecureRequest(c *gin.Context, isDev bool) bool {
      if c.Request.TLS != nil {
          return true
      }
      if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
          return true
      }
      return !isDev
  }
  ```
- Cookie flags:
  - `Name`: `"guest_session"`
  - `Path`: `"/"`
  - `HttpOnly`: `true`
  - `SameSite`: `http.SameSiteLaxMode`
  - `Secure`: `isSecureRequest(c, d.IsDevelopment)`

### FR-03: Private Cache Control Headers
- Handler berikut wajib menambahkan header antipenyimpanan cache:
  - `c.Header("Cache-Control", "no-store, private")`
  - `c.Header("Pragma", "no-cache")`
  - Berlaku pada: `handleGuestVerify`, `handleGuestMe`, `handleGuestLogout`.

### FR-04: TouchSession Error Logging in ValidateSession
- Pada `ValidateSession`:
  - Jika `s.store.TouchSession` mengembalikan error:
    - Log error level `WarnContext` atau `ErrorContext`: `"guest.touch_session_failed"`, `"session_id"`, `session.ID`, `"err"`, `err`.
    - Tidak memperbarui `session.LastActiveAt` dan `session.ExpiresAt` secara in-memory jika update database gagal, memastikan state memori selalu mencerminkan state database.

---

## 2. API Contract Specification

### POST `/api/v1/auth/guest/logout`
- **Request Headers**:
  - `Authorization: Bearer gst_sess_<token>` ATAU Cookie `guest_session=<token>`
- **Response 200 OK**:
  ```http
  HTTP/1.1 200 OK
  Content-Type: application/json
  Cache-Control: no-store, private
  Pragma: no-cache
  Set-Cookie: guest_session=; Path=/; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; SameSite=Lax

  {
    "status": "ok",
    "message": "Sesi Anda telah berhasil diakhiri."
  }
  ```
- **Response 503 Service Unavailable** (Database Revocation Failure):
  ```http
  HTTP/1.1 503 Service Unavailable
  Content-Type: application/json
  Cache-Control: no-store, private
  Pragma: no-cache

  {
    "error": "LOGOUT_FAILED",
    "message": "Gagal mencabut sesi pada server. Silakan coba lagi."
  }
  ```

---

## 3. Error Code Catalog

| Error Code | HTTP Status | Keterangan |
|---|---|---|
| `LOGOUT_FAILED` | 503 Service Unavailable | Gagal menghapus token sesi di database saat logout |
| `UNAUTHORIZED` | 401 Unauthorized | Sesi tidak valid atau telah dicabut |
| `INTERNAL_SERVER_ERROR` | 500 Internal Server Error | Kesalahan internal sistem |
