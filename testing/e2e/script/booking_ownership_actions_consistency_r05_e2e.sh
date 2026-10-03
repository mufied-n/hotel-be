#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Booking Ownership & Allowed Actions Consistency (BE-R05)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: OWASP API Top 10 (Broken Object Level Authorization), UU PDP No. 27/2022
# Menguji:
# 1. Canonical Email Normalization pada Checkout (LOWER & TRIM)
# 2. Case-Insensitive Email Matching pada My Bookings (List, Detail, Count, Receipt)
# 3. Dynamic Allowed Actions: Non-Refundable Confirmed (can_cancel = false)
# 4. Dynamic Allowed Actions: Flexible-48h Confirmed Sebelum Cutoff (can_cancel = true)
# 5. Dynamic Allowed Actions: Flexible-48h Confirmed Melewati Cutoff (can_cancel = false)
# 6. Dynamic Allowed Actions: Expired Pending Hold (can_pay = false, can_cancel = false)
# 7. IDOR Protection (404 saat mengakses reservasi milik akun lain)
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONTAINER="${PG_CONTAINER:-r05-pg}"

TOTAL=0
PASSED=0
FAILED=0

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

assert_eq() {
    local desc="$1"
    local expected="$2"
    local actual="$3"
    TOTAL=$((TOTAL + 1))
    if [ "$actual" == "$expected" ]; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Expected: ${expected})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Expected ${expected}, Got ${actual}"
        FAILED=$((FAILED + 1))
    fi
}

assert_contains() {
    local desc="$1"
    local needle="$2"
    local haystack="$3"
    TOTAL=$((TOTAL + 1))
    if echo "$haystack" | grep -qi "$needle"; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Contains: ${needle})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Did not find: ${needle}"
        echo -e "       Content: ${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}  E2E Test: BE-R05 Booking Ownership & Allowed Actions           ${NC}"
echo -e "${BLUE}=================================================================${NC}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Service Healthcheck ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Healthcheck OK" '"status":"ok"' "$HEALTH"

# Helper login tamu
guest_login() {
    local email="$1"
    local code="882233"
    local req_email=$(echo "$email" | tr -d ' ')
    
    curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${req_email}\"}" > /dev/null

    docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -c \
        "UPDATE guest_auth_challenges SET code_hash = encode(sha256('${code}'::bytea), 'hex') WHERE LOWER(TRIM(email)) = LOWER(TRIM('${req_email}'));" > /dev/null

    local resp=$(curl -s -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${req_email}\",\"code\":\"${code}\"}")

    echo "$resp" | jq -r '.token // empty'
}

# ------------------------------------------------------------------------------
# Test 1: Canonical Email Normalization on Checkout
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Canonical Email Normalization on Checkout ---${NC}"
UNIQUE_TAG=$(date +%s%N | tail -c 8)
CHECK_IN=$(date -d "+10 days" +%Y-%m-%d)
CHECK_OUT=$(date -d "+12 days" +%Y-%m-%d)
ROOM_TYPE_ID="01900000-0000-7000-8000-000000000001"

# 1a. Request Quote
QUOTE_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{
        \"room_type_id\": \"${ROOM_TYPE_ID}\",
        \"check_in\": \"${CHECK_IN}\",
        \"check_out\": \"${CHECK_OUT}\",
        \"num_rooms\": 1,
        \"num_guests\": 2
    }")
QUOTE_ID=$(echo "$QUOTE_RESP" | jq -r '.quote_id // empty')
assert_eq "Quote berhasil diterbitkan" "true" "$([ -n "$QUOTE_ID" ] && echo true || echo false)"

# 1b. Checkout dengan email huruf besar dan spasi: '  Guest.Owner_${UNIQUE_TAG}@EXAMPLE.COM  '
RAW_EMAIL="  Guest.Owner_${UNIQUE_TAG}@EXAMPLE.COM  "
CANONICAL_EMAIL="guest.owner_${UNIQUE_TAG}@example.com"

