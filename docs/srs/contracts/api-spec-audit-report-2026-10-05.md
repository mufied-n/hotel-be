# Laporan Audit Spesifikasi API (OpenAPI 3.1) vs Implementasi Handler HTTP
# Pulang ke Uttara — Hotel Booking Engine (Yogyakarta)

Dokumen ini memuat hasil audit komprehensif atas spesifikasi kontrak API ([`docs/srs/contracts/pulang-hotel-booking.openapi.json`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/contracts/pulang-hotel-booking.openapi.json)) terhadap seluruh 69 operasi rute HTTP yang terdaftar pada router produksi ([`internal/api/http/testdata/routes.golden`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/testdata/routes.golden)) dan diimplementasikan pada package [`internal/api/http/handler`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler).

---

## 1. Ringkasan Eksekutif (Executive Summary)

* **Tanggal & Waktu Audit**: 5 Oktober 2026.
* **Tujuan Audit**: Memastikan akurasi 100% antara spesifikasi OpenAPI 3.1 dan kode sumber handler HTTP aktual, menyelesaikan ketimpangan historis dari draf proposal awal, serta memastikan kepatuhan terhadap standar keamanan Casbin RBAC, format error RFC 7807, dan toggle runtime Feature Flags.
* **Hasil Verifikasi Linter (Redocly CLI)**:
  ```text
  No configurations were provided -- using built in recommended configuration by default.
  validating docs/srs/contracts/pulang-hotel-booking.openapi.json...
  docs/srs/contracts/pulang-hotel-booking.openapi.json: validated in 89ms
  Woohoo! Your API description is valid. 🎉 (0 errors, 0 warnings)
  ```
* **Hasil Verifikasi Paritas Rute (Router vs OpenAPI)**:
  * **Rute Golden Router**: 69 operasi.
  * **Operasi OpenAPI**: 69 operasi.
  * **Rute Hilang / Missing**: 0.
  * **Rute Ekstra / Mismatch**: 0.
  * **Persentase Keselarasan**: **100.0% Exact Match**.

---

## 2. Rekonsiliasi & Riwayat Draf Proposal

Sebelum audit ini dilaksanakan, repositori memiliki dokumen proposal awal:
* [`docs/srs/contracts/booking-roadmap-proposal.openapi.json`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/contracts/booking-roadmap-proposal.openapi.json) (dibuat 3 Oktober 2026, versi `0.1.0-proposal`).
  * **Karakteristik**: Merupakan draf desain awal sebelum implementasi Go selesai. Memuat 38 operasi abstrak (seperti `POST /booking-searches`, `POST /guest-access-challenges`, dsb.) tanpa awalan rute standar `/api/v1/`, belum memetakan middleware Casbin RBAC, dan belum mencakup fitur-fitur lanjutan Batch 2026-10-04 (SSE Event Hub, OTA channel synchronization, PDF invoice Sleman PBJT, last-room complimentary upgrades, dan feature flags).
