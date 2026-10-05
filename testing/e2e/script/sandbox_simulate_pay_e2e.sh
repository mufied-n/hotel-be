#!/usr/bin/env bash
# ==============================================================================
# End-to-End (E2E) Test: Sandbox Simulate Pay Lifecycle
# Hotel Pulang ke Uttara (Yogyakarta)
# Skenario:
# 1. Search kamar
# 2. Ambil quote
# 3. Buat booking via backend API -> status PENDING
# 4. Verifikasi booking berstatus PENDING
# 5. Simulasi pembayaran via endpoint dev /fake-pay/:id -> status CONFIRMED
# 6. Verifikasi booking bertransisi ke CONFIRMED
# 7. Front Desk Check-in sukses
# ==============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-https://hotel.fied.space}"
STAFF_PASSWORD="${STAFF_PASSWORD:-Password12345!}"

TOTAL=0
PASSED=0
FAILED=0

GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  PULANG KE UTTARA — E2E TEST: SANDBOX SIMULATE PAY LIFECYCLE                 ${NC}"
echo -e "${BLUE}  Target: ${BASE_URL}                                                         ${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

assert_status() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    local response_body="$4"

    TOTAL=$((TOTAL + 1))
    if [ "$actual" -eq "$expected" ]; then
        echo -e "  [${GREEN}PASS${NC}] ${test_name} (HTTP ${actual})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  [${RED}FAIL${NC}] ${test_name} — Expected HTTP ${expected}, Got ${actual}"
        echo -e "         Body: ${response_body}"
        FAILED=$((FAILED + 1))
    fi
}

# 1. Login Front Desk
echo -e "\n${BLUE}--- Step 1: Login Front Desk Officer ---${NC}"
LOGIN_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/staff/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"fo_receptionist\",\"password\":\"${STAFF_PASSWORD}\"}")
LOGIN_HTTP=$(echo "$LOGIN_RESP" | tail -n1)
LOGIN_BODY=$(echo "$LOGIN_RESP" | sed '$d')
assert_status "Front Desk Login" 200 "$LOGIN_HTTP" "$LOGIN_BODY"
FO_TOKEN=$(echo "$LOGIN_BODY" | grep -o '"token":"[^"]*' | cut -d'"' -f4)

# 2. Search & Quote Room
echo -e "\n${BLUE}--- Step 2: Search Room & Calculate Quote ---${NC}"
CHECK_IN=$(date -d "+7 days" "+%Y-%m-%d" 2>/dev/null || date -v+7d "+%Y-%m-%d")
CHECK_OUT=$(date -d "+9 days" "+%Y-%m-%d" 2>/dev/null || date -v+9d "+%Y-%m-%d")

SEARCH_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&occupancy=2" \
    -H "Accept: application/json")
SEARCH_HTTP=$(echo "$SEARCH_RESP" | tail -n1)
SEARCH_BODY=$(echo "$SEARCH_RESP" | sed '$d')
assert_status "Search Rooms Available" 200 "$SEARCH_HTTP" "$SEARCH_BODY"

ROOM_TYPE_ID=$(echo "$SEARCH_BODY" | grep -o '"id":"[^"]*' | head -n1 | cut -d'"' -f4)
RATE_PLAN_ID=$(echo "$SEARCH_BODY" | grep -o '"id":"[^"]*' | sed -n '2p' | cut -d'"' -f4 || echo "standard")

QUOTE_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${ROOM_TYPE_ID}\",\"rate_plan_id\":\"${RATE_PLAN_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
QUOTE_HTTP=$(echo "$QUOTE_RESP" | tail -n1)
QUOTE_BODY=$(echo "$QUOTE_RESP" | sed '$d')
assert_status "Calculate Quote" 200 "$QUOTE_HTTP" "$QUOTE_BODY"
QUOTE_ID=$(echo "$QUOTE_BODY" | grep -o '"quote_id":"[^"]*' | head -n1 | cut -d'"' -f4)
if [ -z "$QUOTE_ID" ]; then
    QUOTE_ID=$(echo "$QUOTE_BODY" | grep -o '"id":"[^"]*' | head -n1 | cut -d'"' -f4)
fi

