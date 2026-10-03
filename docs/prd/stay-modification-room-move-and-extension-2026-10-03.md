# PRD — Proposed 03: Stay Modification, Room Move & Stay Extension
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 2026-10-03
- **Status:** Approved / Ready for Implementation
- **Target Role:** `receptionist`, `gm_admin`, `finance`, `housekeeping`
- **Lingkup:** Manajemen Transaksi Modifikasi Masa Menginap Tamu (*In-House Stay Lifecycle*)
- **Dokumen SRS Pasangan:** [SRS Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/stay-modification-room-move-and-extension-2026-10-03.md)
- **Dokumen Tech Architecture:** [Tech Architecture Stay Modification](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/stay-modification-room-move-architecture-2026-10-03.md)

---

## 1. Latar Belakang Bisnis & Urgensi Operasional

Hotel Pulang ke Uttara adalah hotel butik bintang 4 di Jl. Kaliurang, Sleman, D.I. Yogyakarta dengan total kapasitas 95 kamar fisik. Dalam operasional harian, masa menginap tamu (*in-house stay*) bersifat sangat dinamis dan kerap mengalami perubahan setelah proses check-in selesai:

1. **Pemindahan Kamar Fisik (*Mid-Stay Room Move*):**
   * Tamu sering meminta pindah kamar karena kendala teknis (AC tidak dingin, bau saluran air, kebisingan jalan) atau permintaan upgrade kenyamanan.
   * Tabel `room_assignments` hotel menggunakan PostgreSQL GiST exclusion constraint (`EXCLUDE USING GIST (room_number WITH =, stay_dates WITH &&)`). Database menolak tegas setiap penugasan kamar yang rentang tanggalnya bertabrakan.
   * Tanpa alur *room move* atomik, staf front desk tidak dapat memindahkan tamu tanpa risiko benturan tanggal (*exclusion violation*) atau integritas data yang rusak.
2. **Kesiapan Kamar Target (*Readiness Guard*):**
   * Sesuai standar operasional bintang 4, tamu hanya boleh dipindahkan ke kamar yang telah diperiksa dan disetujui supervisor (*inspected*). Pindah ke kamar kotor (*dirty*) atau sedang dibersihkan (*cleaning*) dilarang keras.
   * Kamar lama yang ditinggalkan wajib seketika bertransisi menjadi `vacant_dirty` agar tim Housekeeping segera menjadwalkan pembersihan ulang (*room turnover*).
3. **Perpanjangan Menginap (*Stay Extension*):**
   * Tamu sering memutuskan untuk memperpanjang waktu menginap 1 hingga beberapa malam tambahan.
   * Proses ini memerlukan verifikasi ketersediaan kuota inventaris, perhitungan tarif dinamis (*dynamic pricing*) untuk malam-malam baru, perpanjangan rentang tanggal penugasan kamar fisik, dan pembaruan total tagihan reservasi.

---

## 2. Profil Persona Pengguna & Matriks Hak Akses

| Role | Hak Akses | Batas Tanggung Jawab Operasional |
| :--- | :--- | :--- |
| **Resepsionis Meja Depan (`receptionist`)** | Eksekusi Pindah & Perpanjang | Memindahkan tamu ke kamar lain dalam tipe yang sama atau tipe yang tersedia; memproses perpanjangan menginap dengan konfirmasi penagihan. |
| **Housekeeping (`housekeeping`)** | Penerima Kamar Kotor & Kesiapan | Menyiapkan kamar target berstatus `inspected`; menerima status instan kamar lama berubah menjadi `vacant_dirty`. |
| **Staf Keuangan (`finance`)** | Audit Folio & Penagihan Delta | Memantau penambahan piutang/penerimaan kas dari biaya malam perpanjangan (*additional nights*). |
| **General Manager (`gm_admin`)** | Kontrol Penuh & Override | Melakukan investigasi keluhan tamu melalui riwayat log pemindahan kamar (*room move audit log*); menyetujui room move kompensasi. |
| **Tamu Publik (`guest`)** | Akses Ditolak (403 Forbidden) | Tidak memiliki wewenang memanipulasi penugasan kamar fisik hotel. |

