#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Router & Middleware Architecture Modernization
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar Pengujian & Cakupan:
# 1. Verification of Probe Endpoints (/healthz, /ready) & Rate-limit Bypass (FR-08)
# 2. Strict RFC 7807 404 NoRoute & 405 NoMethod with Allow Header (FR-09, FR-10)
# 3. Security Headers: X-Request-Id, X-Content-Type-Options: nosniff (FR-05)
# 4. Scoped Idempotency Key Isolation & Length Validation (FR-11, FR-12)
# 5. Occupancy & Stay Horizon Invariants via Unified Guest Parser (FR-13, FR-14)
# 6. Webhook Delegation via booking.Service.ApplyPaymentEvent (FR-15)
# 7. In-Process Go E2E Runner Verification (E2E-69, E2E-70, E2E-71, E2E-72)
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
echo -e "${BLUE}  E2E TEST: ROUTER & MIDDLEWARE ARCHITECTURE REFACTOR                        ${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

# ------------------------------------------------------------------------------
# 1. Run Automated In-Process Go E2E Suite (E2E-69, E2E-70, E2E-71, E2E-72)
# ------------------------------------------------------------------------------
echo -e "${CYAN}1. Menjalankan Go In-Process E2E Test Suite (E2E-69 .. E2E-72)...${NC}"
if (cd "${REPO_ROOT}" && go test -v ./testing/e2e/script -run 'TestEndToEndHotelBookingRBACLifecycle/(E2E-69|E2E-70|E2E-71|E2E-72)'); then
    echo -e "  ${GREEN}✓${NC} Go E2E Suite (E2E-69 to E2E-72) passed successfully"
    TOTAL=$((TOTAL + 4))
    PASSED=$((PASSED + 4))
else
    echo -e "  ${RED}✗${NC} Go E2E Suite failed"
    TOTAL=$((TOTAL + 4))
    FAILED=$((FAILED + 4))
fi

# ------------------------------------------------------------------------------
# 2. Boot Ephemeral Server or Use Provided BASE_URL
# ------------------------------------------------------------------------------
TMP_DIR="$(mktemp -d /tmp/e2e_router_XXXXXX)"
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
    EPHEMERAL_PORT=28089
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
# 3. Test Probe Endpoints & Security Headers
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}3. Menguji probe /healthz dan /ready beserta header keamanan...${NC}"
RESP_HEADERS=$(curl -sS -I "${BASE_URL}/healthz")
assert_contains "Header X-Request-Id diinjeksi pada /healthz" "x-request-id" "$RESP_HEADERS"
assert_contains "Header X-Content-Type-Options: nosniff diinjeksi" "x-content-type-options: nosniff" "$RESP_HEADERS"

READY_RESP=$(curl -sS "${BASE_URL}/ready")
assert_contains "Probe /ready mengembalikan status ready" '"status":"ready"' "$READY_RESP"

# ------------------------------------------------------------------------------
# 4. Test RFC 7807 NoRoute (404)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}4. Menguji RFC 7807 NoRoute (404)...${NC}"
STATUS_404=$(curl -s -o "$TMP_DIR/resp404.json" -w "%{http_code}" "${BASE_URL}/api/v1/non-existent-route-for-testing")
assert_eq "Status code untuk rute tidak dikenal adalah 404" "404" "$STATUS_404"
BODY_404=$(cat "$TMP_DIR/resp404.json")
assert_contains "Kode error NOT_FOUND" "NOT_FOUND" "$BODY_404"
assert_contains "Status 404 pada problem details" '"status":404' "$BODY_404"

