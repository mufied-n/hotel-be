# Product Requirements Document (PRD) — Room Variant Catalog CRUD Management
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Status:** Approved / In Implementation
- **Target Persona:** `revenue_mgr` (Revenue Manager), `gm_admin` (General Manager), `guest` (Tamu Publik)
- **Modul:** `internal/catalog`, `internal/api`

---

## 1. Latar Belakang & Masalah Bisnis

Pada arsitektur awal Hotel Pulang ke Uttara, data 7 varian kamar (`sup-king`, `sup-twin`, `dlx-king`, `dlx-twin`, `exc-king`, `jste-suite`, `pste-suite`) diinisialisasi melalui migrasi SQL statis dan fungsi fallback pengujian [`DefaultVariants()`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/catalog/catalog.go).

Meskipun memadai untuk pengujian unit dan initial bootstrapping, hotel bintang 4 dinamis memerlukan kapabilitas operasional bagi tim manajemen:
1. **Manajemen Harga Dasar & Promosi Musiman:** Revenue Manager harus dapat memperbarui harga dasar (`base_price_minor`) kamar tanpa harus melakukan deployment kode atau manual query ke PostgreSQL.
2. **Pembaruan Fasilitas & Aset Media:** Hotel secara berkala memperbarui fasilitas kamar (misal: penambahan Smart TV, espresso machine) serta foto profesional baru (`photos`).
3. **Penyesuaian Tipe Kamar / Paket:** Manajemen dapat menambahkan varian promosi atau varian musiman serta menonaktifkan tipe kamar yang sedang direnovasi.

---

## 2. Persona Pengguna & Matriks Hak Akses

| Operasi | Endpoint HTTP | Guest (Publik) | Receptionist | Revenue Manager | GM Admin |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **Daftar Varian Kamar** | `GET /api/v1/catalog/rooms` | ✅ | ✅ | ✅ | ✅ |
| **Detail Varian Kamar** | `GET /api/v1/catalog/rooms/{id}` | ✅ | ✅ | ✅ | ✅ |
| **Buat Varian Baru** | `POST /api/v1/catalog/rooms` | ❌ (403) | ❌ (403) | ✅ | ✅ |
| **Perbarui Varian Kamar** | `PUT /api/v1/catalog/rooms/{id}` | ❌ (403) | ❌ (403) | ✅ | ✅ |
| **Hapus Varian Kamar** | `DELETE /api/v1/catalog/rooms/{id}` | ❌ (403) | ❌ (403) | ❌ (403) | ✅ |

---

## 3. Kriteria Keberhasilan & Acceptance Criteria

1. **AC-01 (Discovery Lengkap):** Endpoint publik `GET /api/v1/catalog/rooms` dan `GET /api/v1/catalog/rooms/{id}` dapat diakses tanpa autentikasi oleh tamu publik.
2. **AC-02 (Kreasi Varian Baru):** Endpoint `POST /api/v1/catalog/rooms` memvalidasi kelengkapan data (`code`, `name`, `bed_type`, `max_capacity >= 1`, `base_price_minor > 0`), menghasilkan UUID v7 baru, dan menyimpan ke PostgreSQL.
3. **AC-03 (Pembaruan Varian):** Endpoint `PUT /api/v1/catalog/rooms/{id}` memperbarui kolom metadata kamar secara idempotent.
4. **AC-04 (Penghapusan Aman):** Endpoint `DELETE /api/v1/catalog/rooms/{id}` menolak penghapusan (409 Conflict) jika varian kamar terikat pada inventaris fisik atau reservasi aktif.
5. **AC-05 (Proteksi RBAC Ketat):** Akses modifikasi dibatasi ketat oleh Casbin Enforcer: `POST` & `PUT` hanya untuk `revenue_mgr` dan `gm_admin`, `DELETE` hanya untuk `gm_admin`.
6. **AC-06 (Test Coverage & Anti-Overengineering):** Seluruh logika CRUD diuji dengan table-driven test dengan cakupan statemen $\ge 80\%$ tanpa dependensi ORM tambahan.
