# Bug Report: Promo Management UI & BFF Mutation Lockout

- **Tanggal:** 2026-10-05 03:35:00 WIB
- **Modul Terdampak:** Revenue Workspace — Promo Management (`/staff/promos`)
- **Severity:** High (Functional Blocker for Revenue Manager)
- **Pelapor:** User / QA Testing
- **Status:** In Progress (Solving)

---

## 1. Deskripsi Masalah
Pengguna dengan peran `revenue_mgr` atau `gm_admin` yang membuka halaman manajemen promo di `https://hotel.fied.space/staff/promos` tidak dapat melakukan pengeditan atau pembaruan status terhadap promo yang ada. Ketika mencoba menambahkan promo baru, form hanya menampilkan pesan *"Promo baru hanya dibuat sebagai simulasi"* tanpa tersimpan ke database backend.

---

## 2. Analisis Akar Masalah (Root Cause Analysis)

1. **Frontend View Mock Isolation (`app/pages/staff/promos.vue`):**
   * Halaman promo masih mengimpor data statis mockup dari `~/data/management-scenarios` (`samplePromos`).
   * Tidak ada pemanggilan API ke endpoint `/api/bff/staff/revenue/promos` untuk mengambil data promo nyata dari database Neon.
   * Tidak ada kontrol interaktif (tombol Edit, Toggle status Aktif/Nonaktif, atau Modal Edit Kuota) pada kartu promo.
   * Fungsi `save()` hanya menambahkan draft ke array lokal JavaScript dan membuang perubahan saat reload.

2. **BFF Staff Capabilities Hardcoded Read-Only (`server/utils/staff-capabilities.ts`):**
   * Fungsi `resolveStaffRoute(path, method)` memiliki aturan penguncian:
     ```typescript
     if (method !== 'GET' && method !== 'HEAD') return null
     ```
   * Rute `PUT /api/v1/revenue/promos/:id` dan `POST /api/v1/revenue/promos` belum didaftarkan ke dalam matriks kapabilitas yang diizinkan.

3. **BFF Catch-All Handler Method Hardcoded (`server/api/bff/staff/[...path].ts`):**
   * Handler proksi staf hanya meneruskan metode `GET`:
     ```typescript
     return staffRequest(event, capability.upstream, { method: 'GET', query })
     ```
   * Payload request body untuk operasi `POST` dan `PUT` tidak dibaca atau diteruskan ke Go backend.

---

## 3. Rencana Penyelesaian Komprehensif (Action Plan)

1. **BFF Layer Fix (`mimiking-booking-secure/server/`):**
   * Perluas `server/utils/staff-capabilities.ts` untuk mendukung rute mutasi:
     * `POST /api/v1/revenue/promos`
     * `PUT /api/v1/revenue/promos/:id`
   * Perbarui `server/api/bff/staff/[...path].ts` untuk membaca `body` pada metode `POST`/`PUT` dan memverifikasi proteksi CSRF staf via `assertStaffMutation`.
2. **Frontend UI Fix (`mimiking-booking-secure/app/pages/staff/promos.vue`):**
   * Hubungkan pemanggilan data ke `GET /api/bff/staff/revenue/promos` menggunakan fetch live.
   * Tambahkan tombol **Edit Kuota & Status** pada setiap kartu promo.
   * Tambahkan tombol cepat **Toggle Aktif / Nonaktif** (`is_active: boolean`).
   * Hubungkan form pembuatan promo baru langsung ke `POST /api/bff/staff/revenue/promos`.
3. **Verifikasi & Deployment:**
   * Build image frontend baru via Docker BuildKit.
   * Restart container `hotel-fe` di VPS.
   * Uji coba langsung pengeditan status promo dan kuota via UI web dan cURL.

---

## 4. Log Perbaikan & Hasil Pengujian
*(Akan diperbarui setelah implementasi selesai dan divalidasi)*
