# PRD — Proposed 04: Guest Special Requests & Stay Assistance Desk
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 2026-10-03
- **Status:** Approved / Ready for Implementation
- **Target Role:** `guest`, `receptionist`, `housekeeping`, `gm_admin`
- **Lingkup:** Manajemen Permintaan Khusus & Personalisasi Tamu (*Guest Service Fulfillment*)
- **Dokumen SRS Pasangan:** [SRS Guest Assistance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/guest-special-requests-and-assistance-2026-10-03.md)
- **Dokumen Tech Architecture:** [Tech Architecture Guest Assistance](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/guest-special-requests-architecture-2026-10-03.md)

---

## 1. Latar Belakang & Urgensi Bisnis (Crucial Impact)

Hotel Pulang ke Uttara adalah hotel butik bintang 4 di Jl. Kaliurang Km 5.6 Yogyakarta yang mengedepankan kehangatan hunian khas Jogja dan pengalaman menginap personal. Dalam industri perhotelan bintang 4 modern:

1. **Kelemahan Catatan Teks Statis (*Static Text Bottleneck*):**
   * Sebelumnya, kolom `special_requests` pada tabel `bookings` hanya menyimpan teks mentah saat pemesanan dibuat di situs web.
   * Tidak ada pelacakan pemenuhan tugas (*fulfillment lifecycle*), sehingga staf Meja Depan dan Housekeeping sering kali melewatkan persiapan penting seperti setup bulan madu (*honeymoon swan towel*), boks bayi (*baby crib*), preferensi bantal, atau permintaan kamar bebas asap rokok di lantai atas.
2. **Kebutuhan Komunikasi Interaktif Tamu via Portal My Bookings:**
   * Tamu yang telah memesan kamar berhak menambahkan atau mengubah permintaan khusus mereka menjelang hari kedatangan tanpa harus menelepon hotel.
   * Tamu berhak mengetahui status permintaan mereka secara transparan: apakah sudah diterima (*acknowledged*), sudah disiapkan di kamar (*fulfilled*), atau terpaksa ditolak karena keterbatasan fisik kamar (*declined*, dengan catatan penjelasan dari staf hotel).
3. **Penyaluran Antrean Tugas Antar-Departemen (*Departmental Routing*):**
   * Permintaan perlengkapan fisik kamar (*baby crib*, handuk ekstra, dekorasi perayaan) secara otomatis diarahkan ke antrean tugas **Housekeeping**.
   * Permintaan jadwal kedatangan/keberangkatan (*early arrival*, *late departure*, transportasi bandara) secara otomatis diarahkan ke antrean tugas **Front Desk**.

---

## 2. Profil Persona & Matriks Hak Akses (RBAC Matrix)

| Persona | Akses & Otorisasi | Batas Tanggung Jawab Operasional |
| :--- | :--- | :--- |
| **Tamu Terverifikasi (`guest`)** | Sesi Privat (OTP Bearer) | Mengajukan permintaan khusus pada reservasi miliknya; melihat status pemenuhan dan catatan staf; membatalkan permintaan sendiri (Anti-IDOR). |
| **Resepsionis Meja Depan (`receptionist`)** | Staff JWT / Casbin RBAC | Meninjau antrean tugas Front Desk; menyetujui, menyiapkan, atau menolak permintaan jadwal; memperbarui status menjadi `acknowledged` atau `fulfilled`. |
| **Housekeeping (`housekeeping`)** | Staff JWT / Casbin RBAC | Meninjau antrean tugas Housekeeping; menyiapkan perlengkapan kamar fisik; memperbarui status menjadi `fulfilled`. |
| **General Manager (`gm_admin`)** | Full Control | Memantau seluruh antrean permintaan tamu lintas departemen; mengevaluasi tingkat kepuasan dan rasio pemenuhan tugas (*request fulfillment rate*). |

---

## 3. Aturan Bisnis Inti (Business Rules)

1. **BR-REQ-01: Anti-IDOR Request Submission:**
   Tamu hanya dapat mengajukan permintaan khusus pada booking yang terdaftar atas email yang sama dengan email sesi aktif tamu (`session.email == booking.guest_email`).
2. **BR-REQ-02: Structured Categorization & Auto-Routing:**
   Setiap permintaan diklasifikasikan ke dalam kategori resmi:
   - **Housekeeping:** `celebration_setup`, `baby_crib`, `quiet_room`, `high_floor`, `bed_type`.
   - **Front Desk:** `early_arrival`, `late_departure`, `dietary_allergy`, `other`.
3. **BR-REQ-03: Lifecycle State Machine:**
   Status permintaan bergerak secara teratur:
   - `pending` (baru diajukan tamu) $\rightarrow$ `acknowledged` (dilihat dan dicatat staf) $\rightarrow$ `fulfilled` (selesai disiapkan)
   - `pending` / `acknowledged` $\rightarrow$ `declined` (ditolak dengan catatan staf wajib).
4. **BR-REQ-04: Non-Guaranteed Courtesy Service:**
   Seluruh permintaan khusus bersifat kesediaan hotel (*subject to room availability*). Staf wajib memberikan catatan alasan yang ramah dan jelas saat menolak (*declined*) permintaan tamu.

---

## 4. Kriteria Keberterimaan (Acceptance Criteria)

- [ ] **AC-REQ-01 (Guest Submission):** Tamu dapat mengajukan permintaan khusus melalui `POST /api/v1/guest/bookings/{id}/special-requests` dan sistem otomatis menetapkan departemen yang bertanggung jawab.
- [ ] **AC-REQ-02 (Anti-IDOR Defense):** Tamu yang mencoba mengajukan permintaan pada booking milik tamu lain ditolak dengan HTTP 404 Not Found.
- [ ] **AC-REQ-03 (Guest Inquiries):** Tamu dapat melihat daftar permintaan khusus dan catatan tindak lanjut staf melalui `GET /api/v1/guest/bookings/{id}/special-requests`.
- [ ] **AC-REQ-04 (Departmental Queue):** Staf Front Desk dan Housekeeping dapat memfilter antrean tugas berdasarkan departemen dan status pemenuhan melalui `GET /api/v1/front-desk/special-requests`.
- [ ] **AC-REQ-05 (Status Transition):** Staf dapat memperbarui status pemenuhan melalui `PUT /api/v1/front-desk/special-requests/{id}/status` dengan catatan staf (*staff notes*).
- [ ] **AC-REQ-06 (RBAC Protection):** Tamu publik (`guest`) yang mencoba mengakses endpoint antrean staf ditolak dengan HTTP 403 Forbidden.
