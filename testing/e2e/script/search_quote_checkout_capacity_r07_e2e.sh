#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Invariant Kapasitas Kamar Search, Quote & Checkout (BE-R07)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Standar Keselamatan Hotel Bintang 4, Paritas Search-Quote-Checkout
# Menguji:
# 1. Validasi Parameter Search (adults < rooms, child_ages count mismatch, invalid age)
# 2. Flagging Okupansi Search (MaxAdults, MaxChildren, MaxCapacity per varian kamar)
# 3. Penjagaan Kapasitas Quote terhadap Katalog (Overcapacity, breakdown, age count)
# 4. Penjagaan Direct Checkout / Bypass Search Overcapacity
# 5. Siklus Lengkap Reservasi Boundary Valid (Search -> Quote -> Booking -> Pay)
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONTAINER="${PG_CONTAINER:-r07-pg}"

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
echo -e "${BLUE}  E2E Test: BE-R07 Invariant Kapasitas Kamar Katalog             ${NC}"
echo -e "${BLUE}=================================================================${NC}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Service Healthcheck ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Healthcheck OK" '"status":"ok"' "$HEALTH"

CHECK_IN=$(date -d "+10 days" +%Y-%m-%d)
CHECK_OUT=$(date -d "+12 days" +%Y-%m-%d)
SUP_KING_ID="01900000-0000-7000-8000-000000000001"

# ------------------------------------------------------------------------------
# Test 1: Validasi Parameter Search (adults < rooms, child_ages mismatch, invalid age)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Validasi Parameter Search (FR-01) ---${NC}"

# 1A. adults < rooms ditolak 400 INVALID_GUEST_COUNT
SEARCH_1A=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=1&rooms=2")
HTTP_1A=$(echo "$SEARCH_1A" | tail -n1)
BODY_1A=$(echo "$SEARCH_1A" | head -n-1)
assert_eq "Search adults < rooms status code" "400" "$HTTP_1A"
assert_contains "Search adults < rooms error code" "INVALID_GUEST_COUNT" "$BODY_1A"

# 1B. children == 0 tetapi child_ages diberikan -> ditolak 400 CHILD_AGE_COUNT_MISMATCH
SEARCH_1B=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=2&children=0&child_ages=5")
HTTP_1B=$(echo "$SEARCH_1B" | tail -n1)
BODY_1B=$(echo "$SEARCH_1B" | head -n-1)
assert_eq "Search children=0 dengan child_ages status code" "400" "$HTTP_1B"
assert_contains "Search children=0 dengan child_ages error code" "CHILD_AGE_COUNT_MISMATCH" "$BODY_1B"

# 1C. children == 2 tetapi child_ages hanya 1 -> ditolak 400 CHILD_AGE_COUNT_MISMATCH
SEARCH_1C=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=2&children=2&child_ages=5")
HTTP_1C=$(echo "$SEARCH_1C" | tail -n1)
BODY_1C=$(echo "$SEARCH_1C" | head -n-1)
assert_eq "Search child_ages count mismatch status code" "400" "$HTTP_1C"
assert_contains "Search child_ages count mismatch error code" "CHILD_AGE_COUNT_MISMATCH" "$BODY_1C"

# 1D. child age melebihi 17 -> ditolak 400 INVALID_CHILD_AGE
SEARCH_1D=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=2&children=1&child_ages=18")
HTTP_1D=$(echo "$SEARCH_1D" | tail -n1)
BODY_1D=$(echo "$SEARCH_1D" | head -n-1)
assert_eq "Search child age > 17 status code" "400" "$HTTP_1D"
assert_contains "Search child age > 17 error code" "INVALID_CHILD_AGE" "$BODY_1D"

# 1E. Search valid dengan dewasa dan anak sesuai aturan lolos 200 OK
SEARCH_1E=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=2&children=1&child_ages=6&rooms=1")
HTTP_1E=$(echo "$SEARCH_1E" | tail -n1)
assert_eq "Search valid boundary status code" "200" "$HTTP_1E"

# ------------------------------------------------------------------------------
# Test 2: Search Flagging Okupansi Varian Kamar (FR-02)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Evaluasi Okupansi Varian Kamar pada Search (FR-02) ---${NC}"

