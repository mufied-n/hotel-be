# Walkthrough Tracker — Batch BE-F: Verifikasi Konkurensi DB Nyata
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Pelacakan:** 3 Oktober 2026
- **Status:** **Completed (100% — All 5 Integration Tests Pass, 0 Flakiness, Full Standards Adherence)**
- **Feature Target:** Penutupan `BE-G20` (Bukti Verifikasi Konkurensi Database PostgreSQL 18 & Valkey Nyata), Penegakan ACID Row-Locking, GiST Exclusion Constraints, dan Multi-Night Atomicity Rollback.

---

## Standar Rekayasa & Kepatuhan yang Ditegakkan

1. **Integritas Konkurensi Database (ACID & ISO/IEC 25010):**
   - Uji serentak 20 goroutines pada kamar terakhir ($N=1$).
   - Verifikasi penguncian `SELECT ... FOR UPDATE` mencegah double-booking secara mutlak.
   - Pengecekan konsistensi stok kamar non-negatif di tabel `inventory`.
2. **Keamanan Alokasi Kamar Fisik (PostgreSQL GiST & BE-G17):**
   - Verifikasi `FOR UPDATE OF r SKIP LOCKED` pada alokasi kamar fisik bebas konflik.
   - Verifikasi kegagalan `23P01` (*exclusion_violation*) jika terjadi upaya double-assign kamar fisik.
   - Penanganan retry konkuren sementara (*transient conflict retry*) pada service `CheckIn`.
3. **Atomicity Rollback Multi-Malam (ISO/IEC 27001 A.12.1.2):**
   - Kegagalan ketersediaan pada salah satu malam membatalkan seluruh reservasi tanpa ada stok yang bocor.
4. **Anti-Overengineering (Ponytail Rule):**
   - Menggunakan standard library Go `testing`, `sync.WaitGroup`, dan `pgxpool.Pool`.
   - Tanpa dependensi pihak ketiga baru yang tidak perlu.

---

## Checklist Eksekusi Bertahap

- [x] **Fase 1: Riset Mendalam Standar Konkurensi & Database**
  - [x] Riset isolasi PostgreSQL 18, row locks, dan batasan GiST
  - [x] Riset standar ISO/IEC 25010 (Fault Tolerance) dan PCI-DSS v4.0 (Audit Trail)
  - [x] Riset isolasi database uji `booking_test` pada Docker network

- [x] **Fase 2: Dokumen Analisis & Spesifikasi**
  - [x] PRD: [`docs/prd/real-db-concurrency-verification-batch-f-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/real-db-concurrency-verification-batch-f-2026-10-03.md)
  - [x] SRS: [`docs/srs/real-db-concurrency-verification-batch-f-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/real-db-concurrency-verification-batch-f-2026-10-03.md)
  - [x] Tech Architecture: [`docs/tech/real-db-concurrency-verification-architecture-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/real-db-concurrency-verification-architecture-2026-10-03.md)
  - [x] Walkthrough: [`docs/walkthrough/real-db-concurrency-verification-walkthrough-2026-10-03.md`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/real-db-concurrency-verification-walkthrough-2026-10-03.md)

- [x] **Fase 3: Implementasi Test Suite Konkurensi Database Nyata (TDD)**
  - [x] Buat helper koneksi database `testing/integration/db_helper.go`
  - [x] Buat suite `TestRealDB_RaceOnLastRoom` (20 concurrent goroutines merebut 1 kamar)
  - [x] Buat suite `TestRealDB_MultiNightRollbackAtomicity` (all-or-nothing atomicity rollback)
  - [x] Buat suite `TestRealDB_ParallelRoomAssignment_SkipLocked` (check-in paralel via SKIP LOCKED & retry)
  - [x] Buat suite `TestRealDB_GiSTExclusionConstraint` (penolakan physical double booking)
  - [x] Buat suite `TestRealDB_HoldExpirySweepVsPaymentRace` (resolusi balapan hold expired vs confirm)

- [x] **Fase 4: Eksekusi Otomasi & Pelaporan Bukti Nyata**
  - [x] Jalankan suite integrasi terhadap PostgreSQL 18
  - [x] Simpan laporan hasil eksekusi di `testing/e2e/report/2026-10-03-real-db-concurrency-batch-f-e2e-report.md`

- [x] **Fase 5: Verification & Commit Integrity**
  - [x] `go test -v ./...` dan `go vet ./...` (100% pass)
  - [x] Audit selektif git status dan staging hanya file terkait BE-F
  - [x] Minta konfirmasi sebelum melakukan commit