BOOKING_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{
        \"quote_id\": \"${QUOTE_ID}\",
        \"terms_accepted\": true,
        \"privacy_accepted\": true,
        \"room_type_id\": \"${ROOM_TYPE_ID}\",
        \"check_in\": \"${CHECK_IN}\",
        \"check_out\": \"${CHECK_OUT}\",
        \"num_rooms\": 1,
        \"num_guests\": 2,
        \"guest_name\": \"  Budi Santoso  \",
        \"guest_email\": \"${RAW_EMAIL}\",
        \"guest_phone\": \"+6281234567890\"
    }")

BOOKING_ID=$(echo "$BOOKING_RESP" | jq -r '.booking.id // empty')
assert_eq "Booking berhasil dibuat (201 Created)" "true" "$([ -n "$BOOKING_ID" ] && echo true || echo false)"

# 1c. Verifikasi nilai di Database PostgreSQL tersimpan secara kanonikal huruf kecil
DB_STORED_EMAIL=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c \
    "SELECT guest_email FROM bookings WHERE id = '${BOOKING_ID}';")
assert_eq "Email di database tersimpan canonical lowercase" "${CANONICAL_EMAIL}" "${DB_STORED_EMAIL}"

# ------------------------------------------------------------------------------
# Test 2: Case-Insensitive Email Matching for Existing Bookings
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Case-Insensitive Email Matching for Existing Bookings ---${NC}"
LEGACY_EMAIL_RAW="  Legacy.Guest_${UNIQUE_TAG}@HOTEL.COM  "
LEGACY_EMAIL_CLEAN="legacy.guest_${UNIQUE_TAG}@hotel.com"

# Simulasikan data eksisting di DB dengan huruf besar dan spasi
LEGACY_BOOKING_ID=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c "
    INSERT INTO bookings (
        room_type_id, check_in, check_out, num_rooms, num_guests,
        status, total_price_minor, currency, guest_name, guest_email,
        guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
        room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor
    ) VALUES (
        '${ROOM_TYPE_ID}', '${CHECK_IN}', '${CHECK_OUT}', 1, 2,
        'confirmed', 110000000, 'IDR', 'Tamu Legacy', '${LEGACY_EMAIL_RAW}',
        'gst_legacy_tok_123', 'BAR_RO', 'flexible_48h', 'Free cancellation up to 48h',
        110000000, 0, 0, 0
    ) RETURNING id;
" | head -n 1 | tr -d '\r\n')

# Login menggunakan format huruf kecil
LEGACY_TOKEN=$(guest_login "${LEGACY_EMAIL_CLEAN}")
assert_eq "Tamu legacy berhasil login OTP" "true" "$([ -n "$LEGACY_TOKEN" ] && echo true || echo false)"