# 2A. 3 Dewasa di 1 Kamar: Superior King (MaxAdults=2) ditandai EXCEEDS_CAPACITY
SEARCH_2A=$(curl -s "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=3&rooms=1")
SUP_AVAIL_2A=$(echo "$SEARCH_2A" | jq -r '.results[] | select(.room_variant.code=="sup-king") | .available')
SUP_REASON_2A=$(echo "$SEARCH_2A" | jq -r '.results[] | select(.room_variant.code=="sup-king") | .unavailable_reason')
assert_eq "3 Dewasa sup-king available harus false" "false" "$SUP_AVAIL_2A"
assert_eq "3 Dewasa sup-king reason harus EXCEEDS_CAPACITY" "EXCEEDS_CAPACITY" "$SUP_REASON_2A"

# 2B. 2 Anak di 1 Kamar: Superior King (MaxChildren=1) ditandai EXCEEDS_CAPACITY
SEARCH_2B=$(curl -s "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=1&children=2&child_ages=4,8&rooms=1")
SUP_AVAIL_2B=$(echo "$SEARCH_2B" | jq -r '.results[] | select(.room_variant.code=="sup-king") | .available')
SUP_REASON_2B=$(echo "$SEARCH_2B" | jq -r '.results[] | select(.room_variant.code=="sup-king") | .unavailable_reason')
assert_eq "2 Anak sup-king available harus false" "false" "$SUP_AVAIL_2B"
assert_eq "2 Anak sup-king reason harus EXCEEDS_CAPACITY" "EXCEEDS_CAPACITY" "$SUP_REASON_2B"

# ------------------------------------------------------------------------------
# Test 3: Penjagaan Kapasitas Quote terhadap Data Katalog (FR-03)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Penjagaan Kapasitas Quote (FR-03) ---${NC}"

# 3A. Quote overcapacity total guests (5 tamu di 1 kamar Superior King kapasitas 3) -> 400 EXCEEDS_CAPACITY
QUOTE_3A=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":5}")
HTTP_3A=$(echo "$QUOTE_3A" | tail -n1)
BODY_3A=$(echo "$QUOTE_3A" | head -n-1)
assert_eq "Quote overcapacity status code" "400" "$HTTP_3A"
assert_contains "Quote overcapacity error code" "EXCEEDS_CAPACITY" "$BODY_3A"

# 3B. Quote num_guests kurang dari num_rooms (2 kamar, 1 tamu) -> 400 INVALID_GUEST_COUNT
QUOTE_3B=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":2,\"num_guests\":1}")
HTTP_3B=$(echo "$QUOTE_3B" | tail -n1)
BODY_3B=$(echo "$QUOTE_3B" | head -n-1)
assert_eq "Quote guests < rooms status code" "400" "$HTTP_3B"
assert_contains "Quote guests < rooms error code" "INVALID_GUEST_COUNT" "$BODY_3B"

# 3C. Quote breakdown adults melebihi max_adults (3 dewasa di 1 kamar) -> 400 EXCEEDS_CAPACITY
QUOTE_3C=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"adults\":3,\"children\":0}")
HTTP_3C=$(echo "$QUOTE_3C" | tail -n1)
BODY_3C=$(echo "$QUOTE_3C" | head -n-1)
assert_eq "Quote adults > max_adults status code" "400" "$HTTP_3C"
assert_contains "Quote adults > max_adults error code" "EXCEEDS_CAPACITY" "$BODY_3C"

# 3D. Quote breakdown children melebihi max_children (2 anak di 1 kamar) -> 400 EXCEEDS_CAPACITY
QUOTE_3D=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"adults\":1,\"children\":2,\"child_ages\":[4,7]}")
HTTP_3D=$(echo "$QUOTE_3D" | tail -n1)
BODY_3D=$(echo "$QUOTE_3D" | head -n-1)
assert_eq "Quote children > max_children status code" "400" "$HTTP_3D"
assert_contains "Quote children > max_children error code" "EXCEEDS_CAPACITY" "$BODY_3D"

# 3E. Quote tipe kamar tidak ditemukan -> 404 ROOM_NOT_FOUND
QUOTE_3E=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"nonexistent-room-type\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_3E=$(echo "$QUOTE_3E" | tail -n1)
BODY_3E=$(echo "$QUOTE_3E" | head -n-1)
assert_eq "Quote unknown room type status code" "404" "$HTTP_3E"
assert_contains "Quote unknown room type error code" "ROOM_NOT_FOUND" "$BODY_3E"

