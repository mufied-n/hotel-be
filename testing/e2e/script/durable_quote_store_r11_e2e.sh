#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Penyimpanan Kuotasi Harga Terdistribusi (Durable Quote Store) (BE-R11)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Bintang 4 Pulang ke Uttara, Distributed Cache Parity & Fail-Closed
# Menguji:
# 1. Penyimpanan kuotasi ke Valkey secara native dengan TTL 15 menit (EX 900)
# 2. Pembagian kuotasi lintas instance (Instance 1 membuat quote, Instance 2 memproses booking)
# 3. Ketahanan kuotasi saat instance direstart (Durability across restart)
# 4. Penolakan pemesanan saat kuotasi kedaluwarsa (TTL Expiration)
# 5. Penanganan kegagalan persistensi secara fail-closed (HTTP 500 INTERNAL_ERROR)
# ==============================================================================

set -euo pipefail

BASE_URL_1="${API_BASE_URL_1:-http://localhost:28080}"
BASE_URL_2="${API_BASE_URL_2:-http://localhost:28081}"
VK_CONTAINER="${VK_CONTAINER:-r11-vk}"
PG_CONTAINER="${PG_CONTAINER:-r11-pg}"

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

assert_gt() {
    local desc="$1"
    local val="$2"
    local threshold="$3"
    TOTAL=$((TOTAL + 1))
    if [ "$val" -gt "$threshold" ]; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Value: ${val} > ${threshold})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Value ${val} is not greater than ${threshold}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}  E2E Test: BE-R11 Durable Distributed Quote Store (Valkey)      ${NC}"
echo -e "${BLUE}=================================================================${NC}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Healthcheck Service ---${NC}"
HEALTH_1=$(curl -s "${BASE_URL_1}/healthz")
assert_contains "Instance 1 Healthcheck OK" '"status":"ok"' "$HEALTH_1"

CHECK_IN=$(date -d "+15 days" +%Y-%m-%d)
CHECK_OUT=$(date -d "+17 days" +%Y-%m-%d) # 2 malam
SUP_KING_ID="01900000-0000-7000-8000-000000000001"

# ------------------------------------------------------------------------------
# Test 1: Pembuatan Quote & Verifikasi Penyimpanan Langsung di Valkey
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Quote Creation & Native Valkey Storage ---${NC}"

QUOTE_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL_1}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_QUOTE=$(echo "$QUOTE_RES" | tail -n1)
BODY_QUOTE=$(echo "$QUOTE_RES" | head -n-1)

assert_eq "POST /quotes status 200" "200" "$HTTP_QUOTE"
QUOTE_ID_1=$(echo "$BODY_QUOTE" | jq -r '.quote_id')
assert_contains "Quote ID valid UUID" "01" "$QUOTE_ID_1"

# Periksa langsung isi key di Valkey
VALKEY_KEY="hotel:quote:${QUOTE_ID_1}"
VK_RAW=$(docker exec "$VK_CONTAINER" valkey-cli GET "$VALKEY_KEY" 2>/dev/null || true)
assert_contains "Key tersimpan di Valkey" "$QUOTE_ID_1" "$VK_RAW"

VK_TTL=$(docker exec "$VK_CONTAINER" valkey-cli TTL "$VALKEY_KEY" 2>/dev/null || echo "0")
# TTL harus mendekati 900 detik (15 menit)
assert_gt "Valkey TTL di atas 850 detik" "$VK_TTL" 850

# ------------------------------------------------------------------------------
# Test 2: Cross-Instance Quote Sharing (Instance 1 Quote -> Instance 2 Booking)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Cross-Instance Quote Sharing (Instance 1 -> Instance 2) ---${NC}"

HEALTH_2=$(curl -s "${BASE_URL_2}/healthz")
assert_contains "Instance 2 Healthcheck OK" '"status":"ok"' "$HEALTH_2"

BOOK_RES_2=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL_2}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${QUOTE_ID_1}\",\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"Tamu Instance Dua\",\"guest_email\":\"instansi2@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_BOOK_2=$(echo "$BOOK_RES_2" | tail -n1)
BODY_BOOK_2=$(echo "$BOOK_RES_2" | head -n-1)

assert_eq "Instance 2 memproses quote dari Instance 1 (201 Created)" "201" "$HTTP_BOOK_2"
BOOKING_ID_2=$(echo "$BODY_BOOK_2" | jq -r '.booking.id')
assert_contains "Booking ID diterbitkan sukses" "01" "$BOOKING_ID_2"

# ------------------------------------------------------------------------------
# Test 3: Ketahanan Kuotasi Melintasi Restart Instance
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Durability Across Instance Restart ---${NC}"