# 2a. Periksa active_bookings_count di /auth/guest/me
ME_RESP=$(curl -s "${BASE_URL}/api/v1/auth/guest/me" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")
ME_COUNT=$(echo "$ME_RESP" | jq -r '.active_bookings_count')
assert_eq "active_bookings_count mengenali booking legacy mixed-case" "1" "${ME_COUNT}"

# 2b. Periksa daftar pesanan di /guest/bookings
LIST_RESP=$(curl -s "${BASE_URL}/api/v1/guest/bookings" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")
LIST_FOUND=$(echo "$LIST_RESP" | jq -r --arg id "$LEGACY_BOOKING_ID" '.data[] | select(.id == $id) | .id')
assert_eq "List bookings menemukan booking dengan email mixed-case" "${LEGACY_BOOKING_ID}" "${LIST_FOUND}"

# 2c. Periksa detail di /guest/bookings/:id
DETAIL_RESP=$(curl -s "${BASE_URL}/api/v1/guest/bookings/${LEGACY_BOOKING_ID}" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")
DETAIL_ID=$(echo "$DETAIL_RESP" | jq -r '.booking.id // empty')
assert_eq "Get booking detail berhasil untuk email mixed-case" "${LEGACY_BOOKING_ID}" "${DETAIL_ID}"

# ------------------------------------------------------------------------------
# Test 3: Allowed Actions — Non-Refundable Confirmed Booking
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Allowed Actions: Non-Refundable Confirmed Booking ---${NC}"
NON_REF_ID=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c "
    INSERT INTO bookings (
        room_type_id, check_in, check_out, num_rooms, num_guests,
        status, total_price_minor, currency, guest_name, guest_email,
        guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
        room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor
    ) VALUES (
        '${ROOM_TYPE_ID}', '${CHECK_IN}', '${CHECK_OUT}', 1, 2,
        'confirmed', 110000000, 'IDR', 'Tamu NonRef', '${LEGACY_EMAIL_CLEAN}',
        'gst_nonref_tok_123', 'PROMO_NONREF', 'non_refundable', 'Non-refundable rate',
        110000000, 0, 0, 0
    ) RETURNING id;
" | head -n 1 | tr -d '\r\n')

NON_REF_RESP=$(curl -s "${BASE_URL}/api/v1/guest/bookings/${NON_REF_ID}" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")

CAN_CANCEL=$(echo "$NON_REF_RESP" | jq -r '.allowed_actions.can_cancel')
CAN_RECEIPT=$(echo "$NON_REF_RESP" | jq -r '.allowed_actions.can_download_receipt')
CAN_PAY=$(echo "$NON_REF_RESP" | jq -r '.allowed_actions.can_pay')
CAN_ASSIST=$(echo "$NON_REF_RESP" | jq -r '.allowed_actions.can_request_assistance')

assert_eq "Non-refundable confirmed: can_cancel adalah false" "false" "$CAN_CANCEL"
assert_eq "Non-refundable confirmed: can_download_receipt adalah true" "true" "$CAN_RECEIPT"
assert_eq "Non-refundable confirmed: can_pay adalah false" "false" "$CAN_PAY"
assert_eq "Non-refundable confirmed: can_request_assistance adalah true" "true" "$CAN_ASSIST"

# ------------------------------------------------------------------------------
# Test 4: Allowed Actions — Flexible-48h Confirmed Sebelum Cutoff
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Allowed Actions: Flexible-48h Confirmed Sebelum Cutoff ---${NC}"
FLEX_FAR_IN=$(date -d "+7 days" +%Y-%m-%d)
FLEX_FAR_OUT=$(date -d "+9 days" +%Y-%m-%d)

FLEX_VALID_ID=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c "
    INSERT INTO bookings (
        room_type_id, check_in, check_out, num_rooms, num_guests,
        status, total_price_minor, currency, guest_name, guest_email,
        guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
        room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor
    ) VALUES (
        '${ROOM_TYPE_ID}', '${FLEX_FAR_IN}', '${FLEX_FAR_OUT}', 1, 2,
        'confirmed', 110000000, 'IDR', 'Tamu Flex Valid', '${LEGACY_EMAIL_CLEAN}',
        'gst_flex_tok_123', 'BAR_RO', 'flexible_48h', 'Free cancellation up to 48h',
        110000000, 0, 0, 0
    ) RETURNING id;
" | head -n 1 | tr -d '\r\n')

FLEX_VALID_RESP=$(curl -s "${BASE_URL}/api/v1/guest/bookings/${FLEX_VALID_ID}" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")

FLEX_CAN_CANCEL=$(echo "$FLEX_VALID_RESP" | jq -r '.allowed_actions.can_cancel')
FLEX_CAN_RECEIPT=$(echo "$FLEX_VALID_RESP" | jq -r '.allowed_actions.can_download_receipt')
assert_eq "Flexible-48h sebelum deadline: can_cancel adalah true" "true" "$FLEX_CAN_CANCEL"
assert_eq "Flexible-48h sebelum deadline: can_download_receipt adalah true" "true" "$FLEX_CAN_RECEIPT"

# ------------------------------------------------------------------------------
# Test 5: Allowed Actions — Flexible-48h Confirmed Melewati Cutoff (H-1)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Allowed Actions: Flexible-48h Confirmed Melewati Cutoff ---${NC}"
FLEX_PAST_IN=$(date -d "+1 day" +%Y-%m-%d)
FLEX_PAST_OUT=$(date -d "+3 days" +%Y-%m-%d)

FLEX_PAST_ID=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c "
    INSERT INTO bookings (
        room_type_id, check_in, check_out, num_rooms, num_guests,
        status, total_price_minor, currency, guest_name, guest_email,
        guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
        room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor
    ) VALUES (
        '${ROOM_TYPE_ID}', '${FLEX_PAST_IN}', '${FLEX_PAST_OUT}', 1, 2,
        'confirmed', 110000000, 'IDR', 'Tamu Flex Past', '${LEGACY_EMAIL_CLEAN}',
        'gst_flex_past_tok_123', 'BAR_RO', 'flexible_48h', 'Free cancellation up to 48h',
        110000000, 0, 0, 0
    ) RETURNING id;
" | head -n 1 | tr -d '\r\n')

FLEX_PAST_RESP=$(curl -s "${BASE_URL}/api/v1/guest/bookings/${FLEX_PAST_ID}" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")

FLEX_PAST_CAN_CANCEL=$(echo "$FLEX_PAST_RESP" | jq -r '.allowed_actions.can_cancel')
assert_eq "Flexible-48h setelah deadline: can_cancel adalah false" "false" "$FLEX_PAST_CAN_CANCEL"

# ------------------------------------------------------------------------------
# Test 6: Allowed Actions — Expired Pending Hold
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 6. Allowed Actions: Expired Pending Hold ---${NC}"
EXPIRED_HOLD_ID=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c "
    INSERT INTO bookings (
        room_type_id, check_in, check_out, num_rooms, num_guests,
        status, total_price_minor, currency, guest_name, guest_email,
        guest_token, rate_plan_code, cancellation_policy, cancellation_desc,
        room_subtotal_minor, breakfast_charge_minor, discount_minor, tax_minor,
        expires_at
    ) VALUES (
        '${ROOM_TYPE_ID}', '${CHECK_IN}', '${CHECK_OUT}', 1, 2,
        'pending', 110000000, 'IDR', 'Tamu Hold Expired', '${LEGACY_EMAIL_CLEAN}',
        'gst_hold_exp_tok_123', 'BAR_RO', 'flexible_48h', 'Free cancellation up to 48h',
        110000000, 0, 0, 0,
        NOW() - INTERVAL '15 minutes'
    ) RETURNING id;
" | head -n 1 | tr -d '\r\n')

EXPIRED_RESP=$(curl -s "${BASE_URL}/api/v1/guest/bookings/${EXPIRED_HOLD_ID}" \
    -H "Authorization: Bearer ${LEGACY_TOKEN}")

EXP_CAN_PAY=$(echo "$EXPIRED_RESP" | jq -r '.allowed_actions.can_pay')
EXP_CAN_CANCEL=$(echo "$EXPIRED_RESP" | jq -r '.allowed_actions.can_cancel')
EXP_CAN_ASSIST=$(echo "$EXPIRED_RESP" | jq -r '.allowed_actions.can_request_assistance')
assert_eq "Pending hold expired: can_pay adalah false" "false" "$EXP_CAN_PAY"
assert_eq "Pending hold expired: can_cancel adalah false" "false" "$EXP_CAN_CANCEL"
assert_eq "Pending hold expired: can_request_assistance adalah true" "true" "$EXP_CAN_ASSIST"

# ------------------------------------------------------------------------------
# Test 7: IDOR Protection Verification
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 7. IDOR Protection Verification ---${NC}"
ATTACKER_EMAIL="attacker_${UNIQUE_TAG}@example.com"
ATTACKER_TOKEN=$(guest_login "${ATTACKER_EMAIL}")

IDOR_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/guest/bookings/${NON_REF_ID}" \
    -H "Authorization: Bearer ${ATTACKER_TOKEN}")
assert_eq "IDOR attempt returns 404 Not Found" "404" "$IDOR_STATUS"

# ------------------------------------------------------------------------------
# Ringkasan Eksekusi
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}                       RINGKASAN HASIL E2E                       ${NC}"
echo -e "${BLUE}=================================================================${NC}"
echo -e "Total Assertion : $TOTAL"
echo -e "Passed          : ${GREEN}$PASSED${NC}"
echo -e "Failed          : ${RED}$FAILED${NC}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}Semua pengujian Booking Ownership & Actions Consistency (BE-R05) SUKSES!${NC}\n"
    exit 0
else
    echo -e "\n${RED}Terdapat kegagalan pada pengujian BE-R05!${NC}\n"
    exit 1
fi