# 3F. Quote boundary valid (1 kamar, 3 tamu: 2 dewasa, 1 anak) -> 200 OK
QUOTE_3F=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"adults\":2,\"children\":1,\"child_ages\":[6]}")
HTTP_3F=$(echo "$QUOTE_3F" | tail -n1)
BODY_3F=$(echo "$QUOTE_3F" | head -n-1)
assert_eq "Quote valid boundary status code" "200" "$HTTP_3F"
VALID_QUOTE_ID=$(echo "$BODY_3F" | jq -r '.quote_id')
assert_contains "Valid Quote ID dihasilkan" "-" "$VALID_QUOTE_ID"

# ------------------------------------------------------------------------------
# Test 4: Penjagaan Direct Checkout / Bypass Search Overcapacity (FR-04)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Direct Checkout Bypass Overcapacity Guard (FR-04) ---${NC}"

# 4A. Direct booking call dengan num_guests melebihi kapasitas fisik (e.g. 6 guests di 1 kamar)
BOOK_4A=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":6,\"guest_name\":\"Direct Attacker\",\"guest_email\":\"attacker@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_4A=$(echo "$BOOK_4A" | tail -n1)
BODY_4A=$(echo "$BOOK_4A" | head -n-1)
assert_eq "Direct checkout overcapacity status code" "400" "$HTTP_4A"
assert_contains "Direct checkout overcapacity error code" "EXCEEDS_CAPACITY" "$BODY_4A"

# 4B. Direct booking call dengan num_guests < num_rooms (2 rooms, 1 guest)
BOOK_4B=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":2,\"num_guests\":1,\"guest_name\":\"Attacker\",\"guest_email\":\"attacker@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_4B=$(echo "$BOOK_4B" | tail -n1)
BODY_4B=$(echo "$BOOK_4B" | head -n-1)
assert_eq "Direct checkout guests < rooms status code" "400" "$HTTP_4B"
assert_contains "Direct checkout guests < rooms error code" "INVALID_GUEST_COUNT" "$BODY_4B"

# 4C. Direct booking call dengan unknown room type -> 404 ROOM_NOT_FOUND
BOOK_4C=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"invalid-unknown-uuid\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"Attacker\",\"guest_email\":\"attacker@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_4C=$(echo "$BOOK_4C" | tail -n1)
BODY_4C=$(echo "$BOOK_4C" | head -n-1)
assert_eq "Direct checkout unknown room type status code" "404" "$HTTP_4C"
assert_contains "Direct checkout unknown room type error code" "ROOM_NOT_FOUND" "$BODY_4C"

# ------------------------------------------------------------------------------
# Test 5: Siklus Lengkap Reservasi Boundary Valid (Search -> Quote -> Book -> Pay)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Siklus Lengkap Reservasi Boundary Valid ---${NC}"

# Menggunakan quote valid dari Test 3F (1 kamar, 3 tamu: 2 dewasa, 1 anak)
BOOK_5=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${VALID_QUOTE_ID}\",\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":3,\"guest_name\":\"Keluarga Bahagia\",\"guest_email\":\"keluarga@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_5=$(echo "$BOOK_5" | tail -n1)
BODY_5=$(echo "$BOOK_5" | head -n-1)
BOOKING_ID=$(echo "$BODY_5" | jq -r '.booking.id // .id')
STATUS_5=$(echo "$BODY_5" | jq -r '.booking.status // .status')
assert_eq "Booking awal berstatus pending" "pending" "$STATUS_5"

# Simulasi pembayaran fake-pay
FAKE_PAY=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/fake-pay/ref-success?booking_id=${BOOKING_ID}")
HTTP_PAY=$(echo "$FAKE_PAY" | tail -n1)
assert_eq "Fake payment status code" "200" "$HTTP_PAY"

# Verifikasi reservasi terkonfirmasi
GUEST_DETAIL=$(curl -s "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
CONFIRMED_STATUS=$(echo "$GUEST_DETAIL" | jq -r '.status // .booking.status')
assert_eq "Status akhir booking confirmed" "confirmed" "$CONFIRMED_STATUS"

# ==============================================================================
# Ringkasan Pengujian
# ==============================================================================
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "  Total Assertions : ${TOTAL}"
echo -e "  ${GREEN}Passed           : ${PASSED}${NC}"
if [ $FAILED -gt 0 ]; then
    echo -e "  ${RED}Failed           : ${FAILED}${NC}"
    exit 1
else
    echo -e "  ${GREEN}ALL E2E ASSERTIONS PASSED!${NC}"
fi
echo -e "${BLUE}=================================================================${NC}\n"
