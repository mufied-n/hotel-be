# Software Requirements Specification (SRS)
# Role-Based Access Control (RBAC) Subsystem
**Proyek:** Hotel Booking Engine — Pulang ke Uttara  
**Dokumen ID:** `SRS-RBAC-2026-10-03`  
**Versi:** 1.0.0  
**Tanggal:** 2026-10-03  
**Status:** Approved / Specification Baseline  

---

## 1. Pendahuluan

### 1.1 Tujuan
Dokumen Spesifikasi Kebutuhan Perangkat Lunak (*Software Requirements Specification* / SRS) ini menjabarkan seluruh kebutuhan fungsional (*functional requirements*), non-fungsional (*non-functional requirements*), antarmuka sistem (*interface requirements*), serta batasan desain untuk subsistem **Role-Based Access Control (RBAC)** pada sistem pemesanan kamar hotel **Pulang ke Uttara** (Yogyakarta).

### 1.2 Cakupan Sistem (System Scope)
Subsistem RBAC bertanggung jawab untuk:
1. Membaca identitas subjek dan peran (*role*) dari transport layer HTTP.
2. Memuat aturan kebijakan otorisasi (*authorization rules*) dari tabel PostgreSQL `casbin_rule` ke dalam memori.
3. Melakukan evaluasi pencocokan hak akses (*enforcement*) terhadap triplet `(subject/role, object/path, action/method)` secara deterministik dan thread-safe.
4. Memberikan respon HTTP yang konsisten (`401 Unauthorized` atau `403 Forbidden`) saat terjadi kegagalan otentikasi maupun otorisasi.

---

## 2. Aktor Sistem & Batasan Lingkup

### 2.1 Aktor Sistem
1. **Unauthenticated Public Visitor (`guest`)**: Pengunjung publik tanpa kredensial yang menjelajah kamar dan melakukan reservasi awal.
2. **Authenticated Staff Member**: Staf hotel yang memiliki kredensial resmi (kategori: `receptionist`, `housekeeping`, `revenue_mgr`, `finance`, `gm_admin`).
3. **Casbin Enforcer Engine**: Komponen runtime Go yang mengevaluasi relasi subjek-objek-aksi.
4. **PostgreSQL Policy Store**: Database relasional penyimpan tabel `casbin_rule` dan profil `staff_users`.

---

## 3. Kebutuhan Fungsional (Functional Requirements)

### FR-01: Ekstraksi Kredensial Subjek (Subject Extraction)
* **Deskripsi:** Middleware HTTP wajib mengidentifikasi subjek pemanggil dari request yang masuk.
* **Input:**
  * Header `Authorization: Bearer <token>` ATAU
  * Header `X-User-Role: <role>` (khusus internal/development gateway) ATAU
  * Tidak ada header (dianggap sebagai subjek default `guest`).
* **Proses:**
  1. Periksa keberadaan header `X-User-Role` atau token Bearer.
  2. Jika header tidak ditemukan, tetapkan context `Role = "guest"` dan `Subject = "anonymous"`.
  3. Jika header ditemukan dan valid, simpan `Role` dan `Subject` ke dalam `context.Context` request.
* **Output:** Context request terisi `AuthContext{Subject, Role}`.

### FR-02: Evaluasi Izin Akses (Policy Enforcement)
* **Deskripsi:** Middleware wajib mencocokkan subjek/role, path URL yang diminta, dan HTTP method terhadap aturan kebijakan Casbin.
* **Input:**
  * `r.sub`: Nilai role atau subject dari context (misal: `receptionist`).
  * `r.obj`: Path URL yang dinormalisasi (misal: `/api/v1/bookings/b8c0e271-e23a-44e2-a08b-4a5cf2b013e8/check-in`).
  * `r.act`: HTTP Verb (misal: `POST`).
* **Proses:**
  1. Jalankan `enforcer.Enforce(sub, obj, act)`.
  2. Gunakan matcher: `g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == "*")`.
* **Output:**
  * `allowed == true`: Lanjutkan eksekusi ke handler berikutnya (`next.ServeHTTP(w, r)`).
  * `allowed == false`: Hentikan eksekusi dan kirim respon error HTTP `403 Forbidden`.

### FR-03: Penanganan Akses Tidak Berizin (403 Forbidden)
* **Deskripsi:** Menghasilkan payload error JSON terstruktur saat permintaan ditolak oleh enforcer.
* **Format Respon:**
  ```json
  {
    "error": "forbidden",
    "message": "access denied for role '%s' on %s %s"
  }
  ```
* **Status Code:** `403 Forbidden`
* **Header:** `Content-Type: application/json; charset=utf-8`

