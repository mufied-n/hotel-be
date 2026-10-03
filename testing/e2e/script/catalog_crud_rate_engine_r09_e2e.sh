#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Integrasi Harga Katalog CRUD ke Dynamic Rate Engine (BE-R09)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Bintang 4 Pulang ke Uttara, Paritas Katalog-Search-Quote
# Menguji:
# 1. Resolusi tarif dinamis varian awal katalog pada Search dan Quote
# 2. Pembaruan harga via CRUD PUT /api/v1/catalog/rooms/:id langsung memengaruhi Quote & Search
# 3. Penambahan varian baru via CRUD POST /api/v1/catalog/rooms langsung quotable & bookable
# 4. Penjagaan Search Pricing Guard: kamar tanpa tarif valid ditandai RATE_UNAVAILABLE (tidak gratis)
# 5. Penjagaan Quote Guard: quote kamar unpriced ditolak 400 RATE_UNAVAILABLE
# 6. Imutabilitas quote terkunci: perubahan harga katalog tidak mengubah harga quote yang sudah terbit
# 7. Siklus checkout lengkap menggunakan quote hasil sinkronisasi katalog
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONTAINER="${PG_CONTAINER:-r09-pg}"

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
echo -e "${BLUE}  E2E Test: BE-R09 Sinkronisasi Harga Katalog ke Rate Engine     ${NC}"
echo -e "${BLUE}=================================================================${NC}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

# 0. Healthcheck & Staff Auth Setup
echo -e "\n${YELLOW}--- 0. Service Healthcheck & Staff Auth ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Healthcheck OK" '"status":"ok"' "$HEALTH"

export STAFF_PASSWORD="${STAFF_PASSWORD:-Password12345!}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib_staff_login.sh"
load_staff_tokens
echo -e "  ${GREEN}✓${NC} Berhasil login staf (GM Admin & Revenue Mgr)"

# Check-in hari Rabu (weekday) 10 hari lagi
CHECK_IN=$(date -d "+10 days" +%Y-%m-%d)
CHECK_OUT=$(date -d "+12 days" +%Y-%m-%d)
SUP_KING_ID="01900000-0000-7000-8000-000000000001"

# Pastikan harga dasar Superior King pada database dalam kondisi seed awal 550.000
docker exec "$PG_CONTAINER" psql -U postgres -d hotel_booking -c \
    "UPDATE room_types SET base_price_minor = 550000 WHERE code = 'sup-king';" > /dev/null

# ------------------------------------------------------------------------------
# Test 1: Resolusi Tarif Kamar Awal (Superior King)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Resolusi Tarif Kamar Awal (Superior King: 550.000) ---${NC}"

# 1A. Panggilan Quote untuk Superior King (2 malam weekday)
# Tarif dasar katalog = 550.000 * 2 malam = 1.100.000 subtotal, Pajak 10% = 110.000, Total = 1.210.000
QUOTE_1=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_1=$(echo "$QUOTE_1" | tail -n1)
BODY_1=$(echo "$QUOTE_1" | head -n-1)

assert_eq "Quote Superior King status code" "200" "$HTTP_1"
SUBTOTAL_1=$(echo "$BODY_1" | jq -r '.pricing.room_subtotal_minor')
TOTAL_1=$(echo "$BODY_1" | jq -r '.pricing.total_price_minor')
assert_eq "Subtotal awal Superior King 1.100.000" "1100000" "$SUBTOTAL_1"
assert_eq "Total awal Superior King 1.210.000" "1210000" "$TOTAL_1"

# ------------------------------------------------------------------------------
# Test 2: Pembaruan Harga via CRUD PUT /api/v1/catalog/rooms/:id
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Pembaruan Harga Katalog CRUD (PUT /api/v1/catalog/rooms/:id) ---${NC}"

# Revenue manager menaikkan harga Superior King menjadi 700.000
PUT_PAYLOAD=$(cat <<EOF
{
  "code": "sup-king",
  "name": "Superior King Renovation Special",
  "family_name": "Superior",
  "bed_type": "1 King Bed",
  "room_size_sqm": 28,
  "max_capacity": 3,
  "max_adults": 2,
  "max_children": 1,
  "base_price_minor": 700000,
  "description": "Kamar Superior King dengan pemandangan Gunung Merapi dan fasilitas modern."
}
EOF
)

PUT_RES=$(curl -s -w "\n%{http_code}" -X PUT "${BASE_URL}/api/v1/catalog/rooms/${SUP_KING_ID}" \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    -H "Content-Type: application/json" \
    -d "$PUT_PAYLOAD")
HTTP_PUT=$(echo "$PUT_RES" | tail -n1)
BODY_PUT=$(echo "$PUT_RES" | head -n-1)

assert_eq "PUT catalog room status code" "200" "$HTTP_PUT"
NEW_PRICE=$(echo "$BODY_PUT" | jq -r '.base_price_minor')
assert_eq "Harga katalog terbarui menjadi 700.000" "700000" "$NEW_PRICE"

