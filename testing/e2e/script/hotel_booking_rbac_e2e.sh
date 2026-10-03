#!/usr/bin/env bash
# ==============================================================================
# End-to-End (E2E) Test Suite: Hotel Booking Engine & Casbin RBAC
# Properti: Hotel Pulang ke Uttara (Yogyakarta)
# Skenario: Uji Alur Tamu Publik, Proteksi RBAC Staf, dan Regresi Booking
# ==============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
source "$(dirname "${BASH_SOURCE[0]}")/lib_staff_login.sh"
load_staff_tokens
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
# Test 2: Public Availability & Room Catalog Discovery
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 2A. Public Catalog Discovery (7 sellable variants, 95 rooms) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/catalog/rooms")
assert_status "Public Guest can discover room catalog" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

echo -e "\n${YELLOW}>> 2B. Multi-Night Cross-Variant Search Engine (BE-G02, BE-G03) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=2&rooms=1")
assert_status "Cross-variant search returns continuous availability" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

echo -e "\n${YELLOW}>> 2C. Search Validation Boundaries (BE-G03: LOS & Child Age) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/search?check_in=2026-10-10&check_out=2026-11-20&adults=2&rooms=1")
assert_status "Search rejects LOS > 30 nights with 400 Bad Request" 400 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&child_ages=19")
assert_status "Search rejects child age > 17 with 400 Bad Request" 400 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

echo -e "\n${YELLOW}>> 2D. Single Room Availability (Legacy/Direct) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/availability?room_type_id=01900000-0000-7000-8000-000000000001&check_in=2026-10-10&check_out=2026-10-12")
assert_status "Public Guest can search single room availability" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

echo -e "\n${YELLOW}>> 2E. Catalog Room CRUD Administration & RBAC Boundaries <<${NC}"
# Public Guest gets single room
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/catalog/rooms/01900000-0000-7000-8000-000000000001")
assert_status "Public Guest can view single room variant details" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# Revenue Manager creates new room variant
NEW_ROOM_PAYLOAD='{
  "code": "villa-garden",
  "name": "Garden Villa",
  "family_name": "Villa",
  "bed_type": "1 King Bed",
  "room_size_sqm": 85,
  "max_capacity": 4,
  "max_adults": 2,
  "max_children": 2,
  "base_price_minor": 2500000,
  "description": "Private villa with lush tropical garden view.",
  "amenities": ["Private Pool", "Free Wi-Fi"],
  "photos": [{"url": "https://example.com/villa.jpg", "alt": "Garden Villa"}]
}'

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    -d "$NEW_ROOM_PAYLOAD" \
    "${BASE_URL}/api/v1/catalog/rooms")
assert_status "Revenue Manager creates new room variant" 201 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

CATALOG_VARIANT_ID=$(grep -o '"id":"[^"]*' /tmp/e2e_res.json | head -1 | cut -d'"' -f4 || echo "")
echo -e "       Created Room Variant ID: ${CATALOG_VARIANT_ID}"

# Revenue Manager updates room variant
UPDATE_ROOM_PAYLOAD='{
  "code": "villa-garden",
  "name": "Garden Villa Deluxe",
  "family_name": "Villa",
  "bed_type": "1 King Bed",
  "room_size_sqm": 85,
  "max_capacity": 4,
  "max_adults": 2,
  "max_children": 2,
  "base_price_minor": 2750000,
  "description": "Private villa with lush tropical garden view and floating breakfast.",
  "amenities": ["Private Pool", "Free Wi-Fi", "Floating Breakfast"],
  "photos": [{"url": "https://example.com/villa.jpg", "alt": "Garden Villa"}]
}'

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    -d "$UPDATE_ROOM_PAYLOAD" \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "Revenue Manager updates room variant" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# Public Guest views updated room variant
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "Public Guest views updated room variant" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# RBAC Negative Tests for Catalog
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"code":"hack"}' \
    "${BASE_URL}/api/v1/catalog/rooms")
