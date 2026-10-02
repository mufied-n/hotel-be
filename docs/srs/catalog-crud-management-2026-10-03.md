# Software Requirements Specification (SRS) — Room Variant Catalog CRUD Management
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)

- **Tanggal Dokumen:** 3 Oktober 2026
- **Standar Format:** RFC 7807 Problem Details, RESTful JSON
- **Modul:** `internal/catalog`, `internal/api`

---

## 1. Kebutuhan Fungsional (Functional Requirements)

- **FR-01: List Catalog Rooms**
  - **Path:** `GET /api/v1/catalog/rooms`
  - **Auth:** Public (`guest`) & Authenticated staff.
  - **Response 200 OK:**
    ```json
    {
      "total": 7,
      "rooms": [
        {
          "id": "01900000-0000-7000-8000-000000000001",
          "code": "sup-king",
          "name": "Superior King Room",
          "family_name": "Superior",
          "bed_type": "1 King Bed",
          "room_size_sqm": 28,
          "max_capacity": 3,
          "max_adults": 2,
          "max_children": 1,
          "description": "Kamar Superior modern...",
          "base_price_minor": 550000,
          "amenities": ["Free High-Speed Wi-Fi", "Air Conditioning"],
          "photos": [{"url": "https://...", "alt": "Superior King Bedroom"}]
        }
      ]
    }
    ```

- **FR-02: Get Catalog Room by ID or Code**
  - **Path:** `GET /api/v1/catalog/rooms/{id}`
  - **Auth:** Public (`guest`) & Authenticated staff.
  - **Response 200 OK:** Objek `RoomVariant`.
  - **Response 404 Not Found:** `{"code": "ROOM_VARIANT_NOT_FOUND"}`.

- **FR-03: Create Catalog Room Variant**
  - **Path:** `POST /api/v1/catalog/rooms`
  - **Auth:** `revenue_mgr`, `gm_admin`.
  - **Request Body:**
    ```json
    {
      "code": "vlla-pool",
      "name": "Pool Villa Suite",
      "family_name": "Villa",
      "bed_type": "1 Super King Bed",
      "room_size_sqm": 85,
      "max_capacity": 4,
      "max_adults": 3,
      "max_children": 2,
      "description": "Villa eksklusif dengan kolam renang pribadi.",
      "base_price_minor": 2750000,
      "amenities": ["Private Pool", "Butler Service"],
      "photos": [{"url": "https://...", "alt": "Villa Pool"}]
    }
    ```
  - **Response 201 Created:** Objek `RoomVariant` baru dengan generated UUID v7.
  - **Validation Errors (400 Bad Request):**
    - `code` kosong atau sudah ada -> `INVALID_CODE` / `CONFLICT_CODE`
    - `name` kosong -> `INVALID_NAME`
    - `max_capacity < 1` -> `INVALID_CAPACITY`
    - `base_price_minor <= 0` -> `INVALID_PRICE`

- **FR-04: Update Catalog Room Variant**
  - **Path:** `PUT /api/v1/catalog/rooms/{id}`
  - **Auth:** `revenue_mgr`, `gm_admin`.
  - **Request Body:** Payload pembaruan seluruh atribut `RoomVariant`.
  - **Response 200 OK:** Objek `RoomVariant` yang telah diperbarui.
  - **Response 404 Not Found:** `{"code": "ROOM_VARIANT_NOT_FOUND"}`.

- **FR-05: Delete Catalog Room Variant**
  - **Path:** `DELETE /api/v1/catalog/rooms/{id}`
  - **Auth:** `gm_admin`.
  - **Response 200 OK:** `{"status": "deleted", "id": "..."}`.
  - **Response 409 Conflict:** Jika tipe kamar memiliki referensi fisik di `rooms` atau pemesanan di `bookings` -> `CANNOT_DELETE_ACTIVE_VARIANT`.
  - **Response 404 Not Found:** `{"code": "ROOM_VARIANT_NOT_FOUND"}`.

---

## 2. Definisi Kode Kesalahan (RFC 7807)

| Error Code | HTTP Status | Keterangan |
| :--- | :---: | :--- |
| `ROOM_VARIANT_NOT_FOUND` | 404 | ID atau kode varian kamar tidak ditemukan di katalog |
| `INVALID_ROOM_PAYLOAD` | 400 | Format JSON body tidak valid |
| `INVALID_ROOM_DATA` | 400 | Data wajib seperti kode, nama, atau kapasitas tidak memenuhi syarat |
| `CONFLICT_ROOM_CODE` | 409 | Kode varian kamar (`code`) sudah terdaftar |
| `CANNOT_DELETE_ACTIVE_VARIANT` | 409 | Varian tidak dapat dihapus karena memiliki unit kamar atau reservasi aktif |
| `FORBIDDEN` | 403 | Peran pengguna tidak memiliki izin mengeksekusi operasi katalog |