# 2B. Verifikasi Quote baru langsung merefleksikan harga 700.000
# 2 malam * 700.000 = 1.400.000 subtotal, Pajak 10% = 140.000, Total = 1.540.000
QUOTE_2=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_2=$(echo "$QUOTE_2" | tail -n1)
BODY_2=$(echo "$QUOTE_2" | head -n-1)

assert_eq "Quote Superior King setelah update status code" "200" "$HTTP_2"
SUBTOTAL_2=$(echo "$BODY_2" | jq -r '.pricing.room_subtotal_minor')
TOTAL_2=$(echo "$BODY_2" | jq -r '.pricing.total_price_minor')
assert_eq "Subtotal baru Superior King 1.400.000" "1400000" "$SUBTOTAL_2"
assert_eq "Total baru Superior King 1.540.000" "1540000" "$TOTAL_2"

# 2C. Verifikasi Search Rooms langsung merefleksikan harga baru
SEARCH_2=$(curl -s "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=2&rooms=1")
SK_SEARCH_PRICE=$(echo "$SEARCH_2" | jq -r '.results[] | select(.room_variant.code=="sup-king") | .total_price_minor')
assert_eq "Search Superior King total_price_minor terbarui" "1400000" "$SK_SEARCH_PRICE"

# ------------------------------------------------------------------------------
# Test 3: Pembuatan Varian Baru via CRUD POST /api/v1/catalog/rooms
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Pembuatan Varian Baru CRUD (POST /api/v1/catalog/rooms) ---${NC}"

PENTHOUSE_CODE="penthouse-$(date +%s)"
POST_PAYLOAD=$(cat <<EOF
{
  "code": "${PENTHOUSE_CODE}",
  "name": "Uttara Grand Penthouse",
  "family_name": "Penthouse",
  "bed_type": "2 Super King Beds",
  "room_size_sqm": 120,
  "max_capacity": 6,
  "max_adults": 4,
  "max_children": 3,
  "base_price_minor": 4500000,
  "description": "Penthouse mewah di lantai teratas dengan pemandangan 360 derajat kota Yogyakarta."
}
EOF
)

POST_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/catalog/rooms" \
    -H "Authorization: Bearer ${T_GM_ADMIN}" \
    -H "Content-Type: application/json" \
    -d "$POST_PAYLOAD")
HTTP_POST=$(echo "$POST_RES" | tail -n1)
BODY_POST=$(echo "$POST_RES" | head -n-1)

assert_eq "POST new catalog room status code" "201" "$HTTP_POST"
PENTHOUSE_ID=$(echo "$BODY_POST" | jq -r '.id')

# Ensure inventory for new variant on PostgreSQL
docker exec "$PG_CONTAINER" psql -U postgres -d hotel_booking -c \
    "INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms, version)
     SELECT '${PENTHOUSE_ID}', d::date, 2, 2, 0
     FROM generate_series(CURRENT_DATE, CURRENT_DATE + interval '365 days', '1 day'::interval) d
     ON CONFLICT (room_type_id, date) DO NOTHING;" > /dev/null 2>&1 || true