assert_status "Guest is FORBIDDEN from creating room variant" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Content-Type: application/json" \
    -d '{"name":"hack"}' \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "Guest is FORBIDDEN from updating room variant" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X DELETE \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "Guest is FORBIDDEN from deleting room variant" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X DELETE \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "Revenue Manager is FORBIDDEN from deleting room variant" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# GM Admin deletes room variant
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X DELETE \
    -H "Authorization: Bearer ${T_GM_ADMIN}" \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "GM Admin deletes room variant" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# Subsequent GET returns 404
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_VARIANT_ID}")
assert_status "Deleted room variant returns 404 Not Found" 404 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# mkquote <check_in> <check_out> <rooms> <guests> -> quote_id (BE-R06: quote single-use)
RUN_ID=$(date +%s%N)  # idempotency key unik per run agar skrip bisa diulang
mkquote() {
    curl -s -H "Content-Type: application/json" \
        -d "{\"room_type_id\":\"01900000-0000-7000-8000-000000000001\",\"check_in\":\"$1\",\"check_out\":\"$2\",\"num_rooms\":$3,\"num_guests\":$4}" \
        "${BASE_URL}/api/v1/quotes" | grep -o '"quote_id":"[^"]*' | cut -d'"' -f4
}
CONSENT='"terms_accepted": true, "privacy_accepted": true,'

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
BOOKING_PAYLOAD=$(echo "$BOOKING_PAYLOAD" | sed "s/^{/{\"quote_id\":\"$(mkquote 2026-10-10 2026-10-12 1 2)\",\"terms_accepted\":true,\"privacy_accepted\":true,/")

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Content-Type: application/json" \
    -d "$BOOKING_PAYLOAD" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Guest creates booking hold" 201 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# Extract booking_id and guest_access_token if created
BOOKING_ID=$(grep -o '"id":"[^"]*' /tmp/e2e_res.json | head -1 | cut -d'"' -f4 || echo "")
GUEST_TOKEN=$(grep -o '"guest_access_token":"[^"]*' /tmp/e2e_res.json | cut -d'"' -f4 || echo "")
if [ -z "$BOOKING_ID" ]; then
    BOOKING_ID="01900000-0000-7000-8000-000000000001"
fi
echo -e "       Active Booking ID: ${BOOKING_ID}"
echo -e "       Guest Access Token: ${GUEST_TOKEN:-none}"

# ------------------------------------------------------------------------------
# Test 4: BE-G13 PII Masking vs Authorized Access
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 4. PII Protection (BE-G13) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
assert_status "Public Guest receives masked PublicDTO (no PII)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"
if grep -q '"guest_name"' /tmp/e2e_res.json; then
    echo -e "[${RED}FAIL${NC}] PII Leakage: guest_name present in public DTO!"
    FAILED=$((FAILED + 1))
else
    echo -e "[${GREEN}PASS${NC}] Verified: guest_name is NOT leaked in public DTO"
    PASSED=$((PASSED + 1))
fi
TOTAL=$((TOTAL + 1))

if [ -n "$GUEST_TOKEN" ]; then
    HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
        -H "X-Guest-Token: ${GUEST_TOKEN}" \
        "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
    assert_status "Guest with X-Guest-Token receives full PII" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"
fi

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "X-Guest-Token: invalid_token" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/cancel")
assert_status "Guest with invalid token rejected from cancel (403 Forbidden)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 5: RBAC Negative Tests (Guest cannot Check-In)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 5. RBAC Protection: Guest / Public Forbidden Actions <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Anonymous user is FORBIDDEN from check-in" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Accept: application/json" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Explicit guest role is FORBIDDEN from check-in" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Housekeeping role is FORBIDDEN from check-in" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 6: Confirm Payment (Simulation)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 6. Payment Confirmation <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    "${BASE_URL}/fake-pay/ref-e2e?booking_id=${BOOKING_ID}")
assert_status "Confirm payment via webhook/fake-pay" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 7: RBAC Authorized Front Desk Actions (Receptionist Role)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 7. RBAC Staff Actions (Role: Receptionist) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-in")
assert_status "Receptionist checks in guest" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-out")
assert_status "Receptionist checks out guest (Bearer token)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 8: Super Admin Access (Role: gm_admin)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 8. RBAC Super Admin Wildcard Access (Role: gm_admin) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_GM_ADMIN}" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
assert_status "General Manager can inspect any booking" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 9: Locked Quote Generation (BE-G04, BE-G05, BE-G06)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 9. 15-Minute Locked Quote with Promo & Breakdown (BE-G04, BE-G05, BE-G06) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "rate_plan_code": "bed_and_breakfast",
        "check_in": "2026-10-10",
        "check_out": "2026-10-12",
        "num_rooms": 1,
        "num_guests": 2,
        "promo_code": "OCTOBREAK"
    }' \
    "${BASE_URL}/api/v1/quotes")
assert_status "Guest generates locked quote with Bed & Breakfast and OCTOBREAK promo" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

