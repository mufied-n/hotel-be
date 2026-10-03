#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Cancellation Deadline Timezone Alignment (BE-R12)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Kebijakan 48 jam sebelum 14:00 WIB (07:00 UTC) pada tanggal check-in.
# Menguji penolakan pembatalan lewat deadline, penerimaan sebelum deadline,
# dan penolakan tarif non-refundable.
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

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}   E2E Test: BE-R12 Cancellation Deadline WIB Timezone Alignment  ${NC}"
echo -e "${BLUE}=================================================================${NC}"

RT_ID="01900000-0000-7000-8000-000000000001"

# Helper untuk membuat booking dan fake-pay agar berstatus CONFIRMED
create_confirmed_booking() {
    local ci="$1"
    local co="$2"
    local promo="${3:-}"
    
    local q_payload="{\"room_type_id\":\"${RT_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${ci}\",\"check_out\":\"${co}\",\"num_rooms\":1,\"num_guests\":2"
    if [ -n "$promo" ]; then
        q_payload="${q_payload},\"promo_code\":\"${promo}\"}"
    else
        q_payload="${q_payload}}"
    fi

    local q_resp
    q_resp=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" -H "Content-Type: application/json" -d "${q_payload}")
    local q_id
    q_id=$(echo "$q_resp" | jq -r '.quote_id')

    local b_payload=$(cat <<EOF
{
  "quote_id": "${q_id}",
  "terms_accepted": true,
  "privacy_accepted": true,
  "room_type_id": "${RT_ID}",
  "check_in": "${ci}",
  "check_out": "${co}",
  "num_rooms": 1,
  "num_guests": 2,
  "guest_name": "R12 Tester",
  "guest_email": "r12.cancellation@example.com"
}
EOF
)

    local b_resp
    b_resp=$(curl -s -X POST "${BASE_URL}/api/v1/bookings" -H "Content-Type: application/json" -d "${b_payload}")
    local b_id
    local token
    local ref
    b_id=$(echo "$b_resp" | jq -r '.booking.id')
    token=$(echo "$b_resp" | jq -r '.guest_access_token')
    ref=$(echo "$b_resp" | jq -r '.reference')

    # Konfirmasi pembayaran lewat fake-pay dengan booking ID (UUID)
    local fp_resp
    fp_resp=$(curl -s -X POST "${BASE_URL}/fake-pay/${b_id}")
    local fp_status
    fp_status=$(echo "$fp_resp" | jq -r '.status // empty')
    if [ "$fp_status" == "400" ]; then
        echo -e "${RED}FakePay failed: ${fp_resp}${NC}" >&2
        exit 1
    fi

    echo "${b_id}|${token}|${ref}"
}

# ------------------------------------------------------------------------------
# Test 1: Booking Confirmed dengan Check-in Jauh (Bisa Dibatalkan Gratis)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Confirmed Booking Jauh di Masa Depan (Bebas Batas Waktu) ---${NC}"
CI_FUTURE=$(date -u -d "+10 days" +%Y-%m-%d)
CO_FUTURE=$(date -u -d "+12 days" +%Y-%m-%d)

RES1=$(create_confirmed_booking "$CI_FUTURE" "$CO_FUTURE")
B1_ID=$(echo "$RES1" | cut -d'|' -f1)
B1_TOKEN=$(echo "$RES1" | cut -d'|' -f2)

CANCEL_RESP1=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${B1_ID}/cancel" \
    -H "X-Guest-Token: ${B1_TOKEN}")
CODE1=$(echo "$CANCEL_RESP1" | tail -n1)
BODY1=$(echo "$CANCEL_RESP1" | sed '$d')

assert_eq "Pembatalan confirmed sebelum deadline berhasil" "200" "$CODE1"
assert_eq "Status pembatalan adalah cancelled" "cancelled" "$(echo "$BODY1" | jq -r '.status')"

# ------------------------------------------------------------------------------
# Test 2: Booking Confirmed Melewati Batas Waktu (Kurang dari 48 jam sebelum 14:00 WIB)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Confirmed Booking Melewati Batas Waktu (Kurang dari 48 jam) ---${NC}"
CI_PAST=$(date -u -d "+1 days" +%Y-%m-%d)
CO_PAST=$(date -u -d "+3 days" +%Y-%m-%d)

