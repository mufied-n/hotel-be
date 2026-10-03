#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Stay Modification: Room Move & Stay Extension (Proposed 03)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Mid-Stay Room Move, Post-Checkin GiST Range Trimming,
# Housekeeping Auto-Dirty Turnover, Dynamic Extension Pricing & Casbin RBAC.
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:8080}"
source "$(dirname "${BASH_SOURCE[0]}")/lib_staff_login.sh"
load_staff_tokens
source "$(dirname "${BASH_SOURCE[0]}")/lib_fixtures.sh"
# Fixture: 204 kotor (target ditolak), 202 siap huni, booking checked-in baru
db_exec "DELETE FROM room_assignments WHERE room_number IN ('202','204')"  # reset sisa run lama (DB *_test)
set_room_status 204 vacant_dirty
set_room_status 202 vacant_dirty   # cegah booking fixture mendapat 202
TEST_BOOKING_ID="${BOOKING_ID:-$(create_checked_in_booking)}"
set_room_status 202 inspected

echo "=== [E2E] Stay Modification: Room Move & Stay Extension Testing ==="
echo "Target Base URL: ${BASE_URL}"
echo "Test Booking ID: ${TEST_BOOKING_ID}"

# 1. Healthcheck
echo "--- 1. Healthcheck ---"
curl -sS -f "${BASE_URL}/healthz" > /dev/null
echo "✓ Service Healthcheck OK"

# 2. Public Guest Access Denial to Room Move (403 Forbidden)
echo "--- 2. Negative Test: Guest Access Denial to Room Move (403 Forbidden) ---"
GUEST_MOVE_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/room-move" \
  -H "Content-Type: application/json" \
  -d '{"target_room_number":"202","reason_category":"maintenance_defect"}')
if [ "${GUEST_MOVE_STATUS}" -eq 403 ]; then
  echo "✓ Guest ditolak eksekusi Room Move (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${GUEST_MOVE_STATUS}"
  exit 1
fi

# 3. Receptionist Execute Mid-Stay Room Move (200 OK)
echo "--- 3. Receptionist Execute Mid-Stay Room Move (POST /api/v1/bookings/{id}/room-move) ---"
MOVE_RESP=$(curl -sS -X POST "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/room-move" \
  -H "Authorization: Bearer ${T_RECEPTIONIST}" \
  -H "Content-Type: application/json" \
  -d '{
    "target_room_number": "202",
    "reason_category": "maintenance_defect",
    "notes": "AC kamar lama kurang dingin; tamu dipindahkan ke kamar 202"
  }')
echo "Response: ${MOVE_RESP}"
if echo "${MOVE_RESP}" | grep -q '"new_room_number":"202"'; then
  echo "✓ Eksekusi Room Move Sukses (200 OK, target room 202)"
else
  echo "✗ Gagal melakukan Room Move"
  exit 1
fi

# 4. Negative Test: Room Move to Dirty Target Room (409 Conflict)
echo "--- 4. Negative Test: Rejection on Dirty Target Room (409 Conflict) ---"
DIRTY_MOVE_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/room-move" \
  -H "Authorization: Bearer ${T_RECEPTIONIST}" \
  -H "Content-Type: application/json" \
  -d '{
    "target_room_number": "204",
    "reason_category": "guest_request",
    "notes": "Tamu ingin lantai 2 nomor genap"
  }')
if [ "${DIRTY_MOVE_STATUS}" -eq 409 ]; then
  echo "✓ Pindah ke kamar kotor ditolak dengan 409 Conflict (TARGET_ROOM_NOT_READY)"
else
  echo "✗ Expected 409 Conflict, got ${DIRTY_MOVE_STATUS}"
  exit 1
fi

# 5. Public Guest Access Denial to Stay Extension (403 Forbidden)
echo "--- 5. Negative Test: Guest Access Denial to Stay Extension (403 Forbidden) ---"
GUEST_EXT_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/extend-stay" \
  -H "Content-Type: application/json" \
  -d '{"additional_nights":2,"payment_method":"front_desk_edc"}')
if [ "${GUEST_EXT_STATUS}" -eq 403 ]; then
  echo "✓ Guest ditolak eksekusi Stay Extension (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${GUEST_EXT_STATUS}"
  exit 1
fi

# 6. Receptionist Execute Stay Extension (200 OK)
echo "--- 6. Receptionist Execute Stay Extension (POST /api/v1/bookings/{id}/extend-stay) ---"
EXT_RESP=$(curl -sS -X POST "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/extend-stay" \
  -H "Authorization: Bearer ${T_RECEPTIONIST}" \
  -H "Content-Type: application/json" \
  -d '{
    "additional_nights": 2,
    "payment_method": "front_desk_edc"
  }')
echo "Response: ${EXT_RESP}"
if echo "${EXT_RESP}" | grep -q '"additional_nights":2'; then
  echo "✓ Eksekusi Stay Extension 2 Malam Sukses (200 OK)"
else
  echo "✗ Gagal memperpanjang masa menginap"
  exit 1
fi

# 7. Negative Test: Rejection on Zero Additional Nights (400 Bad Request)
echo "--- 7. Negative Test: Rejection on Zero Additional Nights (400 Bad Request) ---"
ZERO_NIGHTS_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/extend-stay" \
  -H "Authorization: Bearer ${T_RECEPTIONIST}" \
  -H "Content-Type: application/json" \
  -d '{"additional_nights":0}')
if [ "${ZERO_NIGHTS_STATUS}" -eq 400 ]; then
  echo "✓ Nilai malam 0 ditolak dengan 400 Bad Request (INVALID_ADDITIONAL_NIGHTS)"
else
  echo "✗ Expected 400 Bad Request, got ${ZERO_NIGHTS_STATUS}"
  exit 1
fi

# 8. Room Move Audit Log Inquiry (200 OK)
echo "--- 8. Receptionist Room Move Audit Log Inquiry (GET /api/v1/bookings/{id}/room-moves) ---"
MOVES_RESP=$(curl -sS -X GET "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/room-moves" \
  -H "Authorization: Bearer ${T_RECEPTIONIST}")
echo "Response: ${MOVES_RESP}"
if echo "${MOVES_RESP}" | grep -q '"moves":\['; then
  echo "✓ Riwayat Pemindahan Kamar Sukses Dimuat (200 OK)"
else
  echo "✗ Gagal memuat riwayat room move"
  exit 1
fi

# 9. Negative Test: Guest Denial to Room Move Audit Log (403 Forbidden)
echo "--- 9. Negative Test: Guest Denial to Room Move Audit Log (403 Forbidden) ---"
GUEST_LOG_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${TEST_BOOKING_ID}/room-moves")
if [ "${GUEST_LOG_STATUS}" -eq 403 ]; then
  echo "✓ Tamu publik dilarang membaca log audit internal pemindahan kamar (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${GUEST_LOG_STATUS}"
  exit 1
fi

echo "=== Selesai: Seluruh pengujian skrip Stay Modification & Room Move lulus! ==="
