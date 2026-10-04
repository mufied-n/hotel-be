#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Candidate A - Dynamic Rates, Room Allotment & Stop-Sell Engine
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Skenario Pengujian:
# 1. Verification of Go In-Process E2E Suite (E2E-73 & E2E-74)
# 2. Revenue Manager Calendar Inspection & Date Range Limits (FR-08)
# 3. Revenue Manager Bulk Stop-Sell Override Configuration (FR-08)
# 4. Guest Public Search Availability Stop-Sell Protection (FR-01)
# 5. Guest Quote Calculation Rejection on Stop-Sold Dates (FR-01)
# 6. Minimum Length of Stay (MinLOS) Restriction Enforcement (FR-08)
# 7. Receptionist Calendar View Access & Multi-Role RBAC (FR-08)
# 8. Unauthorized Role Rejection on Calendar Modifications (FR-08)
# 9. Dynamic Promo Campaign Creation with Stay Constraints (FR-09)
# 10. Promo Code Validation & Minimum Stay Enforcement (FR-09)
# 11. Successful Dynamic Promo Discount Application (FR-09)
# 12. Promo Deactivation & Expiry Enforcement (FR-09)
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

TOTAL=0
PASSED=0
FAILED=0

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
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
        echo -e "  ${RED}✗${NC} ${desc} — Expected: '${expected}', Got: '${actual}'"
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
        echo -e "  ${RED}✗${NC} ${desc} — Needle '${needle}' not found in:\n${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  E2E TEST: DYNAMIC RATES, ROOM ALLOTMENT & STOP-SELL ENGINE (CANDIDATE A)   ${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

# ------------------------------------------------------------------------------
# 1. Run Automated In-Process Go E2E Suite (E2E-73 & E2E-74)
# ------------------------------------------------------------------------------
echo -e "${CYAN}1. Menjalankan Go In-Process E2E Test Suite (E2E-73 & E2E-74)...${NC}"
if (cd "${REPO_ROOT}" && go test -v ./testing/e2e/script -run 'TestEndToEndHotelBookingRBACLifecycle/(E2E-73|E2E-74)'); then
    echo -e "  ${GREEN}✓${NC} Go E2E Suite (E2E-73 & E2E-74) passed successfully"
    TOTAL=$((TOTAL + 2))
    PASSED=$((PASSED + 2))
else
    echo -e "  ${RED}✗${NC} Go E2E Suite failed"
    TOTAL=$((TOTAL + 2))
    FAILED=$((FAILED + 2))
fi

# ------------------------------------------------------------------------------
# 2. Boot Ephemeral Server or Use Provided BASE_URL
# ------------------------------------------------------------------------------
TMP_DIR="$(mktemp -d /tmp/e2e_dynrates_XXXXXX)"
EPHEMERAL_PID=""

cleanup() {
    if [ -n "$EPHEMERAL_PID" ]; then
        kill -TERM "$EPHEMERAL_PID" 2>/dev/null || true
    fi
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

BASE_URL="${API_BASE_URL:-}"
if [ -z "${BASE_URL}" ]; then
    EPHEMERAL_PORT=28091
    BASE_URL="http://127.0.0.1:${EPHEMERAL_PORT}"
    echo -e "\n${CYAN}2. Memulai Ephemeral Live Server pada port ${EPHEMERAL_PORT}...${NC}"
    (cd "${REPO_ROOT}" && RUN_EPHEMERAL_E2E_SERVER_PORT="${EPHEMERAL_PORT}" go test -v ./testing/e2e/script -run TestEphemeralServerRunner > "$TMP_DIR/ephemeral.log" 2>&1) &
    EPHEMERAL_PID=$!

    # Wait for server ready
    READY=0
    for _ in $(seq 1 40); do
        if curl -s -f "${BASE_URL}/healthz" > /dev/null 2>&1; then
            READY=1
            break
        fi
        sleep 0.1
    done

    if [ "$READY" -ne 1 ]; then
        echo -e "  ${RED}✗${NC} Gagal menyalakan Ephemeral Live Server:"
        cat "$TMP_DIR/ephemeral.log"
        exit 1
    fi
    echo -e "  ${GREEN}✓${NC} Ephemeral Live Server siap menerima traffic di ${BASE_URL}"
else
    echo -e "\n${CYAN}2. Menggunakan external API_BASE_URL=${BASE_URL}${NC}"
fi

# ------------------------------------------------------------------------------
# 3. Revenue Manager Calendar Inspection & Date Range Limits
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}3. Pengujian Query Kalender Tarif (Revenue Manager & RBAC)...${NC}"