* **Keputusan Arsitektur**:
  1. Spesifikasi [`docs/srs/contracts/pulang-hotel-booking.openapi.json`](file:///mnt/code/projects/jobs/pulang/current-booking/docs/srs/contracts/pulang-hotel-booking.openapi.json) ditetapkan sebagai **Spesifikasi Kontrak Kanonikal Resmi (Source of Truth)** untuk antarmuka HTTP backend.
  2. Dokumen proposal awal tetap diarsipkan sebagai referensi konseptual fase inisiasi.

---

## 3. Matriks Audit Lengkap 69 Rute HTTP vs OpenAPI 3.1

Berikut adalah rincian pemetaan 69 operasi rute HTTP aktual di router Gin Go terhadap spesifikasi OpenAPI 3.1:

### 3.1 Health & Probes (4 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /healthz` | `GET /healthz` | [`Healthz`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go#L29) | `get_healthz` | Public (`[]`) | None | 200, 400 |
| `HEAD /healthz` | `HEAD /healthz` | [`Healthz`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go#L30) | `head_healthz` | Public (`[]`) | None | 200, 400 |
| `GET /ready` | `GET /ready` | [`Ready`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go#L31) | `get_ready` | Public (`[]`) | None | 200, 400, 503 |
| `HEAD /ready` | `HEAD /ready` | [`Ready`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go#L32) | `head_ready` | Public (`[]`) | None | 200, 400, 503 |

### 3.2 Webhook & Integrasi Pembayaran (2 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /api/v1/webhooks/xendit` | `POST /api/v1/webhooks/xendit` | [`HandleXenditWebhook`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/webhook.go#L15) | `post_webhooks_xendit` | Public (`[]`) | `ff_xendit_payment_gateway` | 200, 400, 503 |
| `POST /api/v1/channel-events` | `POST /api/v1/channel-events` | [`HandleChannelEventWebhook`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/channel.go#L18) | `post_channel_events` | `ChannelWebhookHMAC` | `ff_channel_sync_integration` | 200, 400, 401, 503 |

### 3.3 Autentikasi Tamu / Passwordless OTP (4 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /api/v1/auth/guest/challenge` | `POST /api/v1/auth/guest/challenge` | [`GuestChallenge`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_auth.go#L40) | `post_auth_guest_challenge` | Public (`[]`) | `ff_guest_portal_auth` | 200, 400, 503 |
| `POST /api/v1/auth/guest/verify` | `POST /api/v1/auth/guest/verify` | [`GuestVerify`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_auth.go#L85) | `post_auth_guest_verify` | Public (`[]`) | `ff_guest_portal_auth` | 200, 400, 401, 503 |
| `GET /api/v1/auth/guest/me` | `GET /api/v1/auth/guest/me` | [`GuestMe`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_auth.go#L140) | `get_auth_guest_me` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 503 |
| `POST /api/v1/auth/guest/logout` | `POST /api/v1/auth/guest/logout` | [`GuestLogout`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_auth.go#L159) | `post_auth_guest_logout` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 503 |

### 3.4 Autentikasi Staf Hotel (3 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /api/v1/auth/staff/login` | `POST /api/v1/auth/staff/login` | [`StaffLogin`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/staff_auth.go#L20) | `post_auth_staff_login` | Public (`[]`) | None | 200, 400, 401 |
| `GET /api/v1/auth/staff/me` | `GET /api/v1/auth/staff/me` | [`StaffMe`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/staff_auth.go#L60) | `get_auth_staff_me` | `StaffBearerAuth` | None | 200, 401 |
| `POST /api/v1/auth/staff/logout` | `POST /api/v1/auth/staff/logout` | [`StaffLogout`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/staff_auth.go#L76) | `post_auth_staff_logout` | `StaffBearerAuth` | None | 204, 401 |

### 3.5 Katalog Kamar, Ketersediaan & Quote (7 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/catalog/rooms` | `GET /api/v1/catalog/rooms` | [`GetCatalogRooms`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/catalog.go#L12) | `get_catalog_rooms` | Public (`[]`) | None | 200, 400 |
| `GET /api/v1/catalog/rooms/:id` | `GET /api/v1/catalog/rooms/:id` | [`GetCatalogRoom`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/catalog.go#L27) | `get_catalog_rooms_id` | Public (`[]`) | None | 200, 400, 404 |
| `POST /api/v1/catalog/rooms` | `POST /api/v1/catalog/rooms` | [`CreateCatalogRoom`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/catalog.go#L40) | `post_catalog_rooms` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_catalog_write` | 201, 400, 403, 503 |
| `PUT /api/v1/catalog/rooms/:id` | `PUT /api/v1/catalog/rooms/:id` | [`UpdateCatalogRoom`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/catalog.go#L59) | `put_catalog_rooms_id` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_catalog_write` | 200, 400, 403, 404, 503 |
| `DELETE /api/v1/catalog/rooms/:id` | `DELETE /api/v1/catalog/rooms/:id` | [`DeleteCatalogRoom`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/catalog.go#L79) | `delete_catalog_rooms_id` | `StaffBearerAuth` (`gm_admin`) | `ff_catalog_write` | 200, 400, 403, 404, 503 |
| `GET /api/v1/search` | `GET /api/v1/search` | [`SearchRooms`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/search.go#L15) | `get_search` | Public (`[]`) | `ff_multi_variant_search` | 200, 400, 503 |
| `GET /api/v1/availability` | `GET /api/v1/availability` | [`GetAvailability`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/availability.go#L12) | `get_availability` | Public (`[]`) | None | 200, 400 |

### 3.6 Siklus Hidup Pemesanan Kamar (Bookings) (10 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /api/v1/quotes` | `POST /api/v1/quotes` | [`CreateQuote`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/quote.go#L25) | `post_quotes` | Public (`[]`) | None | 200, 400, 404 |
| `POST /api/v1/bookings` | `POST /api/v1/bookings` | [`CreateBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L36) | `post_bookings` | Public (`[]`) | None | 201, 400, 409 |
| `GET /api/v1/bookings/:id` | `GET /api/v1/bookings/:id` | [`GetBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L130) | `get_bookings_id` | Public / Sesi Staf / Token Tamu | None | 200, 400, 404 |
| `GET /api/v1/bookings/:id/payment` | `GET /api/v1/bookings/:id/payment` | [`GetPaymentStatus`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L170) | `get_bookings_id_payment` | Public / Sesi Staf / Token Tamu | None | 200, 400, 404 |
| `GET /api/v1/bookings/:id/voucher.pdf` | `GET /api/v1/bookings/:id/voucher.pdf` | [`DownloadVoucherPDF`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/voucher_invoice.go#L50) | `get_bookings_id_voucher_pdf` | Public / Sesi Staf / Token Tamu | `ff_official_pdf_documents` | 200 (`application/pdf`), 400, 404, 503 |
| `GET /api/v1/bookings/:id/invoice.pdf` | `GET /api/v1/bookings/:id/invoice.pdf` | [`DownloadInvoicePDF`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/voucher_invoice.go#L110) | `get_bookings_id_invoice_pdf` | Public / Sesi Staf / Token Tamu | `ff_official_pdf_documents` | 200 (`application/pdf`), 400, 404, 503 |
| `POST /api/v1/bookings/:id/cancel` | `POST /api/v1/bookings/:id/cancel` | [`CancelBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L200) | `post_bookings_id_cancel` | Public / Sesi Staf / Token Tamu | None | 200, 400, 404, 409 |
| `POST /api/v1/bookings/:id/check-in` | `POST /api/v1/bookings/:id/check-in` | [`CheckInBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L235) | `post_bookings_id_check_in` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403, 404, 409 |
| `POST /api/v1/bookings/:id/check-out` | `POST /api/v1/bookings/:id/check-out` | [`CheckOutBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L270) | `post_bookings_id_check_out` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403, 404, 409 |
| `POST /api/v1/bookings/:id/no-show` | `POST /api/v1/bookings/:id/no-show` | [`NoShowBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/booking.go#L300) | `post_bookings_id_no_show` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403, 404, 409 |

### 3.7 Guest Portal Privat (11 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/guest/bookings` | `GET /api/v1/guest/bookings` | [`ListGuestBookings`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L20) | `get_guest_bookings` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 503 |
| `GET /api/v1/guest/bookings/:id` | `GET /api/v1/guest/bookings/:id` | [`GetGuestBookingDetail`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L50) | `get_guest_bookings_id` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/payment` | `GET /api/v1/guest/bookings/:id/payment` | [`GetGuestBookingPayment`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L80) | `get_guest_bookings_id_payment` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/receipt` | `GET /api/v1/guest/bookings/:id/receipt` | [`GetGuestBookingReceipt`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L110) | `get_guest_bookings_id_receipt` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/calendar.ics` | `GET /api/v1/guest/bookings/:id/calendar.ics` | [`DownloadGuestCalendarICS`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L140) | `get_guest_bookings_id_calendar_ics` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200 (`text/calendar`), 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/voucher.pdf` | `GET /api/v1/guest/bookings/:id/voucher.pdf` | [`DownloadGuestVoucherPDF`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L170) | `get_guest_bookings_id_voucher_pdf` | `GuestSessionAuth` | `ff_official_pdf_documents` | 200 (`application/pdf`), 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/invoice.pdf` | `GET /api/v1/guest/bookings/:id/invoice.pdf` | [`DownloadGuestInvoicePDF`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L200) | `get_guest_bookings_id_invoice_pdf` | `GuestSessionAuth` | `ff_official_pdf_documents` | 200 (`application/pdf`), 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/live-status` | `GET /api/v1/guest/bookings/:id/live-status` | [`GuestLiveStatusSSE`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/live_stream.go#L25) | `get_guest_bookings_id_live_status` | `GuestSessionAuth` | `ff_realtime_event_hub` | 200 (`text/event-stream`), 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/refund-status` | `GET /api/v1/guest/bookings/:id/refund-status` | [`GetGuestRefundStatus`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/guest_portal.go#L230) | `get_guest_bookings_id_refund_status` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 404, 503 |
| `POST /api/v1/guest/bookings/:id/special-requests` | `POST /api/v1/guest/bookings/:id/special-requests` | [`CreateGuestSpecialRequest`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/special_requests.go#L25) | `post_guest_bookings_id_special_requests` | `GuestSessionAuth` | `ff_guest_portal_auth` | 201, 400, 401, 404, 503 |
| `GET /api/v1/guest/bookings/:id/special-requests` | `GET /api/v1/guest/bookings/:id/special-requests` | [`ListGuestSpecialRequests`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/special_requests.go#L60) | `get_guest_bookings_id_special_requests` | `GuestSessionAuth` | `ff_guest_portal_auth` | 200, 401, 404, 503 |

### 3.8 Front Desk & Meja Depan (7 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/front-desk/daily-roster` | `GET /api/v1/front-desk/daily-roster` | [`GetDailyRoster`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/frontdesk.go#L20) | `get_front_desk_daily_roster` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403 |
| `GET /api/v1/front-desk/handover-notes` | `GET /api/v1/front-desk/handover-notes` | [`ListHandoverNotes`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/frontdesk.go#L50) | `get_front_desk_handover_notes` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403 |
| `POST /api/v1/front-desk/handover-notes` | `POST /api/v1/front-desk/handover-notes` | [`CreateHandoverNote`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/frontdesk.go#L80) | `post_front_desk_handover_notes` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 201, 400, 403 |
| `GET /api/v1/front-desk/verify-voucher` | `GET /api/v1/front-desk/verify-voucher` | [`VerifyVoucher`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/frontdesk.go#L110) | `get_front_desk_verify_voucher` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403, 404 |
| `GET /api/v1/front-desk/live-stream` | `GET /api/v1/front-desk/live-stream` | [`FrontDeskLiveStreamSSE`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/live_stream.go#L60) | `get_front_desk_live_stream` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | `ff_realtime_event_hub` | 200 (`text/event-stream`), 400, 403, 503 |
| `GET /api/v1/front-desk/special-requests` | `GET /api/v1/front-desk/special-requests` | [`StaffListSpecialRequests`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/special_requests.go#L90) | `get_front_desk_special_requests` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403 |
| `PUT /api/v1/front-desk/special-requests/:id/status` | `PUT /api/v1/front-desk/special-requests/:id/status` | [`StaffUpdateSpecialRequestStatus`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/special_requests.go#L120) | `put_front_desk_special_requests_id_status` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | None | 200, 400, 403, 404 |

### 3.9 Stay Modifications / Mid-Stay (3 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /api/v1/bookings/:id/room-move` | `POST /api/v1/bookings/:id/room-move` | [`RoomMoveBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/stay.go#L20) | `post_bookings_id_room_move` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | `ff_room_move_extension` | 200, 400, 403, 404, 409, 503 |
| `POST /api/v1/bookings/:id/extend-stay` | `POST /api/v1/bookings/:id/extend-stay` | [`ExtendStayBooking`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/stay.go#L70) | `post_bookings_id_extend_stay` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | `ff_room_move_extension` | 200, 400, 403, 404, 409, 503 |
| `GET /api/v1/bookings/:id/room-moves` | `GET /api/v1/bookings/:id/room-moves` | [`ListBookingRoomMoves`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/stay.go#L120) | `get_bookings_id_room_moves` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | `ff_room_move_extension` | 200, 400, 403, 404, 503 |

### 3.10 Housekeeping (3 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/housekeeping/rooms` | `GET /api/v1/housekeeping/rooms` | [`ListHousekeepingRooms`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/housekeeping.go#L20) | `get_housekeeping_rooms` | `StaffBearerAuth` (`housekeeping`, `receptionist`, `gm_admin`) | None | 200, 400, 403 |
| `PUT /api/v1/housekeeping/rooms/:id/status` | `PUT /api/v1/housekeeping/rooms/:id/status` | [`UpdateHousekeepingRoomStatus`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/housekeeping.go#L50) | `put_housekeeping_rooms_id_status` | `StaffBearerAuth` (`housekeeping`, `receptionist`, `gm_admin`) | None | 200, 400, 403, 404 |
| `POST /api/v1/housekeeping/rooms/:id/out-of-order` | `POST /api/v1/housekeeping/rooms/:id/out-of-order` | [`SetRoomOutOfOrder`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/housekeeping.go#L80) | `post_housekeeping_rooms_id_out_of_order` | `StaffBearerAuth` (`housekeeping`, `receptionist`, `gm_admin`) | None | 200, 400, 403, 404 |

### 3.11 Keuangan & Rekonsiliasi (Finance) (4 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /api/v1/finance/refunds` | `POST /api/v1/finance/refunds` | [`ProcessRefund`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/finance.go#L20) | `post_finance_refunds` | `StaffBearerAuth` (`finance`, `gm_admin`) | None | 200, 400, 403, 404 |
| `GET /api/v1/finance/cases` | `GET /api/v1/finance/cases` | [`ListFinanceCases`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/finance.go#L60) | `get_finance_cases` | `StaffBearerAuth` (`finance`, `gm_admin`) | None | 200, 400, 403 |
| `POST /api/v1/finance/cases/:id/resolve` | `POST /api/v1/finance/cases/:id/resolve` | [`ResolveFinanceCase`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/finance.go#L90) | `post_finance_cases_id_resolve` | `StaffBearerAuth` (`finance`, `gm_admin`) | None | 200, 400, 403, 404 |
| `GET /api/v1/finance/reconciliations` | `GET /api/v1/finance/reconciliations` | [`GetReconciliations`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/finance.go#L120) | `get_finance_reconciliations` | `StaffBearerAuth` (`finance`, `gm_admin`) | None | 200, 400, 403 |

### 3.12 Manajemen Pendapatan & Promo (Revenue Management) (5 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/revenue/calendar` | `GET /api/v1/revenue/calendar` | [`GetRevenueCalendar`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/revenue.go#L20) | `get_revenue_calendar` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_dynamic_pricing_rules` | 200, 400, 403, 503 |
| `PUT /api/v1/revenue/calendar/bulk` | `PUT /api/v1/revenue/calendar/bulk` | [`BulkUpdateRates`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/revenue.go#L70) | `put_revenue_calendar_bulk` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_dynamic_pricing_rules` | 200, 400, 403, 503 |
| `GET /api/v1/revenue/promos` | `GET /api/v1/revenue/promos` | [`ListPromos`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/revenue.go#L110) | `get_revenue_promos` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_promotions_engine` | 200, 400, 403, 503 |
| `POST /api/v1/revenue/promos` | `POST /api/v1/revenue/promos` | [`CreatePromo`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/revenue.go#L140) | `post_revenue_promos` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_promotions_engine` | 201, 400, 403, 503 |
| `PUT /api/v1/revenue/promos/:id` | `PUT /api/v1/revenue/promos/:id` | [`UpdatePromo`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/revenue.go#L170) | `put_revenue_promos_id` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_promotions_engine` | 200, 400, 403, 404, 503 |

### 3.13 Staf Integrasi Kanal & Penyelesaian Sengketa OTA (3 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/staff/channel-sync-issues` | `GET /api/v1/staff/channel-sync-issues` | [`ListChannelSyncIssues`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/channel.go#L70) | `get_staff_channel_sync_issues` | `StaffBearerAuth` (`receptionist`, `revenue_mgr`, `gm_admin`) | `ff_channel_sync_integration` | 200, 400, 403, 503 |
| `POST /api/v1/staff/channel-sync-issues/:id/resolve` | `POST /api/v1/staff/channel-sync-issues/:id/resolve` | [`ResolveChannelSyncIssue`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/channel.go#L105) | `post_staff_channel_sync_issues_id_resolve` | `StaffBearerAuth` (`receptionist`, `gm_admin`) | `ff_channel_sync_integration` | 200, 400, 403, 404, 503 |
| `GET /api/v1/staff/channel-partners/:code` | `GET /api/v1/staff/channel-partners/:code` | [`GetChannelPartnerStatus`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/channel.go#L150) | `get_staff_channel_partners_code` | `StaffBearerAuth` (`revenue_mgr`, `gm_admin`) | `ff_channel_sync_integration` | 200, 400, 403, 404, 503 |

### 3.14 Administrasi Runtime Feature Flags (2 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `GET /api/v1/admin/feature-flags` | `GET /api/v1/admin/feature-flags` | [`AdminListFlags`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/featureflag.go#L19) | `get_admin_feature_flags` | `StaffBearerAuth` (`gm_admin`) | None | 200, 400, 403 |
| `PUT /api/v1/admin/feature-flags/:key` | `PUT /api/v1/admin/feature-flags/:key` | [`AdminUpdateFlag`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/handler/featureflag.go#L35) | `put_admin_feature_flags_key` | `StaffBearerAuth` (`gm_admin`) | None | 200, 400, 403, 404 |

### 3.15 Mock Simulator Pembayaran Sandbox (1 Operasi)
| HTTP Method & Path | Gin Golden Route | Go Handler Symbol | Operation ID | Security | Feature Flag | Responses |
|---|---|---|---|---|---|---|
| `POST /fake-pay/:ref` | `POST /fake-pay/:ref` | [`FakePayHandler`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes.go#L34) | `post_fake_pay_ref` | Public (`[]`) | None | 200, 400, 404 |

---

## 4. Analisis Kepatuhan Keamanan & Standar Arsitektur

### 4.1 Skema Autentikasi & Otorisasi
1. **`StaffBearerAuth`**: Menggunakan header `Authorization: Bearer stf_xxx`.
   * Diperiksa oleh middleware [`RequireStaffAuth`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/auth.go).
   * Dievaluasi terhadap aturan Casbin RBAC ([`config/rbac_model.conf`](file:///mnt/code/projects/jobs/pulang/current-booking/config/rbac_model.conf)) pada [`CasbinAuthorizer`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/rbac.go).
   * Peran yang didukung: `receptionist`, `housekeeping`, `revenue_mgr`, `finance`, `gm_admin`.
2. **`GuestSessionAuth`**: Menggunakan header `Authorization: Bearer gst_sess_xxx` atau HTTP-only cookie `guest_session`.
   * Diperiksa oleh middleware [`RequireGuestAuth`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/middleware/guest_auth.go).
   * Menjamin isolasi data antar tamu (*multi-tenant guest boundary*).
3. **`ChannelWebhookHMAC`**: Menggunakan header `X-Channel-Signature` dan `X-Channel-Provider`.
   * Diverifikasi menggunakan algoritma HMAC-SHA256 *constant-time comparison* (`subtle.ConstantTimeCompare`) untuk mencegah serangan *timing attack*.

### 4.2 Standar Format Error RFC 7807 (Problem Details)
Seluruh respons non-2xx mengembalikan struktur JSON konsisten:
```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "Kuantitas kamar tidak mencukupi",
  "code": "INSUFFICIENT_ROOMS",
  "request_id": "c1f7a049754942cb83218524a87ad183"
}
```

### 4.3 Runtime Kill-Switch (Feature Flags)
Endpoint yang dilindungi middleware `RequireFeature(d.FeatureFlag, key)` mengembalikan HTTP 503 dengan kode error `FEATURE_DISABLED` bila saklar dinonaktifkan di memori/database:
```json
{
  "type": "about:blank",
  "title": "Service Unavailable",
  "status": 503,
  "detail": "fitur sedang dinonaktifkan sementara oleh sistem",
  "code": "FEATURE_DISABLED",
  "request_id": "..."
}
```

---

## 5. Rekomendasi Pemeliharaan Berkelanjutan

1. **Golden Route Assertion di CI/CD**:
   Pertahankan unit test [`TestRoutesGolden`](file:///mnt/code/projects/jobs/pulang/current-booking/internal/api/http/routes_test.go) yang memverifikasi bahwa penambahan rute baru ke router Gin otomatis gagal di CI kecuali `routes.golden` dan file OpenAPI disinkronkan secara bersamaan.
2. **Automated OpenAPI Linting**:
   Tambahkan langkah pada pipa CI (`.github/workflows`):
   ```bash
   npx --yes @redocly/cli lint docs/srs/contracts/pulang-hotel-booking.openapi.json
   ```
3. **Dokumentasi Terintegrasi**:
   Spesifikasi kanonikal ini dapat langsung dirender menggunakan Redoc, Swagger UI, atau Stoplight Elements untuk portal dokumentasi pengembang internal hotel maupun mitra kanal OTA.