# 3B. Quote langsung berfungsi untuk varian baru
# 2 malam * 4.500.000 = 9.000.000 subtotal, Pajak 10% = 900.000, Total = 9.900.000
QUOTE_3=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${PENTHOUSE_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_3=$(echo "$QUOTE_3" | tail -n1)
BODY_3=$(echo "$QUOTE_3" | head -n-1)

assert_eq "Quote Penthouse baru status code" "200" "$HTTP_3"
SUBTOTAL_3=$(echo "$BODY_3" | jq -r '.pricing.room_subtotal_minor')
TOTAL_3=$(echo "$BODY_3" | jq -r '.pricing.total_price_minor')
assert_eq "Subtotal Penthouse 9.000.000" "9000000" "$SUBTOTAL_3"
assert_eq "Total Penthouse 9.900.000" "9900000" "$TOTAL_3"

# ------------------------------------------------------------------------------
# Test 4: Pricing Guard pada Search & Quote (Pencegahan Kamar Gratis / Unpriced)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Pricing Guard pada Search & Quote (Pencegahan Free Rooms) ---${NC}"

# Buat kamar unpriced di database PostgreSQL (simulasi data korup/tidak bertarif)
UNPRICED_CODE="unpriced-loft-$(date +%s)"
UNPRICED_ID=$(docker exec "$PG_CONTAINER" psql -U postgres -d hotel_booking -t -A -c \
    "INSERT INTO room_types (id, code, name, family_name, bed_type, room_size_sqm, max_capacity, max_adults, max_children, base_price_minor, description, amenities, photos)
     VALUES (gen_random_uuid(), '${UNPRICED_CODE}', 'Unpriced Loft Attic', 'Loft', '1 Queen Bed', 20, 2, 2, 1, 0, 'Unpriced test room', '[]', '[]')
     RETURNING id::text;" | head -n 1)

# Buat inventory untuk unpriced room
docker exec "$PG_CONTAINER" psql -U postgres -d hotel_booking -c \
    "INSERT INTO inventory (room_type_id, date, total_rooms, available_rooms, version)
     SELECT '${UNPRICED_ID}', d::date, 5, 5, 0
     FROM generate_series(CURRENT_DATE, CURRENT_DATE + interval '365 days', '1 day'::interval) d
     ON CONFLICT (room_type_id, date) DO NOTHING;" > /dev/null 2>&1 || true

# 4A. Quote untuk kamar unpriced DITOLAK 400 RATE_UNAVAILABLE
QUOTE_4A=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${UNPRICED_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_4A=$(echo "$QUOTE_4A" | tail -n1)
BODY_4A=$(echo "$QUOTE_4A" | head -n-1)

assert_eq "Quote kamar unpriced status code" "400" "$HTTP_4A"
assert_contains "Quote kamar unpriced error code" "RATE_UNAVAILABLE" "$BODY_4A"

# 4B. Search kamar unpriced ditandai available=false dengan reason=RATE_UNAVAILABLE
SEARCH_4B=$(curl -s "${BASE_URL}/api/v1/search?check_in=${CHECK_IN}&check_out=${CHECK_OUT}&adults=2&rooms=1")
UNPRICED_AVAIL=$(echo "$SEARCH_4B" | jq -r ".results[] | select(.room_variant.id==\"${UNPRICED_ID}\") | .available")
UNPRICED_REASON=$(echo "$SEARCH_4B" | jq -r ".results[] | select(.room_variant.id==\"${UNPRICED_ID}\") | .unavailable_reason")
UNPRICED_PRICE=$(echo "$SEARCH_4B" | jq -r ".results[] | select(.room_variant.id==\"${UNPRICED_ID}\") | .total_price_minor")

assert_eq "Unpriced room tidak available di Search" "false" "$UNPRICED_AVAIL"
assert_eq "Unpriced room reason adalah RATE_UNAVAILABLE" "RATE_UNAVAILABLE" "$UNPRICED_REASON"
assert_eq "Unpriced room total_price_minor adalah 0" "0" "$UNPRICED_PRICE"

# ------------------------------------------------------------------------------
# Test 5: Imutabilitas Quote yang Telah Terkunci (Locked Quote Freeze)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Imutabilitas Quote yang Telah Terkunci (Locked Quote Freeze) ---${NC}"

# Ambil quote untuk Superior King saat harga 700.000 (total = 1.540.000)
QUOTE_5A=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
FROZEN_QUOTE_ID=$(echo "$QUOTE_5A" | jq -r '.quote_id')
FROZEN_PRICE=$(echo "$QUOTE_5A" | jq -r '.pricing.total_price_minor')
assert_eq "Quote beku awal bernilai 1.540.000" "1540000" "$FROZEN_PRICE"

# Manajemen menaikkan lagi harga Superior King ke 900.000
PUT_900K=$(cat <<EOF
{
  "code": "sup-king",
  "name": "Superior King Peak Season",
  "family_name": "Superior",
  "bed_type": "1 King Bed",
  "room_size_sqm": 28,
  "max_capacity": 3,
  "max_adults": 2,
  "max_children": 1,
  "base_price_minor": 900000,
  "description": "Kamar Superior King Peak Season."
}
EOF
)
curl -s -X PUT "${BASE_URL}/api/v1/catalog/rooms/${SUP_KING_ID}" \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    -H "Content-Type: application/json" \
    -d "$PUT_900K" > /dev/null

# Booking menggunakan FROZEN_QUOTE_ID harus tetap membayar sesuai harga quote beku (1.540.000)
BOOK_5=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${FROZEN_QUOTE_ID}\",\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"Tamu Frozen Quote\",\"guest_email\":\"frozen@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_BOOK5=$(echo "$BOOK_5" | tail -n1)
BODY_BOOK5=$(echo "$BOOK_5" | head -n-1)

assert_eq "Booking dengan frozen quote status code" "201" "$HTTP_BOOK5"
BOOKED_TOTAL=$(echo "$BODY_BOOK5" | jq -r '.pricing.total_price_minor // .booking.total_price_minor')
assert_eq "Booking price tetap beku pada 1.540.000 bukan 1.980.000" "1540000" "$BOOKED_TOTAL"

BOOKING_ID=$(echo "$BODY_BOOK5" | jq -r '.booking.id // .id')

# Pembayaran fake pay
FAKE_PAY=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/fake-pay/ref-success?booking_id=${BOOKING_ID}")
HTTP_PAY=$(echo "$FAKE_PAY" | tail -n1)
assert_eq "Fake payment status code" "200" "$HTTP_PAY"

# Verifikasi reservasi terkonfirmasi
GUEST_DETAIL=$(curl -s "${BASE_URL}/api/v1/bookings/${BOOKING_ID}")
CONFIRMED_STATUS=$(echo "$GUEST_DETAIL" | jq -r '.status // .booking.status')
assert_eq "Status booking terkonfirmasi" "confirmed" "$CONFIRMED_STATUS"

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