# Query tanpa rentang tanggal wajib gagal 400
RESP_MISSING=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer revenue_mgr" "${BASE_URL}/api/v1/revenue/calendar")
HTTP_CODE=$(echo "$RESP_MISSING" | tail -n 1)
BODY=$(echo "$RESP_MISSING" | head -n -1)
assert_eq "Query tanpa tanggal mengembalikan 400" "400" "$HTTP_CODE"
assert_contains "Error code MISSING_DATE_RANGE" "MISSING_DATE_RANGE" "$BODY"

# Query dengan rentang tanggal valid (10 hari)
START_DATE="2026-10-10"
END_DATE="2026-10-15"
RESP_CAL=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer revenue_mgr" "${BASE_URL}/api/v1/revenue/calendar?start_date=${START_DATE}&end_date=${END_DATE}")
HTTP_CODE=$(echo "$RESP_CAL" | tail -n 1)
BODY=$(echo "$RESP_CAL" | head -n -1)
assert_eq "Query kalender valid mengembalikan 200 OK" "200" "$HTTP_CODE"
assert_contains "Respons memuat start_date" "start_date" "$BODY"

# Receptionist boleh mengakses kalender (read-only)
RESP_RECEPT=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer receptionist" "${BASE_URL}/api/v1/revenue/calendar?start_date=${START_DATE}&end_date=${END_DATE}")
HTTP_CODE=$(echo "$RESP_RECEPT" | tail -n 1)
assert_eq "Receptionist diizinkan melihat kalender (200 OK)" "200" "$HTTP_CODE"

# Guest/unauthorized role ditolak 403
RESP_GUEST=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/revenue/calendar?start_date=${START_DATE}&end_date=${END_DATE}")
HTTP_CODE=$(echo "$RESP_GUEST" | tail -n 1)
assert_eq "Guest tanpa auth ditolak 403 Forbidden" "403" "$HTTP_CODE"

# ------------------------------------------------------------------------------
# 4. Bulk Stop-Sell Override & Public Search Protection
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}4. Pengujian Konfigurasi Stop-Sell & Proteksi Pencarian Publik...${NC}"

ROOM_TYPE_ID="01900000-0000-7000-8000-000000000001"
STOP_DATE="2026-10-20"
NEXT_DATE="2026-10-21"

# Revenue Manager menerapkan stop-sell pada tanggal 2026-10-20
STOP_SELL_PAYLOAD=$(cat <<EOF
{
  "room_type_ids": ["${ROOM_TYPE_ID}"],
  "start_date": "${STOP_DATE}",
  "end_date": "${STOP_DATE}",
  "rate_plan_code": "RO",
  "is_stop_sell": true
}
EOF
)

RESP_BULK=$(curl -s -w "\n%{http_code}" -X PUT \
    -H "Authorization: Bearer revenue_mgr" \
    -H "Content-Type: application/json" \
    -d "${STOP_SELL_PAYLOAD}" \
    "${BASE_URL}/api/v1/revenue/calendar/bulk")
HTTP_CODE=$(echo "$RESP_BULK" | tail -n 1)
BODY=$(echo "$RESP_BULK" | head -n -1)
assert_eq "Bulk update stop-sell berhasil (200 OK)" "200" "$HTTP_CODE"
assert_contains "Status success pada respons" "success" "$BODY"

# Guest mencari kamar pada tanggal stop-sell -> wajib muncul status available=false dan reason=STOP_SELL
RESP_SEARCH=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=${STOP_DATE}&check_out=${NEXT_DATE}&rooms=1&adults=1")
HTTP_CODE=$(echo "$RESP_SEARCH" | tail -n 1)
BODY=$(echo "$RESP_SEARCH" | head -n -1)
assert_eq "Pencarian publik mengembalikan 200 OK" "200" "$HTTP_CODE"
assert_contains "Room ditandai STOP_SELL pada hasil pencarian" "STOP_SELL" "$BODY"