---

## 3. Aturan Bisnis Inti (Business Rules)

1. **BR-STAY-01: Target Room Readiness Guard:**
   Kamar target pemindahan wajib berstatus `inspected`. Sistem menolak pemindahan ke kamar yang berstatus `vacant_dirty`, `cleaning`, `out_of_service`, atau `out_of_order` dengan kode `TARGET_ROOM_NOT_READY`.
2. **BR-STAY-02: Atomisitas GiST pada Pemotongan Tanggal:**
   Pemindahan kamar memotong rentang tanggal kamar asal menjadi `[check_in, move_date)` dan menambahkan penugasan kamar baru `[move_date, check_out)`. Apabila pemindahan terjadi di hari yang sama dengan check-in (`move_date == check_in`), penugasan kamar asal dihapus/ditiadakan.
3. **BR-STAY-03: Turnover Otomatis Kamar Lama:**
   Seketika setelah room move berhasil dilakukan, kamar asal wajib bertransisi menjadi `vacant_dirty` dengan catatan pembersihan `Moved to room {target}`.
4. **BR-STAY-04: Audit Trail Pemindahan Kamar:**
   Setiap eksekusi room move wajib mencatat data lengkap ke tabel `room_move_logs`: ID reservasi, kamar asal, kamar tujuan, tanggal perpindahan, kategori alasan (`maintenance_defect`, `noise_complaint`, `upgrade`, `guest_request`), catatan teknis, dan identitas staf.
5. **BR-STAY-05: Extension Inventory & Rate Enforcement:**
   Perpanjangan menginap wajib memverifikasi ketersediaan inventaris untuk malam-malam tambahan. Jika tersedia, kuota didekremen secara atomik dan tarif dihitung berdasarkan mesin tarif (*pricing engine*) hotel.
6. **BR-STAY-06: Extension Room Assignment Continuity:**
   Kamar fisik yang sedang ditempati diperpanjang rentang tanggalnya hingga `new_check_out`. Jika kamar fisik tersebut telah memiliki penugasan tamu lain pada tanggal perpanjangan, perpanjangan kamar yang sama ditolak (`ROOM_PHYSICAL_OVERLAP`).

---

## 4. Kriteria Keberterimaan (Acceptance Criteria)

- [ ] **AC-STAY-01 (Room Move Execution):** Resepsionis berhasil memindahkan tamu aktif (`checked_in`) ke kamar lain yang berstatus `inspected` tanpa melanggar GiST exclusion constraint.
- [ ] **AC-STAY-02 (Dirty Transition on Source Room):** Kamar asal yang ditinggalkan seketika berstatus `vacant_dirty`.
- [ ] **AC-STAY-03 (Rejection on Unready Target Room):** Pemindahan kamar ke kamar target yang belum `inspected` ditolak dengan HTTP 409 `TARGET_ROOM_NOT_READY`.
- [ ] **AC-STAY-04 (Audit Logging):** Riwayat perpindahan tersimpan di tabel `room_move_logs` dan dapat dibaca oleh staf berwenang.
- [ ] **AC-STAY-05 (Stay Extension Execution):** Resepsionis berhasil memperpanjang masa menginap tamu, kuota inventaris didekremen, tarif malam tambahan dihitung akurat, dan `check_out` booking diperbarui.
- [ ] **AC-STAY-06 (No Availability Rejection):** Perpanjangan menginap ditolak dengan HTTP 409 `NO_AVAILABILITY_FOR_EXTENSION` jika kuota kamar habis pada tanggal perpanjangan.
- [ ] **AC-STAY-07 (RBAC Isolation):** Tamu publik (`guest`) yang mencoba memanggil endpoint room move atau extend stay ditolak dengan HTTP 403 Forbidden.