### FR-04: Penanganan Otentikasi Gagal (401 Unauthorized)
* **Deskripsi:** Dihasilkan saat endpoint terproteksi khusus staf diakses tanpa token otentikasi yang valid.
* **Format Respon:**
  ```json
  {
    "error": "unauthorized",
    "message": "authentication token required or invalid"
  }
  ```
* **Status Code:** `401 Unauthorized`

### FR-05: Pewarisan Peran (Role Inheritance)
* **Deskripsi:** Sistem harus mendukung hierarki peran di mana peran dengan privilese lebih tinggi dapat mewarisi seluruh hak akses peran dasar.
* **Aturan Default:**
  * `g, receptionist, guest` (Staf Front Office dapat melakukan seluruh tindakan Tamu).
  * `p, gm_admin, /api/v1/*, *` (General Manager memiliki hak akses tak terbatas).

### FR-06: Penyimpanan & Sinkronisasi Kebijakan (Policy Persistence)
* **Deskripsi:** Seluruh aturan kebijakan harus disimpan di database PostgreSQL tabel `casbin_rule`.
* **Proses:**
  1. Saat server start ([`cmd/server/main.go`](file:///mnt/code/projects/jobs/pulang/current-booking/cmd/server/main.go)), adapter membaca seluruh baris `casbin_rule` ke memori enforcer.
  2. Adapter menyediakan metode `LoadPolicy(model)` yang kompatibel dengan antarmuka Casbin `persist.Adapter`.

---

## 4. Antarmuka Eksternal & Kontrak HTTP (Interface Requirements)

### 4.1 Header Request
| Nama Header | Tipe | Wajib | Keterangan |
| :--- | :--- | :---: | :--- |
| `Authorization` | String | Kondisional | Format `Bearer <token>` untuk otentikasi staf. |
| `X-User-Role` | String | Opsional | Digunakan pada testing otomatis atau service internal. |
| `X-Request-ID` | String | Otomatis | UUID tracing request (diinjeksi Chi middleware). |

### 4.2 Endpoint RBAC Map (Baseline Pulang ke Uttara)
```text
GET   /healthz                             --> [Public]
GET   /ready                               --> [Public]
GET   /api/v1/availability                --> [guest, receptionist, revenue_mgr, finance, gm_admin]
POST  /api/v1/bookings                    --> [guest, receptionist, gm_admin]
GET   /api/v1/bookings/:id                --> [guest (own), receptionist, revenue_mgr, finance, gm_admin]
POST  /api/v1/bookings/:id/cancel         --> [guest, receptionist, gm_admin]
POST  /api/v1/bookings/:id/check-in       --> [receptionist, gm_admin]
POST  /api/v1/bookings/:id/check-out      --> [receptionist, gm_admin]
POST  /api/v1/bookings/:id/no-show        --> [receptionist, gm_admin]
POST  /fake-pay/:ref                      --> [guest, receptionist, gm_admin]
```

---

## 5. Kebutuhan Data & Skema Database

### 5.1 Skema Tabel `casbin_rule`
```sql
CREATE TABLE IF NOT EXISTS casbin_rule (
    id SERIAL PRIMARY KEY,
    ptype VARCHAR(100) NOT NULL,
    v0 VARCHAR(100) DEFAULT '',
    v1 VARCHAR(100) DEFAULT '',
    v2 VARCHAR(100) DEFAULT '',
    v3 VARCHAR(100) DEFAULT '',
    v4 VARCHAR(100) DEFAULT '',
    v5 VARCHAR(100) DEFAULT '',
    CONSTRAINT uq_casbin_rule UNIQUE (ptype, v0, v1, v2, v3, v4, v5)
);

CREATE INDEX IF NOT EXISTS idx_casbin_rule_lookup 
    ON casbin_rule (ptype, v0, v1);
```

### 5.2 Skema Tabel `staff_users`
```sql
CREATE TABLE IF NOT EXISTS staff_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(50) UNIQUE NOT NULL,
    role VARCHAR(50) NOT NULL,
    full_name VARCHAR(100) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## 6. Kebutuhan Non-Fungsional (Non-Functional Requirements)

1. **Performa (Performance)**:
   * Waktu evaluasi izin `Enforce()`: Rata-rata $< 1 \mu\text{s}$, P99 $< 5 \mu\text{s}$.
   * Tidak ada query SQL yang dilakukan saat request HTTP berlangsung untuk mengevaluasi hak akses.
2. **Keamanan Konkurensi (Concurrency Safety)**:
   * Menggunakan `casbin.SyncedEnforcer` yang dilengkapi `sync.RWMutex` guna memastikan pembacaan bersamaan oleh ribuan goroutine aman dari data race.
3. **Ketergantungan Minimal (Ponytail Compliance)**:
   * Tidak ada library pihak ketiga berukuran besar untuk persistensi (tanpa GORM, XORM, dsb.).
   * Menggunakan `pgxpool.Pool` yang sudah dikonfigurasi pada platform.