RES2=$(create_confirmed_booking "$CI_PAST" "$CO_PAST")
B2_ID=$(echo "$RES2" | cut -d'|' -f1)
B2_TOKEN=$(echo "$RES2" | cut -d'|' -f2)

CANCEL_RESP2=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${B2_ID}/cancel" \
    -H "X-Guest-Token: ${B2_TOKEN}")
CODE2=$(echo "$CANCEL_RESP2" | tail -n1)
BODY2=$(echo "$CANCEL_RESP2" | sed '$d')

assert_eq "Pembatalan lewat deadline ditolak dengan HTTP 409 Conflict" "409" "$CODE2"
assert_contains "Error code CANCELLATION_DEADLINE_EXCEEDED" "CANCELLATION_DEADLINE_EXCEEDED" "$BODY2"
assert_contains "Pesan error batas waktu 48 jam sebelum check-in" "batas waktu pembatalan gratis 48 jam sebelum check-in telah terlewati" "$BODY2"

# Verifikasi booking tetap confirmed di DB
READ_B2=$(curl -s -H "X-Guest-Token: ${B2_TOKEN}" "${BASE_URL}/api/v1/bookings/${B2_ID}")
assert_eq "Booking tetap berstatus confirmed setelah penolakan cancel" "confirmed" "$(echo "$READ_B2" | jq -r '.status')"

# ------------------------------------------------------------------------------
# Test 3: Confirmed Booking dengan Promo Non-Refundable (OCTOBREAK)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Confirmed Booking Non-Refundable (Promo OCTOBREAK) ---${NC}"
RES3=$(create_confirmed_booking "$CI_FUTURE" "$CO_FUTURE" "OCTOBREAK")
B3_ID=$(echo "$RES3" | cut -d'|' -f1)
B3_TOKEN=$(echo "$RES3" | cut -d'|' -f2)

CANCEL_RESP3=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${B3_ID}/cancel" \
    -H "X-Guest-Token: ${B3_TOKEN}")
CODE3=$(echo "$CANCEL_RESP3" | tail -n1)
BODY3=$(echo "$CANCEL_RESP3" | sed '$d')

assert_eq "Pembatalan non-refundable ditolak dengan HTTP 409 Conflict" "409" "$CODE3"
assert_contains "Error code NON_REFUNDABLE_BOOKING" "NON_REFUNDABLE_BOOKING" "$BODY3"

# ------------------------------------------------------------------------------
# Test 4: Pending Hold Booking (Non-Refundable) dapat Dibatalkan (Release Hold)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Pending Hold Booking Non-Refundable (Bisa Dilepas) ---${NC}"
Q4_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${RT_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CI_FUTURE}\",\"check_out\":\"${CO_FUTURE}\",\"num_rooms\":1,\"num_guests\":2,\"promo_code\":\"OCTOBREAK\"}")
Q4_ID=$(echo "$Q4_RESP" | jq -r '.quote_id')

B4_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/bookings" -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${Q4_ID}\",\"terms_accepted\":true,\"privacy_accepted\":true,\"room_type_id\":\"${RT_ID}\",\"check_in\":\"${CI_FUTURE}\",\"check_out\":\"${CO_FUTURE}\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"R12 Hold\",\"guest_email\":\"hold@example.com\"}")
B4_ID=$(echo "$B4_RESP" | jq -r '.booking.id')
B4_TOKEN=$(echo "$B4_RESP" | jq -r '.guest_access_token')

CANCEL_RESP4=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${B4_ID}/cancel" \
    -H "X-Guest-Token: ${B4_TOKEN}")
CODE4=$(echo "$CANCEL_RESP4" | tail -n1)

assert_eq "Hold pending dapat dibatalkan (dilepas)" "200" "$CODE4"

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}                       RINGKASAN HASIL E2E                       ${NC}"
echo -e "${BLUE}=================================================================${NC}"
echo -e "Total Assertion : $TOTAL"
echo -e "Passed          : ${GREEN}$PASSED${NC}"
echo -e "Failed          : ${RED}$FAILED${NC}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}Semua pengujian kebijakan batas waktu pembatalan WIB (BE-R12) SUKSES!${NC}\n"
    exit 0
else
    echo -e "\n${RED}Terdapat kegagalan pada pengujian BE-R12!${NC}\n"
    exit 1
fi
