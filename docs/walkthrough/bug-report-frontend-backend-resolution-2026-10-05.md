# Bug Report & Resolution: BFF 503 Upstream Unreachable (`hotel.fied.space`)

**Date:** 2026-10-05  
**Reporter:** System Operations / Automated Diagnostic  
**Status:** **RESOLVED & VERIFIED**  
**Affected Service:** Frontend BFF (`mimiking-booking-secure`) / Backend API Integration  

---

## 1. Problem Statement

Saat mengakses endpoint katalog kamar dari frontend:
```http
GET /api/bff/catalog/rooms/01900000-0000-7000-8000-000000000007
```
Frontend Nuxt BFF mengembalikan response error HTTP 503:
```json
{
  "statusCode": 503,
  "statusMessage": "Backend booking belum dapat dijangkau."
}
```

---

## 2. Root Cause Analysis (RCA)

1. **DNS Missing A Record:**
   - Saat proses konfigurasi verifikasi email Resend sebelumnya, record DNS `A` untuk subdomain `hotel.fied.space` tidak lagi terdaftar di Cloudflare DNS (`dig hotel.fied.space A` menghasilkan `0 answer`).
2. **Container Name Resolution Failure:**
   - Container frontend Nuxt (`hotel-fe`) di VPS memiliki environment variable `NUXT_BACKEND_BASE_URL=https://hotel.fied.space`.
   - Node.js runtime (`undici / fetch`) di dalam container mengeksekusi request ke `https://hotel.fied.space`.
   - Resolver DNS container gagal melakukan lookup nama host:
     ```text
     TypeError: fetch failed
       [cause]: Error: getaddrinfo ENOTFOUND hotel.fied.space
         code: 'ENOTFOUND',
         syscall: 'getaddrinfo',
         hostname: 'hotel.fied.space'
     ```
   - Blok penanganan error pada `server/utils/bff.ts` menangkap exception tersebut dan mengonversinya menjadi error 503:
     ```ts
     try {
       response = await fetch(safeBackendURL(event, path, options.query), { ... })
     } catch {
       throw createError({ statusCode: 503, statusMessage: 'Backend booking belum dapat dijangkau.' })
     }
     ```

---

## 3. Resolution & Mitigation

Untuk menjamin komunikasi antar-container di VPS tetap **100% stabil, berkecepatan tinggi, dan tidak rentan terhadap fluktuasi atau ketiadaan DNS publik**:

1. **Konfigurasi `extra_hosts` pada `docker-compose.prod.yml`:**
   Menambahkan pemetaan host statis pada container frontend:
   ```yaml
   services:
     web:
       image: ghcr.io/mufied-n/hotel-fe:latest
       container_name: hotel-fe
       restart: unless-stopped
       ports:
         - "127.0.0.1:3002:3000"
       extra_hosts:
         - "hotel.fied.space:95.179.243.181"
   ```
2. **Deploy & Container Restart di VPS:**
   - File `/opt/hotel-fe/docker-compose.prod.yml` diperbarui di server produksi.
   - Container `hotel-fe` di-recreate dengan pemetaan `extra_hosts`.
   - Container sekarang langsung menghubungi Nginx lokal di `95.179.243.181:443` dengan SNI TLS yang valid (`DNS:hotel.fied.space`).
3. **Commit ke Repository Frontend:**
   - Perubahan di-commit ke branch `main` (`a6a8c51`) agar persist pada deployment GitHub Actions berikutnya.

---

## 4. Verification & Testing

### Uji 1: Node Fetch Internal Container
```bash
docker exec hotel-fe node -e "fetch('https://hotel.fied.space/ready').then(r=>console.log('STATUS:', r.status))"
# Output: STATUS: 200
```

### Uji 2: HTTP GET Catalog Room Endpoint
```bash
curl -s http://127.0.0.1:3002/api/bff/catalog/rooms/01900000-0000-7000-8000-000000000007 | jq .name
# Output: "Presidential Suite"
```

Seluruh request BFF ke backend kini berhasil diproses secara instan (HTTP 200).