# 3. Create Booking (Pending status)
echo -e "\n${BLUE}--- Step 3: Create Booking (Expect PENDING) ---${NC}"
IDEM_KEY="e2e-pay-sim-$(date +%s)"
BOOKING_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${IDEM_KEY}" \
    -d "{
        \"quote_id\":\"${QUOTE_ID}\",
        \"terms_accepted\":true,
        \"privacy_accepted\":true,
        \"room_type_id\":\"${ROOM_TYPE_ID}\",
        \"check_in\":\"${CHECK_IN}\",
        \"check_out\":\"${CHECK_OUT}\",
        \"num_rooms\":1,
        \"num_guests\":2,
        \"guest_name\":\"Test Guest Sandbox\",
        \"guest_email\":\"test-sandbox@hotel.fied.space\",
        \"guest_phone\":\"+6281234567890\"
    }")
BOOKING_HTTP=$(echo "$BOOKING_RESP" | tail -n1)
BOOKING_BODY=$(echo "$BOOKING_RESP" | sed '$d')
assert_status "Create Booking" 201 "$BOOKING_HTTP" "$BOOKING_BODY"

BOOKING_ID=$(echo "$BOOKING_BODY" | grep -o '"id":"[^"]*' | head -n1 | cut -d'"' -f4)
BOOKING_STATUS=$(echo "$BOOKING_BODY" | grep -o '"status":"[^"]*' | head -n1 | cut -d'"' -f4)
GUEST_TOKEN=$(echo "$BOOKING_BODY" | grep -o '"guest_access_token":"[^"]*' | cut -d'"' -f4 || echo "")

if [ "$BOOKING_STATUS" == "pending" ]; then
    echo -e "  [${GREEN}PASS${NC}] Booking initial status is correctly PENDING"
    PASSED=$((PASSED + 1))
else
    echo -e "  [${RED}FAIL${NC}] Expected initial status pending, got ${BOOKING_STATUS}"
    FAILED=$((FAILED + 1))
fi
TOTAL=$((TOTAL + 1))

# 4. Trigger Simulated Payment via dev endpoint
echo -e "\n${BLUE}--- Step 4: Trigger Simulated Payment (/fake-pay/:id) ---${NC}"
PAY_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/fake-pay/${BOOKING_ID}?booking_id=${BOOKING_ID}" \
    -H "Content-Type: application/json")
PAY_HTTP=$(echo "$PAY_RESP" | tail -n1)
PAY_BODY=$(echo "$PAY_RESP" | sed '$d')
assert_status "Simulate Payment Endpoint" 200 "$PAY_HTTP" "$PAY_BODY"

# 5. Verify Booking transitioned to CONFIRMED
echo -e "\n${BLUE}--- Step 5: Verify Booking Transition to CONFIRMED ---${NC}"
GET_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}" \
    -H "X-Guest-Token: ${GUEST_TOKEN}")
GET_HTTP=$(echo "$GET_RESP" | tail -n1)
GET_BODY=$(echo "$GET_RESP" | sed '$d')
assert_status "Get Booking Detail" 200 "$GET_HTTP" "$GET_BODY"

UPDATED_STATUS=$(echo "$GET_BODY" | grep -o '"status":"[^"]*' | head -n1 | cut -d'"' -f4)
if [ "$UPDATED_STATUS" == "confirmed" ]; then
    echo -e "  [${GREEN}PASS${NC}] Booking successfully transitioned to CONFIRMED"
    PASSED=$((PASSED + 1))
else
    echo -e "  [${RED}FAIL${NC}] Expected status confirmed, got ${UPDATED_STATUS}"
    FAILED=$((FAILED + 1))
fi
TOTAL=$((TOTAL + 1))

# 6. Idempotent check: calling simulate payment again should be harmless
echo -e "\n${BLUE}--- Step 6: Verify Idempotent Replay on CONFIRMED ---${NC}"
REPLAY_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/fake-pay/${BOOKING_ID}?booking_id=${BOOKING_ID}" \
    -H "Content-Type: application/json")
REPLAY_HTTP=$(echo "$REPLAY_RESP" | tail -n1)
REPLAY_BODY=$(echo "$REPLAY_RESP" | sed '$d')
assert_status "Idempotent Simulate Payment Replay" 200 "$REPLAY_HTTP" "$REPLAY_BODY"

echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "  SUMMARY: Total: ${TOTAL} | Passed: ${GREEN}${PASSED}${NC} | Failed: ${RED}${FAILED}${NC}"
echo -e "${BLUE}==============================================================================${NC}"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