# Guest mencoba membuat kuotasi harga pada tanggal stop-sell -> wajib ditolak 400 ROOM_STOP_SELL
QUOTE_STOP_PAYLOAD=$(cat <<EOF
{
  "room_type_id": "${ROOM_TYPE_ID}",
  "check_in": "${STOP_DATE}",
  "check_out": "${NEXT_DATE}",
  "num_rooms": 1,
  "num_guests": 1
}
EOF
)
RESP_QUOTE_STOP=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "${QUOTE_STOP_PAYLOAD}" \
    "${BASE_URL}/api/v1/quotes")
HTTP_CODE=$(echo "$RESP_QUOTE_STOP" | tail -n 1)
BODY=$(echo "$RESP_QUOTE_STOP" | head -n -1)
assert_eq "Quote pada tanggal stop-sell ditolak (400 Bad Request)" "400" "$HTTP_CODE"
assert_contains "Error code ROOM_STOP_SELL" "ROOM_STOP_SELL" "$BODY"

# ------------------------------------------------------------------------------
# 5. Minimum Length of Stay (MinLOS) Enforcement
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}5. Pengujian Restriksi Minimum Length of Stay (MinLOS)...${NC}"

MIN_LOS_DATE="2026-10-25"
MIN_LOS_END="2026-10-27"
MIN_LOS_PAYLOAD=$(cat <<EOF
{
  "room_type_ids": ["${ROOM_TYPE_ID}"],
  "start_date": "${MIN_LOS_DATE}",
  "end_date": "${MIN_LOS_END}",
  "rate_plan_code": "RO",
  "min_los": 3
}
EOF
)

RESP_MIN_LOS=$(curl -s -w "\n%{http_code}" -X PUT \
    -H "Authorization: Bearer revenue_mgr" \
    -H "Content-Type: application/json" \
    -d "${MIN_LOS_PAYLOAD}" \
    "${BASE_URL}/api/v1/revenue/calendar/bulk")
HTTP_CODE=$(echo "$RESP_MIN_LOS" | tail -n 1)
assert_eq "Bulk update MinLOS=3 berhasil (200 OK)" "200" "$HTTP_CODE"

# Guest mencoba kuotasi 1 malam pada tanggal MinLOS=3 -> wajib ditolak 400 MIN_LENGTH_OF_STAY_VIOLATED
QUOTE_1NIGHT_PAYLOAD=$(cat <<EOF
{
  "room_type_id": "${ROOM_TYPE_ID}",
  "check_in": "${MIN_LOS_DATE}",
  "check_out": "2026-10-26",
  "num_rooms": 1,
  "num_guests": 1
}
EOF
)
RESP_QUOTE_LOS=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "${QUOTE_1NIGHT_PAYLOAD}" \
    "${BASE_URL}/api/v1/quotes")
HTTP_CODE=$(echo "$RESP_QUOTE_LOS" | tail -n 1)
BODY=$(echo "$RESP_QUOTE_LOS" | head -n -1)
assert_eq "Quote kurang dari MinLOS ditolak (400 Bad Request)" "400" "$HTTP_CODE"
assert_contains "Error code MIN_LENGTH_OF_STAY_VIOLATED" "MIN_LENGTH_OF_STAY_VIOLATED" "$BODY"

# ------------------------------------------------------------------------------
# 6. Dynamic Promo Campaigns Lifecycle & Validation
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}6. Pengujian Lifecycle Kampanye Promo Dinamis...${NC}"

PROMO_CODE="E2ESPECIAL20"
PROMO_PAYLOAD=$(cat <<EOF
{
  "code": "${PROMO_CODE}",
  "name": "E2E Special Discount 20%",
  "discount_type": "PERCENT",
  "discount_value": 20,
  "max_discount_idr": 400000,
  "min_stay_nights": 2,
  "quota_total": 10,
  "valid_from": "2026-10-01T00:00:00Z",
  "valid_to": "2026-11-01T23:59:59Z",
  "is_active": true
}
EOF
)

RESP_PROMO_CREATE=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Authorization: Bearer revenue_mgr" \
    -H "Content-Type: application/json" \
    -d "${PROMO_PAYLOAD}" \
    "${BASE_URL}/api/v1/revenue/promos")
