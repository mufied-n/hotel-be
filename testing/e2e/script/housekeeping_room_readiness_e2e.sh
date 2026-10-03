#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Housekeeping Room Status & Readiness Lifecycle (Proposed 01)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: AHLA Quality Standards, Cloudbeds Dual-Stage Verification,
# Casbin RBAC, dan Front Desk Check-in Readiness Guard.
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:8080}"
source "$(dirname "${BASH_SOURCE[0]}")/lib_staff_login.sh"
load_staff_tokens
source "$(dirname "${BASH_SOURCE[0]}")/lib_fixtures.sh"
# Fixture: 202 & 204 kotor agar siklus dirty->cleaning->clean->inspected bisa diuji
set_room_status 202 vacant_dirty
set_room_status 204 vacant_dirty

echo "=== [E2E] Housekeeping Room Status & Readiness Lifecycle Testing ==="
echo "Target Base URL: ${BASE_URL}"

# 1. Healthcheck
echo "--- 1. Healthcheck ---"
curl -sS -f "${BASE_URL}/healthz" > /dev/null
echo "✓ Service Healthcheck OK"

# 2. Housekeeping Room Board Query (Role: housekeeping)
echo "--- 2. Query Housekeeping Room Board (GET /api/v1/housekeeping/rooms) ---"
BOARD_RESP=$(curl -sS -X GET "${BASE_URL}/api/v1/housekeeping/rooms?floor=2" \
  -H "Authorization: Bearer ${T_HOUSEKEEPING}")
echo "Response: ${BOARD_RESP}"
if echo "${BOARD_RESP}" | grep -q '"total_rooms"'; then
  echo "✓ Housekeeping Room Board Query Sukses (200 OK)"
else
  echo "✗ Housekeeping Room Board Query Gagal"
  exit 1
fi

# 3. Cleanliness Lifecycle Transition: dirty -> cleaning -> clean -> inspected
echo "--- 3. Cleanliness Lifecycle Transitions on Room 202 ---"
# 3a. Start cleaning
curl -sS -f -X PUT "${BASE_URL}/api/v1/housekeeping/rooms/202/status" \
  -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
  -H "Content-Type: application/json" \
  -d '{"to_status":"cleaning","notes":"Attendant started cleaning"}' > /dev/null
echo "✓ Transisi dirty -> cleaning OK"

# 3b. Finish cleaning
curl -sS -f -X PUT "${BASE_URL}/api/v1/housekeeping/rooms/202/status" \
  -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
  -H "Content-Type: application/json" \
  -d '{"to_status":"clean","notes":"Linen changed, amenities stocked"}' > /dev/null
echo "✓ Transisi cleaning -> clean OK"

# 3c. Supervisor inspection
curl -sS -f -X PUT "${BASE_URL}/api/v1/housekeeping/rooms/202/status" \
  -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
  -H "Content-Type: application/json" \
  -d '{"to_status":"inspected","notes":"QC inspection passed"}' > /dev/null
echo "✓ Transisi clean -> inspected OK"

# 4. Anti-Bypass Guard: Attempt direct transition dirty -> inspected
echo "--- 4. Negative Test: Anti-Bypass Direct dirty -> inspected (409 Conflict) ---"
BYPASS_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X PUT "${BASE_URL}/api/v1/housekeeping/rooms/204/status" \
  -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
  -H "Content-Type: application/json" \
  -d '{"to_status":"inspected","notes":"Direct bypass attempt"}')

if [ "${BYPASS_STATUS}" -eq 409 ]; then
  echo "✓ Bypass ditolak secara aman dengan 409 Conflict (INVALID_STATUS_TRANSITION)"
else
  echo "✗ Expected 409 Conflict, got ${BYPASS_STATUS}"
  exit 1
fi

# 5. Out of Order Authorization Check
echo "--- 5. Out-of-Order RBAC Isolation Check ---"
NON_GM_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/housekeeping/rooms/203/out-of-order" \
  -H "Authorization: Bearer ${T_RECEPTIONIST}" \
  -H "Content-Type: application/json" \
  -d '{"start_date":"2026-10-10","end_date":"2026-10-15","reason":"AC breakdown"}')

if [ "${NON_GM_STATUS}" -eq 403 ]; then
  echo "✓ Resepsionis dilarang menetapkan Out of Order (403 Forbidden)"
else
  echo "✗ Expected 403 Forbidden, got ${NON_GM_STATUS}"
  exit 1
fi

echo "=== Selesai: Seluruh pengujian skrip housekeeping room readiness lulus! ==="
