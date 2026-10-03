#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Public DTO Privacy & Token Protection (BE-R02)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: UU PDP No. 27/2022, Free-text masking, Arrival time masking,
# Least-privilege Credential Isolation (Staff token redaction, Guest read redaction).
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"

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
    if echo "$haystack" | grep -q "$needle"; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Contains: ${needle})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Did not find: ${needle}"
        echo -e "       Content: ${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

assert_not_contains() {
    local desc="$1"
    local needle="$2"
    local haystack="$3"
    TOTAL=$((TOTAL + 1))
    if ! echo "$haystack" | grep -q "$needle"; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Does NOT contain: ${needle})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Found forbidden string: ${needle}"
        echo -e "       Content: ${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}   E2E Test: BE-R02 Public DTO Privacy & Token Protection       ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# 1. Buat Quote terlebih dahulu
CHECK_IN="2026-11-20"
CHECK_OUT="2026-11-22"
RT_ID="01900000-0000-7000-8000-000000000001"

echo -e "\n${YELLOW}--- 1. Menyiapkan Quote & Booking Baru dengan Catatan Khusus Sensitif ---${NC}"
QUOTE_PAYLOAD=$(cat <<EOF
{
  "room_type_id": "${RT_ID}",
  "rate_plan_code": "room_only",
  "check_in": "${CHECK_IN}",
  "check_out": "${CHECK_OUT}",
  "num_rooms": 1,
  "num_guests": 2
}
EOF
)

QUOTE_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "${QUOTE_PAYLOAD}")
QUOTE_CODE=$(echo "$QUOTE_RESP" | tail -n1)
QUOTE_BODY=$(echo "$QUOTE_RESP" | sed '$d')

assert_eq "Hitung quote kalkulasi" "200" "$QUOTE_CODE"
QUOTE_ID=$(echo "$QUOTE_BODY" | jq -r '.quote_id')

# 2. Inisiasi Booking Baru dengan SpecialRequests sensitif dan EstimatedArrivalTime
SENSITIVE_NOTE="MARKER_SENSITIF_GUEST_ALERGI_DEBU_DAN_HONEYMOON_REQUEST_999"
ARRIVAL_TIME="15:00"

BOOKING_PAYLOAD=$(cat <<EOF
{
  "quote_id": "${QUOTE_ID}",
  "terms_accepted": true,
  "privacy_accepted": true,
  "room_type_id": "${RT_ID}",
  "check_in": "${CHECK_IN}",
  "check_out": "${CHECK_OUT}",
  "num_rooms": 1,
  "num_guests": 2,
  "guest_name": "Raden Mas Danang",
  "guest_email": "danang.privacy@example.com",
  "guest_phone": "+6281298765432",
  "estimated_arrival_time": "${ARRIVAL_TIME}",
  "special_requests": "${SENSITIVE_NOTE}"
}
EOF
)

BOOK_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "${BOOKING_PAYLOAD}")
BOOK_CODE=$(echo "$BOOK_RESP" | tail -n1)
BOOK_BODY=$(echo "$BOOK_RESP" | sed '$d')

assert_eq "Inisiasi booking hold berhasil" "201" "$BOOK_CODE"
BOOKING_ID=$(echo "$BOOK_BODY" | jq -r '.booking.id')
GUEST_ACCESS_TOKEN=$(echo "$BOOK_BODY" | jq -r '.guest_access_token')

if [ -z "$BOOKING_ID" ] || [ "$BOOKING_ID" == "null" ]; then
    echo -e "${RED}Fatal: Booking ID tidak didapatkan${NC}"
    exit 1
fi
if [ -z "$GUEST_ACCESS_TOKEN" ] || [ "$GUEST_ACCESS_TOKEN" == "null" ]; then
    echo -e "${RED}Fatal: Guest access token tidak didapatkan${NC}"
    exit 1
fi

echo -e "\n${YELLOW}--- 2. Public Access: Unauthenticated Caller (Tanpa Token) ---${NC}"
PUB_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
PUB_CODE=$(echo "$PUB_RESP" | tail -n1)
PUB_BODY=$(echo "$PUB_RESP" | sed '$d')

assert_eq "Public read status code" "200" "$PUB_CODE"
assert_eq "Public read ID" "$BOOKING_ID" "$(echo "$PUB_BODY" | jq -r '.id')"
assert_eq "Public read room_type_id" "$RT_ID" "$(echo "$PUB_BODY" | jq -r '.room_type_id')"
assert_not_contains "Public response tidak membocorkan special_requests" "special_requests" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan marker sensitif" "$SENSITIVE_NOTE" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan estimated_arrival_time" "estimated_arrival_time" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan nama tamu" "guest_name" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan email tamu" "guest_email" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan nomor telepon" "guest_phone" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan token tamu" "guest_token" "$PUB_BODY"
assert_not_contains "Public response tidak membocorkan rincian harga" "total_price_minor" "$PUB_BODY"

echo -e "\n${YELLOW}--- 3. Negative Ownership: Token Palsu / Acak ---${NC}"
FAKE_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}" \
    -H "X-Guest-Token: gst_fake_token_attacker_123")
FAKE_CODE=$(echo "$FAKE_RESP" | tail -n1)
FAKE_BODY=$(echo "$FAKE_RESP" | sed '$d')

