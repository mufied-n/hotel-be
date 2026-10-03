#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Kebijakan Biaya Sarapan & Multi-Room Guest Tiers (BE-R10)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Bintang 4 Pulang ke Uttara, Paritas Biaya Sarapan & Multi-Room
# Menguji:
# 1. Single-room Bed & Breakfast kalkulasi dasar
# 2. Multi-room (2 kamar, 4 dewasa) eliminasi bug double counting (800k -> 400k/malam)
# 3. Multi-room (3 kamar, 6 dewasa) eliminasi pengali jumlah kamar
# 4. Evaluasi berjenjang usia anak (0-5 thn gratis, 6-11 thn 50%, >=12 thn penuh)
# 5. Siklus pemesanan lengkap multi-room Bed & Breakfast hingga terkonfirmasi
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONTAINER="${PG_CONTAINER:-r10-pg}"

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
echo -e "${BLUE}  E2E Test: BE-R10 Biaya Sarapan & Multi-Room Guest Tiers        ${NC}"
echo -e "${BLUE}=================================================================${NC}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

# 0. Healthcheck & Staff Auth
echo -e "\n${YELLOW}--- 0. Service Healthcheck ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Healthcheck OK" '"status":"ok"' "$HEALTH"

CHECK_IN=$(date -d "+10 days" +%Y-%m-%d)
CHECK_OUT=$(date -d "+12 days" +%Y-%m-%d) # 2 malam weekday
SUP_KING_ID="01900000-0000-7000-8000-000000000001" # 550.000 / malam
JSTE_ID="01900000-0000-7000-8000-000000000006"     # 1.650.000 / malam

# Pastikan harga dasar Superior King pada database dalam kondisi seed awal 550.000
docker exec "$PG_CONTAINER" psql -U postgres -d hotel_booking -c \
    "UPDATE room_types SET base_price_minor = 550000 WHERE code = 'sup-king';" > /dev/null

# ------------------------------------------------------------------------------
# Test 1: Single Room Bed & Breakfast (1 Kamar, 2 Dewasa, 2 Malam)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Single Room Bed & Breakfast (1 Kamar, 2 Dewasa, 2 Malam) ---${NC}"

# Subtotal kamar: 550k * 2 = 1.100.000
# Sarapan: 2 dewasa * 2 malam * 100k = 400.000
# Pajak 10%: (1.1M + 400k) * 10% = 150.000
# Total: 1.650.000
QUOTE_1=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"bed_and_breakfast\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"adults\":2,\"children\":0}")
HTTP_1=$(echo "$QUOTE_1" | tail -n1)
BODY_1=$(echo "$QUOTE_1" | head -n-1)

assert_eq "Quote 1 kamar BB status code" "200" "$HTTP_1"
ROOM_SUBTOTAL_1=$(echo "$BODY_1" | jq -r '.pricing.room_subtotal_minor')
BF_CHARGE_1=$(echo "$BODY_1" | jq -r '.pricing.breakfast_charge_minor')
TOTAL_1=$(echo "$BODY_1" | jq -r '.pricing.total_price_minor')

assert_eq "Subtotal kamar 1.100.000" "1100000" "$ROOM_SUBTOTAL_1"
assert_eq "Biaya sarapan 1 kamar 400.000" "400000" "$BF_CHARGE_1"
assert_eq "Total tagihan 1.650.000" "1650000" "$TOTAL_1"

# ------------------------------------------------------------------------------
# Test 2: Multi-Room Bed & Breakfast (2 Kamar, 4 Dewasa, 2 Malam) — Fix Bug BE-R10
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Multi-Room Bed & Breakfast (2 Kamar, 4 Dewasa, 2 Malam) ---${NC}"

# Subtotal kamar: 550k * 2 malam * 2 kamar = 2.200.000
# Sarapan: 4 dewasa * 2 malam * 100k = 800.000 (BUKAN 1.600.000 karena tidak lagi dikalikan numRooms!)
# Pajak 10%: (2.2M + 800k) * 10% = 300.000
# Total: 3.300.000
QUOTE_2=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"bed_and_breakfast\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":2,\"adults\":4,\"children\":0}")
HTTP_2=$(echo "$QUOTE_2" | tail -n1)
BODY_2=$(echo "$QUOTE_2" | head -n-1)

assert_eq "Quote 2 kamar BB status code" "200" "$HTTP_2"
ROOM_SUBTOTAL_2=$(echo "$BODY_2" | jq -r '.pricing.room_subtotal_minor')
BF_CHARGE_2=$(echo "$BODY_2" | jq -r '.pricing.breakfast_charge_minor')
TOTAL_2=$(echo "$BODY_2" | jq -r '.pricing.total_price_minor')
QUOTE_2_ID=$(echo "$BODY_2" | jq -r '.quote_id')

assert_eq "Subtotal 2 kamar 2.200.000" "2200000" "$ROOM_SUBTOTAL_2"
assert_eq "Biaya sarapan 2 kamar 800.000 (tidak double counting)" "800000" "$BF_CHARGE_2"
assert_eq "Total tagihan 2 kamar 3.300.000" "3300000" "$TOTAL_2"

# ------------------------------------------------------------------------------
# Test 3: Multi-Room Bed & Breakfast (3 Kamar, 6 Dewasa, 1 Malam)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Multi-Room Bed & Breakfast (3 Kamar, 6 Dewasa, 1 Malam) ---${NC}"

