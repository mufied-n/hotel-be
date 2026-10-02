# End-to-End (E2E) Test Report: Room Variant Catalog CRUD Management & Batch BE-B Parity
**Properti:** Hotel Pulang ke Uttara, Yogyakarta (95 Kamar)  
**Tanggal Pengujian:** 2026-10-03 02:20:00 WIB  
**Status Eksekusi:** PASS (100% Berhasil)  
**Artefak Terkait:**
- PRD: [catalog-crud-management-2026-10-03.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/prd/catalog-crud-management-2026-10-03.md)
- SRS: [catalog-crud-management-2026-10-03.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/catalog-crud-management-2026-10-03.md)
- Tech Architecture: [catalog-crud-architecture-2026-10-03.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/tech/catalog-crud-architecture-2026-10-03.md)
- Walkthrough: [catalog-crud-walkthrough-2026-10-03.md](file:///mnt/code/projects/jobs/pulang/current-booking/docs/walkthrough/catalog-crud-walkthrough-2026-10-03.md)
- Skrip Pengujian: [e2e_runner_test.go](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/e2e_runner_test.go) & [hotel_booking_rbac_e2e.sh](file:///mnt/code/projects/jobs/pulang/current-booking/testing/e2e/script/hotel_booking_rbac_e2e.sh)

---

## 1. Ringkasan Eksekutif

Menindaklanjuti permintaan pengujian dan verifikasi terhadap fitur **Room Variant Catalog CRUD Management** serta penyelesaian gap batch BE-B (BE-G01, BE-G02, BE-G03, BE-G13, BE-G14), seluruh rangkaian pengujian integrasi unit dan End-to-End (E2E) telah dijalankan secara komprehensif.

Fitur ini mengeliminasi ketergantungan pada inisialisasi hardcode statis dan membuka kapabilitas administratif RESTful CRUD penuh bagi staf internal hotel (`revenue_mgr` dan `gm_admin`), sembari memastikan proteksi granular berbasis Casbin RBAC bagi publik/tamu (`guest`).

---

## 2. Matriks Pengujian & Cakupan Skenario

| ID Skenario | Endpoint & Aksi | Peran / Subjek | Harapan (Status Code) | Hasil Aktual | Status |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **E2E-01** | `GET /healthz` | Anonymous | 200 OK | 200 OK | **PASS** |
| **E2E-02** | `GET /api/v1/availability` | Anonymous | 200 OK | 200 OK | **PASS** |
| **E2E-02A**| `GET /api/v1/catalog/rooms` | Guest | 200 OK (7 Sellable Variants) | 200 OK (7 variants) | **PASS** |
| **E2E-02B**| `GET /api/v1/search` | Guest | 200 OK (Multi-night search) | 200 OK (Continuous) | **PASS** |
| **E2E-02C**| `GET /api/v1/search` (LOS > 30) | Guest | 400 Bad Request (Exceed LOS) | 400 Bad Request | **PASS** |
| **E2E-02D**| `GET /api/v1/search` (Age > 17) | Guest | 400 Bad Request (Invalid Age) | 400 Bad Request | **PASS** |
| **E2E-02E**| `POST /api/v1/catalog/rooms` | Revenue Manager | 201 Created (`villa-garden`) | 201 Created | **PASS** |
| **E2E-02F**| `PUT /api/v1/catalog/rooms/:id` | Revenue Manager | 200 OK (Harga & Nama baru) | 200 OK | **PASS** |
| **E2E-02G**| `GET /api/v1/catalog/rooms/:id` | Public Guest | 200 OK (Harga terupdate) | 200 OK | **PASS** |
| **E2E-02H**| `POST/PUT/DELETE /catalog/rooms` | Guest | 403 Forbidden | 403 Forbidden | **PASS** |
| **E2E-02H**| `DELETE /catalog/rooms/:id` | Revenue Manager | 403 Forbidden (Non-admin) | 403 Forbidden | **PASS** |
| **E2E-02I**| `DELETE /catalog/rooms/:id` | GM Admin | 200 OK & subsequent 404 | 200 OK -> 404 Not Found | **PASS** |
| **E2E-03** | `POST /api/v1/bookings` | Guest | 201 Created (Hold booking) | 201 Created | **PASS** |
| **E2E-04** | `POST /api/v1/bookings/:id/check-in` | Guest | 403 Forbidden | 403 Forbidden | **PASS** |
| **E2E-05** | `POST /fake-pay/:ref` | Guest | 200 OK (Confirmed) | 200 OK | **PASS** |
| **E2E-06** | `POST /api/v1/bookings/:id/check-in` | Receptionist | 200 OK (Checked-in) | 200 OK | **PASS** |
| **E2E-07** | `POST /api/v1/bookings/:id/check-out`| Housekeeping | 403 Forbidden | 403 Forbidden | **PASS** |
| **E2E-08** | `POST /api/v1/bookings/:id/check-out`| Receptionist | 200 OK (Checked-out) | 200 OK | **PASS** |
| **E2E-09** | `GET /api/v1/bookings/:id` | GM Admin | 200 OK (Full record) | 200 OK | **PASS** |
| **E2E-10** | `GET /api/v1/bookings/:id` | Public Guest | 200 OK (Masked DTO, no PII) | 200 OK (Zero PII leak) | **PASS** |
| **E2E-11** | `GET /api/v1/bookings/:id` (Token) | Guest w/ Token | 200 OK (Full PII DTO) | 200 OK (Full Name/Email) | **PASS** |
| **E2E-12** | `POST /api/v1/bookings/:id/cancel` | Invalid Token | 403 Forbidden | 403 Forbidden | **PASS** |
| **E2E-13** | `POST /fake-pay/:ref` (Prod Mode) | Public | 404 Not Found (Dev-only gate)| 404 Not Found | **PASS** |

---

## 3. Hasil Pengujian Unit & Statement Code Coverage

Sesuai standar baku rekayasa perangkat lunak pada repositori ini, seluruh paket baru dan paket yang mengalami refinement wajib melampaui batas ambang minimum $\ge 80\%$ statement code coverage:

```text
ok      github.com/example/hotel-booking/internal/catalog         0.003s   coverage: 93.7% of statements
ok      github.com/example/hotel-booking/internal/api             0.015s   coverage: 88.2% of statements
ok      github.com/example/hotel-booking/internal/platform/auth   0.005s   coverage: 80.9% of statements
ok      github.com/example/hotel-booking/internal/rates           0.002s   coverage: 91.7% of statements
ok      github.com/example/hotel-booking/testing/e2e/script       0.013s   coverage: [no statements]
```

Hasil verifikasi linter dan static analysis:
```bash
$ go vet ./...
# Status: EXIT 0 (Zero warnings, Zero errors)
```

---

## 4. Log Eksekusi E2E Otomatis

```text
=== RUN   TestEndToEndHotelBookingRBACLifecycle
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-01:_Health_check
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02:_Public_search_availability
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02A:_Public_catalog_room_discovery_(7_sellable_variants,_95_rooms)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02B:_Multi-night_cross-variant_search
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02C:_Search_validation_reject_stay_>_30_nights_(400_Bad_Request)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02D:_Search_validation_reject_child_age_>_17_(400_Bad_Request)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02E:_Revenue_Manager_creates_room_variant_(201_Created)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02F:_Revenue_Manager_updates_room_variant_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02G:_Public_Guest_views_single_room_variant_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02H:_Catalog_RBAC_Negative_Tests_(403_Forbidden)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-02I:_GM_Admin_deletes_room_variant_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-03:_Public_create_booking_hold
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-04:_Guest_cannot_check-in_(403_Forbidden)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-05:_Payment_confirmation
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-06:_Receptionist_check-in_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-07:_Housekeeping_cannot_check-out_(403_Forbidden)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-08:_Receptionist_check-out_via_Bearer_token_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-09:_GM_Admin_inspect_booking_(200_OK)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-10:_Public_guest_receives_masked_PublicDTO_(no_PII)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-11:_Guest_with_X-Guest-Token_receives_full_PII
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-12:_Guest_with_invalid_token_cannot_cancel_booking_(403_Forbidden)
=== RUN   TestEndToEndHotelBookingRBACLifecycle/E2E-13:_Production_mode_gates_/fake-pay_(404_Not_Found)
--- PASS: TestEndToEndHotelBookingRBACLifecycle (0.01s)
PASS
ok      github.com/example/hotel-booking/testing/e2e/script       0.013s
```

---

## 5. Kepatuhan Prinsip Desain (Ponytail & Best Practices)

1. **Anti-Overengineering (Ponytail Lite):**
   - Menggunakan pure Go standard library (`crypto/rand`, `encoding/json`, `sync.RWMutex`).
   - Tidak menambahkan external dependency baru atau ORM wrapper.
   - Implementasi adapter dual-mode: `MemoryStore` (pengujian cepat tanpa DB) dan `PostgresStore` (query parameterized langsung dengan penanganan kode constraint Postgres `23505` dan `23503`).
2. **Keamanan & Standar RFC:**
   - Error ditranslasikan ke format standar **RFC 7807 Problem Details** (`INVALID_ROOM_PAYLOAD`, `CONFLICT_ROOM_CODE`, `CANNOT_DELETE_ACTIVE_VARIANT`, `FORBIDDEN_CASBIN`).
   - Prinsip *Least Privilege* ditegakkan: `revenue_mgr` hanya berhak `POST` dan `PUT`, sementara operasi destruktif `DELETE` dilindungi khusus untuk `gm_admin`.

---

## 6. Kesimpulan & Rekomendasi

Implementasi fitur Room Variant Catalog CRUD Management dan pemenuhan seluruh gap Batch BE-B telah selesai dengan status **SEMPURNA (All Passed)**. Seluruh target coverage ($\ge 80\%$) dan kriteria penerimaan terpenuhi tanpa ada regresi pada fitur yang telah ada sebelumnya. Siap dilakukan commit ke repository.
