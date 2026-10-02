#!/usr/bin/env bash
# ==============================================================================
# End-to-End (E2E) Test Suite: Hotel Booking Engine & Casbin RBAC
# Properti: Hotel Pulang ke Uttara (Yogyakarta)
# Skenario: Uji Alur Tamu Publik, Proteksi RBAC Staf, dan Regresi Booking
# ==============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
PASSED=0
FAILED=0
TOTAL=0

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}=================================================================${NC}"
echo -e "${BLUE}   Pulang ke Uttara — E2E Test Suite (RBAC & Booking Engine)     ${NC}"
echo -e "${BLUE}   Target: ${BASE_URL}                                           ${NC}"
echo -e "${BLUE}=================================================================${NC}\n"

assert_status() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    local response_body="$4"

    TOTAL=$((TOTAL + 1))
    if [ "$actual" -eq "$expected" ]; then
        echo -e "[${GREEN}PASS${NC}] ${test_name} (HTTP ${actual})"
        PASSED=$((PASSED + 1))
    else
        echo -e "[${RED}FAIL${NC}] ${test_name} — Expected HTTP ${expected}, Got ${actual}"
        echo -e "       Body: ${response_body}"
        FAILED=$((FAILED + 1))
    fi
}

# ------------------------------------------------------------------------------
# Test 1: Healthz & Readiness Check
# ------------------------------------------------------------------------------
echo -e "${YELLOW}>> 1. System Health & Probes <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" "${BASE_URL}/healthz")
assert_status "GET /healthz returns 200 OK" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 2: Public Availability Search
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 2. Public Availability Search (Role: Guest / Anonymous) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/availability?room_type_id=01900000-0000-7000-8000-000000000001&check_in=2026-10-10&check_out=2026-10-12")
assert_status "Public Guest can search availability" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 3: Public Create Booking (Hold)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 3. Public Create Booking Hold (Role: Guest) <<${NC}"
BOOKING_PAYLOAD='{
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "num_rooms": 1,
  "num_guests": 2,
  "guest_name": "Budi Santoso",
  "guest_email": "budi.santoso@example.com"
}'

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Content-Type: application/json" \
    -d "$BOOKING_PAYLOAD" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Guest creates booking hold" 201 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# Extract booking_id if created
BOOKING_ID=$(grep -o '"id":"[^"]*' /tmp/e2e_res.json | cut -d'"' -f4 || echo "")
if [ -z "$BOOKING_ID" ]; then
    BOOKING_ID="01900000-0000-7000-8000-000000000001"
fi
echo -e "       Active Booking ID: ${BOOKING_ID}"

# ------------------------------------------------------------------------------
# Test 4: RBAC Negative Tests (Guest cannot Check-In)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 4. RBAC Protection: Guest / Public Forbidden Actions <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Anonymous user is FORBIDDEN from check-in" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "X-User-Role: guest" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Explicit guest role is FORBIDDEN from check-in" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "X-User-Role: housekeeping" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Housekeeping role is FORBIDDEN from check-in" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 5: Confirm Payment (Simulation)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 5. Payment Confirmation <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    "${BASE_URL}/fake-pay/ref-e2e?booking_id=${BOOKING_ID}")
assert_status "Confirm payment via webhook/fake-pay" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 6: RBAC Authorized Front Desk Actions (Receptionist Role)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 6. RBAC Staff Actions (Role: Receptionist) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "X-User-Role: receptionist" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Receptionist checks in guest" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer receptionist" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-out")
assert_status "Receptionist checks out guest (Bearer token)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 7: Super Admin Access (Role: gm_admin)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 7. RBAC Super Admin Wildcard Access (Role: gm_admin) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "X-User-Role: gm_admin" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
assert_status "General Manager can inspect any booking" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Summary
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "   E2E Results: ${TOTAL} Total | ${GREEN}${PASSED} Passed${NC} | ${RED}${FAILED} Failed${NC}"
echo -e "${BLUE}=================================================================${NC}"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
exit 0