HTTP_CODE=$(echo "$RESP_PROMO_CREATE" | tail -n 1)
BODY=$(echo "$RESP_PROMO_CREATE" | head -n -1)
assert_eq "Pembuatan promo baru berhasil (201 Created)" "201" "$HTTP_CODE"
assert_contains "Respons memuat kode promo" "${PROMO_CODE}" "$BODY"

PROMO_ID=$(echo "$BODY" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')

# Guest mencoba memakai promo dengan menginap 1 malam (syarat min 2 malam) -> ditolak 400 PROMO_MIN_STAY_VIOLATED
QUOTE_PROMO_1N=$(cat <<EOF
{
  "room_type_id": "${ROOM_TYPE_ID}",
  "check_in": "2026-10-10",
  "check_out": "2026-10-11",
  "num_rooms": 1,
  "num_guests": 1,
  "promo_code": "${PROMO_CODE}"
}
EOF
)
RESP_PROMO_1N=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "${QUOTE_PROMO_1N}" \
    "${BASE_URL}/api/v1/quotes")
HTTP_CODE=$(echo "$RESP_PROMO_1N" | tail -n 1)
BODY=$(echo "$RESP_PROMO_1N" | head -n -1)
assert_eq "Promo dengan masa inap kurang dari min_stay ditolak (400)" "400" "$HTTP_CODE"
assert_contains "Error code PROMO_MIN_STAY_VIOLATED" "PROMO_MIN_STAY_VIOLATED" "$BODY"

# Guest mencoba memakai promo dengan menginap 2 malam -> berhasil 200 OK
QUOTE_PROMO_2N=$(cat <<EOF
{
  "room_type_id": "${ROOM_TYPE_ID}",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "num_rooms": 1,
  "num_guests": 1,
  "promo_code": "${PROMO_CODE}"
}
EOF
)
RESP_PROMO_2N=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "${QUOTE_PROMO_2N}" \
    "${BASE_URL}/api/v1/quotes")
HTTP_CODE=$(echo "$RESP_PROMO_2N" | tail -n 1)
BODY=$(echo "$RESP_PROMO_2N" | head -n -1)
assert_eq "Promo 2 malam berhasil dihitung kuotasi (200 OK)" "200" "$HTTP_CODE"
assert_contains "Respons memuat detail promo terkunci" "discount_minor" "$BODY"

# Revenue Manager menonaktifkan kampanye promo
DEACTIVATE_PAYLOAD='{"is_active": false}'
RESP_DEACT=$(curl -s -w "\n%{http_code}" -X PUT \
    -H "Authorization: Bearer revenue_mgr" \
    -H "Content-Type: application/json" \
    -d "${DEACTIVATE_PAYLOAD}" \
    "${BASE_URL}/api/v1/revenue/promos/${PROMO_ID}")
HTTP_CODE=$(echo "$RESP_DEACT" | tail -n 1)
assert_eq "Deaktivasi promo berhasil (200 OK)" "200" "$HTTP_CODE"

# Guest mencoba memakai promo yang telah dinonaktifkan -> ditolak 400 PROMO_EXPIRED
RESP_PROMO_DEACT=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "${QUOTE_PROMO_2N}" \
    "${BASE_URL}/api/v1/quotes")
HTTP_CODE=$(echo "$RESP_PROMO_DEACT" | tail -n 1)
BODY=$(echo "$RESP_PROMO_DEACT" | head -n -1)
assert_eq "Promo yang dinonaktifkan ditolak (400 Bad Request)" "400" "$HTTP_CODE"
assert_contains "Error code PROMO_EXPIRED" "PROMO_EXPIRED" "$BODY"

# ------------------------------------------------------------------------------
# Hasil Akhir Ringkasan Pengujian
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  RINGKASAN HASIL PENGUJIAN E2E DYNAMIC RATES & PROMOS                        ${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo -e "Total Pengujian : ${TOTAL}"
echo -e "Lulus (Passed)  : ${GREEN}${PASSED}${NC}"
echo -e "Gagal (Failed)  : ${RED}${FAILED}${NC}"

if [ "${FAILED}" -eq 0 ]; then
    echo -e "\n${GREEN}>>> SELURUH PENGUJIAN E2E CANDIDATE A BERHASIL DENGAN STATUS 100% PASS <<<${NC}\n"
    exit 0
else
    echo -e "\n${RED}>>> BEBERAPA PENGUJIAN E2E CANDIDATE A GAGAL <<<${NC}\n"
    exit 1
fi