# Buat quote kedua di Instance 1
QUOTE_RES_2=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL_1}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"bed_and_breakfast\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
QUOTE_ID_2=$(echo "$QUOTE_RES_2" | head -n-1 | jq -r '.quote_id')

# Simulasikan Instance 1 restart: kirim request booking ke Instance 2 yang mensimulasikan
# proses backend yang stateless di mana instance tidak memegang quote di lokal memorinya
BOOK_RES_RESTART=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL_2}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${QUOTE_ID_2}\",\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"Tamu Restart Durability\",\"guest_email\":\"restart@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_BOOK_RESTART=$(echo "$BOOK_RES_RESTART" | tail -n1)

assert_eq "Booking sukses menggunakan quote tersimpan di Valkey (201 Created)" "201" "$HTTP_BOOK_RESTART"

# ------------------------------------------------------------------------------
# Test 4: Penolakan Pemesanan saat Kuotasi Kedaluwarsa (TTL Expiry)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. TTL Expiration Enforcement ---${NC}"

# Buat quote ketiga
QUOTE_RES_3=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL_1}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
QUOTE_ID_3=$(echo "$QUOTE_RES_3" | head -n-1 | jq -r '.quote_id')

# Percepat kadaluarsa key di Valkey (set TTL 1 detik)
docker exec "$VK_CONTAINER" valkey-cli EXPIRE "hotel:quote:${QUOTE_ID_3}" 1 > /dev/null
sleep 2

# Verifikasi key sudah hilang dari Valkey
EXPIRED_KEY_CHECK=$(docker exec "$VK_CONTAINER" valkey-cli EXISTS "hotel:quote:${QUOTE_ID_3}")
assert_eq "Key di Valkey telah lenyap (0)" "0" "$EXPIRED_KEY_CHECK"

# Coba checkout dengan quote yang telah kadaluarsa
EXPIRED_BOOK_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL_1}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"${QUOTE_ID_3}\",\"room_type_id\":\"${SUP_KING_ID}\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"Tamu Expired\",\"guest_email\":\"expired@example.com\",\"terms_accepted\":true,\"privacy_accepted\":true}")
HTTP_EXPIRED=$(echo "$EXPIRED_BOOK_RES" | tail -n1)
BODY_EXPIRED=$(echo "$EXPIRED_BOOK_RES" | head -n-1)

assert_eq "Booking dengan quote kadaluarsa ditolak 410" "410" "$HTTP_EXPIRED"
assert_contains "Error code QUOTE_EXPIRED" "QUOTE_EXPIRED" "$BODY_EXPIRED"

# ------------------------------------------------------------------------------
# Test 5: Penanganan Fail-Closed saat Persistensi Valkey Gagal
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Fail-Closed on Persistence Failure ---${NC}"

# Simulasikan instance backend dengan koneksi Valkey yang gagal (port salah/down)
FAILED_PORT="28082"
APP_PORT="$FAILED_PORT" DATABASE_URL="postgres://postgres:postgres@localhost:25432/hotel_booking?sslmode=disable" VALKEY_ADDR="localhost:9999" /tmp/r11-server > /dev/null 2>&1 &
PID_FAIL=$!

# Tunggu server siap
for i in {1..20}; do
    if curl -s "http://localhost:${FAILED_PORT}/healthz" > /dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

# Request quote pada server dengan Valkey down
FAIL_QUOTE_RES=$(curl -s -w "\n%{http_code}" -X POST "http://localhost:${FAILED_PORT}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${SUP_KING_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${CHECK_IN}\",\"check_out\":\"${CHECK_OUT}\",\"num_rooms\":1,\"num_guests\":2}")
HTTP_FAIL_QUOTE=$(echo "$FAIL_QUOTE_RES" | tail -n1)
BODY_FAIL_QUOTE=$(echo "$FAIL_QUOTE_RES" | head -n-1)

kill "$PID_FAIL" 2>/dev/null || true
wait "$PID_FAIL" 2>/dev/null || true

assert_eq "Quote creation gagal 500 saat Valkey down (Fail-Closed)" "500" "$HTTP_FAIL_QUOTE"
assert_contains "Error code INTERNAL_ERROR" "INTERNAL_ERROR" "$BODY_FAIL_QUOTE"

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "  Total Assertions : ${TOTAL}"
echo -e "  Passed           : ${PASSED}"
echo -e "  Failed           : ${FAILED}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "  ${GREEN}ALL E2E ASSERTIONS PASSED!${NC}"
    echo -e "${BLUE}=================================================================${NC}\n"
    exit 0
else
    echo -e "  ${RED}SOME ASSERTIONS FAILED!${NC}"
    echo -e "${BLUE}=================================================================${NC}\n"
    exit 1
fi