assert_eq "Caller token palsu tetap dapat 200 (PublicDTO fallback)" "200" "$FAKE_CODE"
assert_not_contains "Caller token palsu tidak dapat membaca special_requests" "special_requests" "$FAKE_BODY"
assert_not_contains "Caller token palsu tidak dapat membaca marker sensitif" "$SENSITIVE_NOTE" "$FAKE_BODY"
assert_not_contains "Caller token palsu tidak dapat membaca arrival time" "estimated_arrival_time" "$FAKE_BODY"
assert_not_contains "Caller token palsu tidak dapat membaca token tamu" "guest_token" "$FAKE_BODY"

echo -e "\n${YELLOW}--- 4. Negative Ownership: Token Milik Booking Lain ---${NC}"
OTHER_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}" \
    -H "X-Guest-Token: gst_other_booking_token_999")
OTHER_CODE=$(echo "$OTHER_RESP" | tail -n1)
OTHER_BODY=$(echo "$OTHER_RESP" | sed '$d')

assert_eq "Caller token booking lain dapat 200 (PublicDTO fallback)" "200" "$OTHER_CODE"
assert_not_contains "Caller token booking lain tidak membaca special_requests" "special_requests" "$OTHER_BODY"
assert_not_contains "Caller token booking lain tidak membaca marker sensitif" "$SENSITIVE_NOTE" "$OTHER_BODY"

echo -e "\n${YELLOW}--- 5. Authorized Guest Owner: Membaca Booking Sendiri ---${NC}"
OWNER_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}" \
    -H "X-Guest-Token: ${GUEST_ACCESS_TOKEN}")
OWNER_CODE=$(echo "$OWNER_RESP" | tail -n1)
OWNER_BODY=$(echo "$OWNER_RESP" | sed '$d')

assert_eq "Tamu terautentikasi status code" "200" "$OWNER_CODE"
assert_eq "Tamu terautentikasi dapat membaca special_requests" "$SENSITIVE_NOTE" "$(echo "$OWNER_BODY" | jq -r '.special_requests')"
assert_eq "Tamu terautentikasi dapat membaca arrival_time" "$ARRIVAL_TIME" "$(echo "$OWNER_BODY" | jq -r '.estimated_arrival_time')"
assert_eq "Tamu terautentikasi dapat membaca nama" "Raden Mas Danang" "$(echo "$OWNER_BODY" | jq -r '.guest_name')"
assert_eq "Tamu terautentikasi dapat membaca email" "danang.privacy@example.com" "$(echo "$OWNER_BODY" | jq -r '.guest_email')"
assert_not_contains "Response GET pemilik tidak menyertakan guest_token ulang" "guest_token" "$OWNER_BODY"

echo -e "\n${YELLOW}--- 6. Staff Role Verification: Pemalsuan Role vs Sesi Staf Asli ---${NC}"
# 6a. Pemalsuan role: Caller mengirim Bearer receptionist tanpa sesi valid (BE-R01)
FORGE_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}" \
    -H "Authorization: Bearer receptionist")
FORGE_CODE=$(echo "$FORGE_RESP" | tail -n1)
FORGE_BODY=$(echo "$FORGE_RESP" | sed '$d')

assert_eq "Caller dengan Bearer palsu tidak mendapatkan data privat (PublicDTO)" "200" "$FORGE_CODE"
assert_not_contains "Caller Bearer palsu tidak dapat membaca special_requests" "special_requests" "$FORGE_BODY"
assert_not_contains "Caller Bearer palsu tidak dapat membaca arrival time" "estimated_arrival_time" "$FORGE_BODY"

# 6b. Sesi staf resmi: Login via /api/v1/auth/staff/login
STAFF_LOGIN_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/auth/staff/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"fo_receptionist","password":"password-receptionist-123"}')
STAFF_TOKEN=$(echo "$STAFF_LOGIN_RESP" | jq -r '.token')

if [ -z "$STAFF_TOKEN" ] || [ "$STAFF_TOKEN" == "null" ]; then
    echo -e "${RED}Fatal: Login staf gagal: ${STAFF_LOGIN_RESP}${NC}"
    exit 1
fi

STAFF_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${BOOKING_ID}" \
    -H "Authorization: Bearer ${STAFF_TOKEN}")
STAFF_CODE=$(echo "$STAFF_RESP" | tail -n1)
STAFF_BODY=$(echo "$STAFF_RESP" | sed '$d')

assert_eq "Staf terverifikasi status code" "200" "$STAFF_CODE"
assert_eq "Staf dapat membaca special_requests operasional" "$SENSITIVE_NOTE" "$(echo "$STAFF_BODY" | jq -r '.special_requests')"
assert_eq "Staf dapat membaca estimated_arrival_time operasional" "$ARRIVAL_TIME" "$(echo "$STAFF_BODY" | jq -r '.estimated_arrival_time')"
assert_not_contains "Staf TIDAK BOLEH menerima guest_token rahasia tamu (BE-R02)" "guest_token" "$STAFF_BODY"

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}                       RINGKASAN HASIL E2E                       ${NC}"
echo -e "${BLUE}=================================================================${NC}"
echo -e "Total Assertion : $TOTAL"
echo -e "Passed          : ${GREEN}$PASSED${NC}"
echo -e "Failed          : ${RED}$FAILED${NC}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}Semua pengujian privasi PublicDTO dan sanitasi token (BE-R02) SUKSES!${NC}\n"
    exit 0
else
    echo -e "\n${RED}Terdapat kegagalan pada pengujian BE-R02!${NC}\n"
    exit 1
fi