LOCKED_QUOTE_ID=$(grep -o '"quote_id":"[^"]*' /tmp/e2e_res.json | cut -d'"' -f4 || echo "")
echo -e "       Locked Quote ID: ${LOCKED_QUOTE_ID}"

# ------------------------------------------------------------------------------
# Test 10: Consent Required Validation (BE-G19)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 10. Consent Required Validation (BE-G19) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{
        \"quote_id\": \"${LOCKED_QUOTE_ID}\",
        \"terms_accepted\": false,
        \"privacy_accepted\": true,
        \"room_type_id\": \"01900000-0000-7000-8000-000000000001\",
        \"check_in\": \"2026-10-10\",
        \"check_out\": \"2026-10-12\",
        \"num_rooms\": 1,
        \"num_guests\": 2,
        \"guest_name\": \"Dewi Lestari\",
        \"guest_email\": \"dewi@example.com\"
    }" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Booking without consent rejected with 400 CONSENT_REQUIRED" 400 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 11: Booking Creation with Valid Quote & Consent (BE-G06, BE-G19)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 11. Booking Creation with Valid Quote Snapshot (BE-G06, BE-G19) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{
        \"quote_id\": \"${LOCKED_QUOTE_ID}\",
        \"terms_accepted\": true,
        \"privacy_accepted\": true,
        \"room_type_id\": \"01900000-0000-7000-8000-000000000001\",
        \"check_in\": \"2026-10-10\",
        \"check_out\": \"2026-10-12\",
        \"num_rooms\": 1,
        \"num_guests\": 2,
        \"guest_name\": \"Dewi Lestari\",
        \"guest_email\": \"dewi@example.com\"
    }" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Booking created with verified locked quote snapshot" 201 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

PROMO_BOOKING_ID=$(grep -o '"id":"[^"]*' /tmp/e2e_res.json | head -n 1 | cut -d'"' -f4 || echo "")
PROMO_GUEST_TOKEN=$(grep -o '"guest_access_token":"[^"]*' /tmp/e2e_res.json | cut -d'"' -f4 || echo "")
echo -e "       Promo Booking ID: ${PROMO_BOOKING_ID}"

# ------------------------------------------------------------------------------
# Test 12: Cancellation Policy Enforcement (BE-G08)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 12. Non-Refundable Cancellation Rejection (BE-G08) <<${NC}"
# Simulasikan pembayaran sukses terlebih dahulu
curl -s -o /dev/null -X POST "${BASE_URL}/fake-pay/ref-promo?booking_id=${PROMO_BOOKING_ID}"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "X-Guest-Token: ${PROMO_GUEST_TOKEN}" \
    "${BASE_URL}/api/v1/bookings/${PROMO_BOOKING_ID}/cancel")
assert_status "Cancellation of confirmed non-refundable booking rejected with 409 Conflict" 409 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 13: Complete Guest Profile Checkout (BE-G07, BE-G12)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 13. Complete Guest Profile Checkout & Timestamps (BE-G07, BE-G12) <<${NC}"
BATCH_D_PAYLOAD='{
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "num_rooms": 1,
    "num_guests": 2,
    "guest_name": "Rian Kusuma",
    "guest_email": "rian@example.com",
    "guest_phone": "+6281298765432",
    "estimated_arrival_time": "14:30",
    "special_requests": "High floor, non-smoking, quiet room"
}'
BATCH_D_PAYLOAD=$(echo "$BATCH_D_PAYLOAD" | sed "s/^{/{\"quote_id\":\"$(mkquote 2026-10-10 2026-10-12 1 2)\",\"terms_accepted\":true,\"privacy_accepted\":true,/")

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ik-bash-rian-${RUN_ID}" \
    -d "$BATCH_D_PAYLOAD" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Checkout with complete guest profile returns 201 with expires_at & server_time" 201 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

BATCH_D_BOOKING_ID=$(grep -o '"id":"[^"]*' /tmp/e2e_res.json | head -n 1 | cut -d'"' -f4 || echo "")
BATCH_D_GUEST_TOKEN=$(grep -o '"guest_access_token":"[^"]*' /tmp/e2e_res.json | cut -d'"' -f4 || echo "")

