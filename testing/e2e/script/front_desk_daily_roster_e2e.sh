#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Front Desk Daily Operations Roster & Shift Handover Board (Proposed 02)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Real-Time Operational Roster (95 rooms), Shift Handover Logbook,
# Dual-Layer RBAC Authorization (Casbin fail-closed), and Multi-Role Access.
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:8080}"

echo "=== [E2E] Front Desk Daily Operations Roster & Shift Handover Board Testing ==="
echo "Target Base URL: ${BASE_URL}"

# 1. Healthcheck
echo "--- 1. Healthcheck ---"
curl -sS -f "${BASE_URL}/healthz" > /dev/null
echo "✓ Service Healthcheck OK"

# 2. Public Guest Access Denial to Daily Roster (403 Forbidden)
echo "--- 2. Negative Test: Guest Access Denial to Roster (403 Forbidden) ---"
GUEST_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/daily-roster")
if [ "${GUEST_STATUS}" -eq 403 ]; then
  echo "✓ Guest ditolak akses Daily Roster (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${GUEST_STATUS}"
  exit 1
fi

# 3. Receptionist Query Daily Roster (200 OK)
echo "--- 3. Receptionist Query Daily Operations Roster (GET /api/v1/front-desk/daily-roster) ---"
ROSTER_RESP=$(curl -sS -X GET "${BASE_URL}/api/v1/front-desk/daily-roster" \
  -H "Authorization: Bearer receptionist")
echo "Response: ${ROSTER_RESP}"
if echo "${ROSTER_RESP}" | grep -q '"total_rooms":95'; then
  echo "✓ Receptionist Daily Roster Query Sukses (200 OK, total 95 physical rooms)"
else
  echo "✗ Receptionist Daily Roster Response tidak memuat total_rooms 95"
  exit 1
fi

# 4. Cross-Department Visibility (Housekeeping & Revenue Manager)
echo "--- 4. Cross-Department Daily Roster Access (Housekeeping & Revenue Mgr) ---"
HK_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/daily-roster" \
  -H "Authorization: Bearer housekeeping")
REV_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/daily-roster" \
  -H "Authorization: Bearer revenue_mgr")

if [ "${HK_STATUS}" -eq 200 ] && [ "${REV_STATUS}" -eq 200 ]; then
  echo "✓ Housekeeping (200 OK) dan Revenue Manager (200 OK) sukses mengakses Daily Roster"
else
  echo "✗ Gagal otorisasi cross-department: HK=${HK_STATUS}, Rev=${REV_STATUS}"
  exit 1
fi

# 5. Future Date Forecast Query
echo "--- 5. Future Date Forecast Query (GET ?date=2026-10-10) ---"
FORECAST_RESP=$(curl -sS -X GET "${BASE_URL}/api/v1/front-desk/daily-roster?date=2026-10-10" \
  -H "Authorization: Bearer receptionist")
if echo "${FORECAST_RESP}" | grep -q '"date":"2026-10-10"'; then
  echo "✓ Forecast Daily Roster Sukses untuk tanggal 2026-10-10 (200 OK)"
else
  echo "✗ Forecast Daily Roster Gagal memuat tanggal yang diminta"
  exit 1
fi

# 6. Record Shift Handover Note (Role: receptionist)
echo "--- 6. Receptionist Record Shift Handover Note (POST /api/v1/front-desk/handover-notes) ---"
HANDOVER_PAYLOAD='{
  "shift": "morning",
  "cash_float_minor": 1500000,
  "pending_issues": "Keycard encoder workstation 2 error; IT ticket #102 created",
  "vip_guest_notes": "VIP Mr. Tan arrives 14:00, prepare welcome amenities"
}'
HANDOVER_RESP=$(curl -sS -X POST "${BASE_URL}/api/v1/front-desk/handover-notes" \
  -H "Authorization: Bearer receptionist" \
  -H "Content-Type: application/json" \
  -d "${HANDOVER_PAYLOAD}")
echo "Response: ${HANDOVER_RESP}"
if echo "${HANDOVER_RESP}" | grep -q '"shift":"morning"'; then
  echo "✓ Pencatatan Handover Note Sukses (201 Created)"
else
  echo "✗ Gagal mencatat Handover Note"
  exit 1
fi

# 7. Negative Test: Guest Rejection on Handover Note
echo "--- 7. Negative Test: Guest Rejection on Handover Note (403 Forbidden) ---"
GUEST_NOTE_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/front-desk/handover-notes" \
  -H "Content-Type: application/json" \
  -d "${HANDOVER_PAYLOAD}")
if [ "${GUEST_NOTE_STATUS}" -eq 403 ]; then
  echo "✓ Guest ditolak mencatat Handover Note (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${GUEST_NOTE_STATUS}"
  exit 1
fi

# 8. Negative Test: Invalid Shift Rejection
echo "--- 8. Negative Test: Invalid Shift Rejection (400 Bad Request) ---"
INVALID_SHIFT_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/front-desk/handover-notes" \
  -H "Authorization: Bearer receptionist" \
  -H "Content-Type: application/json" \
  -d '{"shift":"dawn","cash_float_minor":1000000,"pending_issues":"None"}')
if [ "${INVALID_SHIFT_STATUS}" -eq 400 ]; then
  echo "✓ Shift tidak valid ditolak dengan 400 Bad Request"
else
  echo "✗ Expected 400 Bad Request, got ${INVALID_SHIFT_STATUS}"
  exit 1
fi

# 9. List Shift Handover History & Pagination
echo "--- 9. Receptionist List Handover Notes History (GET /api/v1/front-desk/handover-notes) ---"
LIST_RESP=$(curl -sS -X GET "${BASE_URL}/api/v1/front-desk/handover-notes?limit=10&offset=0" \
  -H "Authorization: Bearer receptionist")
echo "Response: ${LIST_RESP}"
if echo "${LIST_RESP}" | grep -q '"notes":\['; then
  echo "✓ Riwayat Handover Notes Sukses Dimuat (200 OK)"
else
  echo "✗ Gagal memuat daftar riwayat handover notes"
  exit 1
fi

# 10. Negative Test: Housekeeping Forbidden from Viewing Front Desk Handover Notes
echo "--- 10. Negative Test: Housekeeping Forbidden from Handover Notes (403 Forbidden) ---"
HK_NOTES_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/handover-notes" \
  -H "Authorization: Bearer housekeeping")
if [ "${HK_NOTES_STATUS}" -eq 403 ]; then
  echo "✓ Housekeeping dilarang membaca Handover Notes internal Meja Depan (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${HK_NOTES_STATUS}"
  exit 1
fi

echo "=== Selesai: Seluruh pengujian skrip Front Desk Daily Operations Roster lulus! ==="