CHECK_OUT_1NIGHT=$(date -d "+11 days" +%Y-%m-%d)
# Subtotal kamar: 550k * 1 malam * 3 kamar = 1.650.000
# Sarapan: 6 dewasa * 1 malam * 100k = 600.000 (BUKAN 1.800.000!)
# Pajak 10%: (1.65M + 600k) * 10% = 225.000
# Total: 2.475.000
QUOTE_3=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"bed_and_breakfast\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT_1NIGHT}\",\"num_rooms\":3,\"adults\":6,\"children\":0}")
HTTP_3=$(echo "$QUOTE_3" | tail -n1)
BODY_3=$(echo "$QUOTE_3" | head -n-1)

assert_eq "Quote 3 kamar BB status code" "200" "$HTTP_3"
ROOM_SUBTOTAL_3=$(echo "$BODY_3" | jq -r '.pricing.room_subtotal_minor')
BF_CHARGE_3=$(echo "$BODY_3" | jq -r '.pricing.breakfast_charge_minor')
TOTAL_3=$(echo "$BODY_3" | jq -r '.pricing.total_price_minor')

assert_eq "Subtotal 3 kamar 1.650.000" "1650000" "$ROOM_SUBTOTAL_3"
assert_eq "Biaya sarapan 3 kamar 600.000 (tidak triple counting)" "600000" "$BF_CHARGE_3"
assert_eq "Total tagihan 3 kamar 2.475.000" "2475000" "$TOTAL_3"

# ------------------------------------------------------------------------------
# Test 4: Child Age Tiers (2 Dewasa, Balita 4 thn, Anak 8 thn, Remaja 15 thn)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Child Age Tiers (Balita Gratis, Anak 50%, Remaja 100%) ---${NC}"

# Junior Suite (1.650.000 / kamar / malam), 2 kamar, 2 malam
# Subtotal kamar: 1.650.000 * 2 kamar * 2 malam = 6.600.000
# Tamu (Total 5 tamu terdistribusi dalam 2 kamar):
# - 2 Dewasa: 2 * 100k = 200k/malam
# - 1 Balita (4 thn): gratis = 0
# - 1 Anak (8 thn): 50% = 50k/malam
# - 1 Remaja (15 thn): 100% = 100k/malam
# Total sarapan per malam = 350.000 (TIDAK dikalikan 2 kamar!)
# Sarapan 2 malam = 700.000
# Pajak 10%: (6.6M + 700k) * 10% = 730.000
# Total: 8.030.000
QUOTE_4=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${JSTE_ID}\",\"rate_plan_code\":\"bed_and_breakfast\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":2,\"adults\":2,\"children\":3,\"child_ages\":[4,8,15]}")
HTTP_4=$(echo "$QUOTE_4" | tail -n1)
BODY_4=$(echo "$QUOTE_4" | head -n-1)

assert_eq "Quote Family Child Tiers status code" "200" "$HTTP_4"
BF_CHARGE_4=$(echo "$BODY_4" | jq -r '.pricing.breakfast_charge_minor')
TOTAL_4=$(echo "$BODY_4" | jq -r '.pricing.total_price_minor')
ADULTS_4=$(echo "$BODY_4" | jq -r '.adults')
CHILDREN_4=$(echo "$BODY_4" | jq -r '.children')
AGES_4_COUNT=$(echo "$BODY_4" | jq -r '.child_ages | length')

assert_eq "Biaya sarapan keluarga dengan child tiers tepat 700.000" "700000" "$BF_CHARGE_4"
assert_eq "Total tagihan keluarga 8.030.000" "8030000" "$TOTAL_4"
assert_eq "Snapshot quote adults 2" "2" "$ADULTS_4"
assert_eq "Snapshot quote children 3" "3" "$CHILDREN_4"
assert_eq "Snapshot quote child_ages count 3" "3" "$AGES_4_COUNT"

# ------------------------------------------------------------------------------
# Test 5: Siklus Pemesanan Lengkap Multi-Room Bed & Breakfast
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Siklus Pemesanan Lengkap Multi-Room Bed & Breakfast ---${NC}"

# Menggunakan QUOTE_2_ID (2 kamar, 4 dewasa, total 3.300.000)
BOOK_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${QUOTE_2_ID}\",\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":2,\"num_guests\":4,\"guest_name\":\"Rombongan Bisnis\",\"guest_email\":\"bisnis@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_BOOK=$(echo "$BOOK_RES" | tail -n1)
BODY_BOOK=$(echo "$BOOK_RES" | head -n-1)

assert_eq "Booking multi-room status code" "201" "$HTTP_BOOK"
BOOKING_ID=$(echo "$BODY_BOOK" | jq -r '.booking.id // .id')
BOOKED_TOTAL=$(echo "$BODY_BOOK" | jq -r '.pricing.total_price_minor // .booking.total_price_minor')
assert_eq "Total tagihan booking 3.300.000" "3300000" "$BOOKED_TOTAL"

# Fake Pay
PAY_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/fake-pay/ref-success?booking_id=${BOOKING_ID}")
HTTP_PAY=$(echo "$PAY_RES" | tail -n1)
assert_eq "Fake payment status code" "200" "$HTTP_PAY"

# Verifikasi booking confirmed
DETAIL_RES=$(curl -s "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
FINAL_STATUS=$(echo "$DETAIL_RES" | jq -r '.status // .booking.status')
assert_eq "Status akhir booking confirmed" "confirmed" "$FINAL_STATUS"

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
