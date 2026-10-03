# Software Requirements Specification (SRS) — Feature Flags & Runtime Configuration System
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Dokumen ID:** `SRS-FEATURE-FLAGS-2026-10-03`
- **Versi:** 1.0.0
- **Tanggal:** 2026-10-03
- **Status:** Approved / In Execution
- **PRD Pasangan:** [`docs/prd/feature-flags-system-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/feature-flags-system-2026-10-03.md)
- **Tech Doc Pasangan:** [`docs/tech/feature-flags-system-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/feature-flags-system-architecture-2026-10-03.md)

---

## 1. Kebutuhan Fungsional (Functional Requirements)

### FR-FF-01: Penyimpanan Persisten & Skema Data
Sistem wajib menyimpan konfigurasi feature flag pada database PostgreSQL tabel `feature_flags`:
```sql
CREATE TABLE feature_flags (
    key           VARCHAR(64) PRIMARY KEY,
    name          VARCHAR(128) NOT NULL,
    description   TEXT NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    allowed_roles TEXT[] NOT NULL DEFAULT '{}',
    updated_by    VARCHAR(64) NOT NULL DEFAULT 'system',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### FR-FF-02: Evaluasi In-Memory Lock-Free
1. Sistem wajib menyediakan package `internal/platform/featureflag` yang memuat snapshot memori RAM murni (`atomic.Pointer[map[string]FlagConfig]`).
2. Evaluasi status flag melalui method `IsEnabled(ctx context.Context, key string) bool` wajib mengeksekusi logika:
   - Jika key tidak ditemukan di snapshot, return `false` (Fail-Closed).
   - Jika flag berstatus `enabled == false`, return `false`.
   - Jika flag berstatus `enabled == true` dan `len(allowed_roles) == 0`, return `true` (berlaku global).
   - Jika `len(allowed_roles) > 0`, ekstrak role caller dari context (`GetAuthContext(ctx).Role`). Jika role caller ada di dalam slice `allowed_roles`, return `true`, selain itu return `false`.

### FR-FF-03: Transport Middleware `RequireFeature`
Sistem menyediakan Chi HTTP middleware `RequireFeature(ffManager, flagKey)` yang disematkan pada route API:
- Jika `ffManager.IsEnabled(r.Context(), flagKey)` bernilai `true`, request dilanjutkan ke handler berikutnya (`next.ServeHTTP`).
- Jika bernilai `false`, hentikan request dan kembalikan response `503 Service Unavailable` atau `404 Not Found` berformat RFC 7807:
  ```json
  {
    "type": "https://errors.pulangkeuttara.id/feature-disabled",
    "title": "Service Unavailable",
    "status": 503,
    "detail": "Fitur 'ff_guest_portal_auth' sedang dinonaktifkan sementara.",
    "code": "FEATURE_DISABLED"
  }
  ```

### FR-FF-04: API Administrasi Runtime Feature Flags
Sistem menyediakan endpoint RESTful bagi administrator (`gm_admin`):
1. **Daftar Seluruh Flag:**
   * **Method:** `GET /api/v1/admin/feature-flags`
   * **Auth:** Bearer Token role `gm_admin`.
   * **Respons 200 OK:**
     ```json
     {
       "total": 17,
       "flags": [
         {
           "key": "ff_catalog_write",
           "name": "Catalog Mutation CRUD",
           "description": "Mengizinkan penambahan, pembaruan, dan penghapusan varian kamar",
           "enabled": true,
           "allowed_roles": ["revenue_mgr", "gm_admin"],
           "updated_by": "gm_admin",
           "updated_at": "2026-10-03T11:00:00Z"
         }
       ]
     }
     ```
2. **Pembaruan Status Flag:**
   * **Method:** `PUT /api/v1/admin/feature-flags/{key}`
   * **Auth:** Bearer Token role `gm_admin`.
   * **Payload Request:**
     ```json
     {
       "enabled": false,
       "allowed_roles": ["gm_admin"]
     }
     ```
   * **Respons 200 OK:**
     ```json
     {
       "status": "updated",
       "flag": {
         "key": "ff_catalog_write",
         "enabled": false,
         "allowed_roles": ["gm_admin"],
         "updated_by": "staff:gm_admin",
         "updated_at": "2026-10-03T11:32:00Z"
       }
     }
     ```

### FR-FF-05: Sinkronisasi Multi-Node Real-time (Hybrid)
1. Saat endpoint update dipanggil, sistem secara atomik memperbarui tabel PostgreSQL dan mempublikasikan event ke Valkey channel `pku:feature_flags:updated`.
2. Seluruh node backend yang me-subscribe channel menerima notifikasi dan me-reload snapshot memori secara lokal (< 10 milidetik).
3. Goroutine polling berkala (interval 30 detik) berjalan di latar belakang untuk rekonsiliasi cadangan jika koneksi pub/sub terputus sesaat.

---

## 2. Kebutuhan Non-Fungsional (Non-Functional Requirements)

1. **NFR-PERF (Latensi < 50 Nanodetik):** Evaluasi flag pada hot path HTTP tidak boleh melakukan network roundtrip (nol query DB/Redis per request).
2. **NFR-CONC (Lock-Free Thread Safety):** Pembacaan flag aman dari data race pada ribuan goroutine paralel tanpa *mutex contention* melalui `atomic.Pointer`.
3. **NFR-RESIL (Tahan Banting terhadap Outage DB):** Jika PostgreSQL mengalami gangguan koneksi, service tetap berjalan normal melayani evaluasi flag menggunakan snapshot RAM terakhir.
4. **NFR-ZERO-DEP (Zero External Dependencies):** Mengikuti filosofi Ponytail — hanya memanfaatkan Go standard library, driver pgx, dan Valkey client yang sudah ada.
