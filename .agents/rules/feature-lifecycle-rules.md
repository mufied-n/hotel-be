---
trigger: always_on
description: Mandatory development lifecycle, engineering standards, TDD table test 80% coverage, and E2E testing rules for AI agents.
---

# AI Agent Feature Lifecycle & Engineering Standards Rule

Setiap kali AI Agent menerima permintaan pengembangan fitur baru (*new feature*) atau penyempurnaan (*refinement*) pada repositori Hotel Booking Engine (`Pulang ke Uttara`), AI Agent **WAJIB** mengeksekusi 6 tahapan siklus hidup (*lifecycle*) berikut secara ketat dan berurutan:

```
[Tahap 1: Research] ➔ [Tahap 2: Analisis & Dokumen] ➔ [Tahap 3: Walkthrough Tracking] ➔ [Tahap 4: TDD (≥80% Coverage)] ➔ [Tahap 5: E2E Testing] ➔ [Tahap 6: Review & Final Report]
```

---

## Tahap 1: Riset Mendalam (Market, Competitor, User, Engineering)
Sebelum menulis satu baris kode pun, Agent wajib melakukan riset komprehensif:
1. **Pasar & Kompetitor**: Gunakan tool `search_web` dan `read_url_content` untuk meneliti sistem booking hotel sekelas (misal: Book-Secure, Cloudbeds, SiteMinder, Agoda/Booking.com engine) dan pola industri perhotelan bintang 4.
2. **Kebutuhan Pengguna (User / Personas)**: Bedah kebutuhan tamu publik (`guest`) serta staf internal hotel (`receptionist`, `housekeeping`, `revenue_mgr`, `finance`, `gm_admin`).
3. **Engineering Standards**: Analisis best-practice Go, PostgreSQL, Redis/Valkey, dan arsitektur Hexagonal.
* **Skill Wajib Digunakan:** `search_web`, `read_url_content`, `golang-pro`, `postgres-best-practices`.

---

## Tahap 2: Analisis & Pembuatan Dokumen Siklus Hidup
Agent wajib membuat 3 dokumen formal sebelum implementasi:
1. **PRD (Product Requirements Document)**: Simpan di `docs/prd/[feature-name]-[date].md`. Berisi business background, user personas, permission/feature matrix, acceptance criteria, dan non-functional requirements.
2. **SRS (Software Requirements Specification)**: Simpan di `docs/srs/[feature-name]-[date].md`. Berisi Functional Requirements (FR-01, FR-02, dst.), kontrak HTTP RESTful (Request/Response JSON), dan spesifikasi error code.
3. **Dokumen Desain Teknis (Tech Architecture)**: Simpan di `docs/tech/[feature-name]-architecture-[date].md`. Berisi diagram arsitektur Mermaid, skema database, pola interface/dependency injection, dan profiling performa.
4. **Analisis Anti-Overengineering (Ponytail)**: Pastikan solusi seringkas mungkin (YAGNI, no unneeded dependencies, minimal abstractions).
* **Skill Wajib Digunakan:** `api-design-principles`, `postgres-best-practices`, `ponytail`, `golang-pro`.

---

## Tahap 3: Penyusunan Rencana & Walkthrough Tracking
1. Dokumen rencana kerja detil tidak perlu dikomit ke master jika berupa draft sementara, namun **Wajib** membuat dokumen pelacak eksekusi:
   * Format: `docs/walkthrough/[feature-name]-walkthrough-[date].md`.
2. Walkthrough berisi checklist fase, file yang akan diubah/dibuat, log eksekusi berkala, dan bukti output pengujian.

---

## Tahap 4: Implementasi Bertahap & Sistematis (TDD Pattern)
1. **Prinsip TDD**: Tulis test terlebih dahulu (Red), implementasikan kode minimal yang bekerja (Green), lalu rapikan (Refactor).
2. **Table-Driven Test Pattern**: Seluruh unit test dan HTTP handler test wajib menggunakan pola table-driven test Go (`tests := []struct{...}`).
3. **Standar Coverage Minimum**: 
   * Package baru atau modul yang mengalami refinement **WAJIB mencapai minimal 80% test coverage**.
   * Verifikasi dengan perintah: `go test -v -cover ./...`.
4. **Zero Lint / Vet Errors**: Wajib lulus `go vet ./...` tanpa warning atau error.
* **Skill Wajib Digunakan:** `golang-pro`, `ponytail`.

---

## Tahap 5: Manual & Automated End-to-End (E2E) Testing
Setelah pengujian unit dan integrasi lulus, Agent wajib melakukan pengujian E2E nyata terhadap alur fitur baru DAN regresi fitur yang sudah ada:
1. **Skrip E2E**:
   * Simpan di: `testing/e2e/script/[script-name].sh` atau Go test binary di `testing/e2e/script/`.
   * Skrip wajib menguji skenario lengkap dari awal hingga akhir (misal: Create Booking $\rightarrow$ Hold Expiry / Fake Pay $\rightarrow$ Staff Check-in $\rightarrow$ Staff Check-out $\rightarrow$ RBAC Forbidden checks).
2. **Laporan Hasil E2E**:
   * Simpan di: `testing/e2e/report/[YYYY-MM-DD-HHMMSS]-[feature-name]-e2e-report.md`.
   * Berisi ringkasan eksekusi, environment yang diuji, payload request/response nyata, HTTP status codes, dan kesimpulan verifikasi.

---

## Tahap 6: Verification & Commit Integrity
1. Jalankan `go test -v ./...` dan pastikan seluruh test suite pass 100%.
2. Pastikan file dokumentasi (`docs/prd/`, `docs/srs/`, `docs/tech/`, `docs/walkthrough/`, `testing/e2e/`) terisi lengkap dan terhubung dengan clickable github markdown links `file://`.
3. Laporkan status kepada User secara ringkas, transparan, dan minta konfirmasi sebelum melakukan git commit.