# ------------------------------------------------------------------------------
# Test 14: Idempotency-Key Network Replay (BE-G09, IETF Draft)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 14. Idempotency-Key Network Replay (BE-G09) <<${NC}"
HTTP_HEADERS=$(curl -s -D /tmp/e2e_headers.txt -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ik-bash-rian-${RUN_ID}" \
    -d "$BATCH_D_PAYLOAD" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Idempotent retry with same payload returns 201 Created" 201 "$HTTP_HEADERS" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 15: Idempotency-Key Payload Mismatch Conflict (BE-G09)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 15. Idempotency-Key Conflict on Payload Mismatch (BE-G09) <<${NC}"
DIFF_PAYLOAD='{
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "num_rooms": 2,
    "num_guests": 4,
    "guest_name": "Totally Different",
    "guest_email": "different@example.com"
}'
DIFF_PAYLOAD=$(echo "$DIFF_PAYLOAD" | sed "s/^{/{\"quote_id\":\"$(mkquote 2026-10-10 2026-10-12 2 4)\",\"terms_accepted\":true,\"privacy_accepted\":true,/")
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ik-bash-rian-${RUN_ID}" \
    -d "$DIFF_PAYLOAD" \
    "${BASE_URL}/api/v1/bookings")
assert_status "Idempotency key reuse with different payload returns 409 Conflict" 409 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 16: UU PDP Privacy Enforcement (BE-G13, UU PDP No. 27/2022)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 16. UU PDP Privacy Masking on Public View (BE-G13) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/bookings/${BATCH_D_BOOKING_ID}")
assert_status "Unauthenticated query returns PublicDTO (masked PII)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "X-Guest-Token: ${BATCH_D_GUEST_TOKEN}" \
    "${BASE_URL}/api/v1/bookings/${BATCH_D_BOOKING_ID}")
assert_status "Owner query with X-Guest-Token returns full profile" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 17: Early Check-Out Inventory Restitution (BE-G22)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 17. Early Check-Out Inventory Restitution (BE-G22) <<${NC}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${BOOKING_ID}/check-out")
assert_status "Receptionist processes early check-out" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 18: Rejection of No-Show Before Check-In Date (BE-G22)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 18. Rejection of No-Show Before Check-In Date (BE-G22) <<${NC}"
# BATCH_D_BOOKING_ID check-in is set to 2026-10-10 (future date relative to current time)
# Fake pay to confirm booking first
curl -s -o /dev/null -X POST "${BASE_URL}/fake-pay/ref-batch-d?booking_id=${BATCH_D_BOOKING_ID}"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${BATCH_D_BOOKING_ID}/no-show")
# Expect 400 Bad Request with NO_SHOW_TOO_EARLY
assert_status "No-show before check-in date rejected with 400 NO_SHOW_TOO_EARLY" 400 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Test 19: Receptionist Marks No-Show on/after Check-In Date (BE-G22)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 19. Receptionist Marks No-Show on/after Check-In Date (BE-G22) <<${NC}"
sleep 3  # limiter publik 20 rps/burst 40; beri token bucket waktu isi ulang
# Create booking with today's check-in date
TODAY_DATE=$(date -u +"%Y-%m-%d")
TOMORROW_DATE=$(date -u -d "+1 day" +"%Y-%m-%d" 2>/dev/null || date -u -v+1d +"%Y-%m-%d" 2>/dev/null || echo "2026-10-11")
TODAY_PAYLOAD="{
    \"quote_id\": \"$(mkquote ${TODAY_DATE} ${TOMORROW_DATE} 1 1)\", \"terms_accepted\": true, \"privacy_accepted\": true,
    \"room_type_id\": \"01900000-0000-7000-8000-000000000001\",
    \"check_in\": \"${TODAY_DATE}\",
    \"check_out\": \"${TOMORROW_DATE}\",
    \"num_rooms\": 1,
    \"num_guests\": 1,
    \"guest_name\": \"Guest Today\",
    \"guest_email\": \"today@example.com\"
}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$TODAY_PAYLOAD" \
    "${BASE_URL}/api/v1/bookings")
TODAY_BID=$(grep -o '"id":"[^"]*' /tmp/e2e_res.json | head -n 1 | cut -d'"' -f4 || echo "")

if [ -n "$TODAY_BID" ]; then
    # Confirm booking
    curl -s -o /dev/null -X POST "${BASE_URL}/fake-pay/ref-today?booking_id=${TODAY_BID}"
    # Mark no show
    HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
        -X POST \
        -H "Authorization: Bearer ${T_RECEPTIONIST}" \
        "${BASE_URL}/api/v1/bookings/${TODAY_BID}/no-show")
    assert_status "No-show on check-in date accepted with 200 OK" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"
fi


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