# ------------------------------------------------------------------------------
# 5. Test RFC 7807 NoMethod (405) & Allow Header
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}5. Menguji RFC 7807 NoMethod (405) & Allow Header...${NC}"
STATUS_405=$(curl -s -o "$TMP_DIR/resp405.json" -D "$TMP_DIR/headers405.txt" -w "%{http_code}" -X DELETE "${BASE_URL}/api/v1/search")
assert_eq "Status code untuk method tidak diizinkan adalah 405" "405" "$STATUS_405"
BODY_405=$(cat "$TMP_DIR/resp405.json")
assert_contains "Kode error METHOD_NOT_ALLOWED" "METHOD_NOT_ALLOWED" "$BODY_405"
HDR_405=$(cat "$TMP_DIR/headers405.txt")
assert_contains "Header Allow hadir pada respon 405" "Allow: GET" "$HDR_405"

# ------------------------------------------------------------------------------
# 6. Test Idempotency Key Header Validation (> 64 Chars)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}6. Menguji validasi panjang Idempotency-Key (> 64 chars)...${NC}"
LONG_KEY="012345678901234567890123456789012345678901234567890123456789012345"
STATUS_IDEM=$(curl -s -o "$TMP_DIR/respidem.json" -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${LONG_KEY}" \
    -d '{"room_type_id":"01900000-0000-7000-8000-000000000001"}')
assert_eq "Status code untuk Idempotency-Key > 64 chars adalah 400" "400" "$STATUS_IDEM"
BODY_IDEM=$(cat "$TMP_DIR/respidem.json")
assert_contains "Kode error INVALID_IDEMPOTENCY_KEY" "INVALID_IDEMPOTENCY_KEY" "$BODY_IDEM"

# ------------------------------------------------------------------------------
# 7. Test Occupancy & Stay Horizon Invariants (Unified Guest Parser)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}7. Menguji parser query tamu: batas usia anak > 17 tahun...${NC}"
STATUS_OCC=$(curl -s -o "$TMP_DIR/respocc.json" -w "%{http_code}" \
    "${BASE_URL}/api/v1/search?check_in=2026-11-01&check_out=2026-11-03&adults=2&child_ages=18")
assert_eq "Status code untuk usia anak > 17 adalah 400" "400" "$STATUS_OCC"
BODY_OCC=$(cat "$TMP_DIR/respocc.json")
assert_contains "Kode error INVALID_CHILD_AGE" "INVALID_CHILD_AGE" "$BODY_OCC"

echo -e "\n${CYAN}8. Menguji parser query tamu: batas durasi inap > 30 malam...${NC}"
STATUS_STAY=$(curl -s -o "$TMP_DIR/respstay.json" -w "%{http_code}" \
    "${BASE_URL}/api/v1/search?check_in=2026-11-01&check_out=2026-12-10&adults=2")
assert_eq "Status code untuk durasi inap > 30 malam adalah 400" "400" "$STATUS_STAY"
BODY_STAY=$(cat "$TMP_DIR/respstay.json")
assert_contains "Kode error EXCEEDS_MAX_LOS" "EXCEEDS_MAX_LOS" "$BODY_STAY"

# ------------------------------------------------------------------------------
# 9. Test Webhook Authentication Rejection
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}9. Menguji proteksi Webhook Xendit tanpa token sah...${NC}"
STATUS_WH=$(curl -s -o "$TMP_DIR/respwh.json" -w "%{http_code}" -X POST "${BASE_URL}/api/v1/webhooks/xendit" \
    -H "Content-Type: application/json" \
    -d '{"id":"test-123"}')
assert_eq "Status code webhook tanpa callback token adalah 401" "401" "$STATUS_WH"
BODY_WH=$(cat "$TMP_DIR/respwh.json")
assert_contains "Kode error UNAUTHORIZED" "UNAUTHORIZED" "$BODY_WH"

# ------------------------------------------------------------------------------
# Ringkasan Eksekusi
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "TOTAL TESTS : ${TOTAL}"
echo -e "PASSED      : ${GREEN}${PASSED}${NC}"
echo -e "FAILED      : ${RED}${FAILED}${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}E2E Test Suites FAILED!${NC}"
    exit 1
else
    echo -e "${GREEN}ALL E2E Test Suites PASSED!${NC}"
    exit 0
fi
