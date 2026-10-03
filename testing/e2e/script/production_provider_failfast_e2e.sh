#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Production Provider Fail-Fast & Readiness Safety (BE-R16)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: PCI-DSS v4.0 Requirement 6.4, UU PDP No. 27/2022, Twelve-Factor Config.
# Pengujian:
# 1. Fail-fast startup pada APP_ENV=production tanpa provider credentials.
# 2. Transparansi kapabilitas pada GET /ready (environment, payment_gateway, notifier).
# 3. Isolasi mutlak /fake-pay (404 Not Found di production).
# 4. Hardening /fake-pay pada dev (400 INVALID_BOOKING_ID pada non-UUID, no SQL leak).
# 5. Konfirmasi sah fake-pay pada booking hold valid.
# ==============================================================================

set -euo pipefail

DEV_BASE_URL="${BASE_URL:-http://localhost:28080}"
PROD_PORT="${PROD_TEST_PORT:-28089}"
PROD_BASE_URL="http://localhost:${PROD_PORT}"

SERVER_BIN="/tmp/audit-server"
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
echo -e "${BLUE}   E2E Test: BE-R16 Production Provider Fail-Fast & Safety       ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# Pastikan binary server terkompilasi
go build -o "${SERVER_BIN}" ./cmd/server

# ------------------------------------------------------------------------------
# Test 1: Production Startup Fails Fast when Missing Keys
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 1. Production Startup Fail-Fast (Missing Keys) <<${NC}"

# 1a. Missing Xendit Key
OUT_1A=$(APP_ENV=production APP_PORT=28999 DATABASE_URL='postgres://postgres:dev@localhost:25432/booking_test?sslmode=disable' VALKEY_ADDR='localhost:26379' "${SERVER_BIN}" 2>&1 || true)
assert_contains "Production without XENDIT_SECRET_KEY fails fast" "config: XENDIT_SECRET_KEY is required in production" "$OUT_1A"

# 1b. Missing Xendit Webhook Token
OUT_1B=$(APP_ENV=production XENDIT_SECRET_KEY='xnd_dummy' APP_PORT=28999 DATABASE_URL='postgres://postgres:dev@localhost:25432/booking_test?sslmode=disable' VALKEY_ADDR='localhost:26379' "${SERVER_BIN}" 2>&1 || true)
assert_contains "Production without XENDIT_WEBHOOK_TOKEN fails fast" "config: XENDIT_WEBHOOK_TOKEN is required in production" "$OUT_1B"

# 1c. Missing Resend Key
OUT_1C=$(APP_ENV=production XENDIT_SECRET_KEY='xnd_dummy' XENDIT_WEBHOOK_TOKEN='wh_dummy' APP_PORT=28999 DATABASE_URL='postgres://postgres:dev@localhost:25432/booking_test?sslmode=disable' VALKEY_ADDR='localhost:26379' "${SERVER_BIN}" 2>&1 || true)
assert_contains "Production without RESEND_API_KEY fails fast" "config: RESEND_API_KEY is required in production" "$OUT_1C"

# ------------------------------------------------------------------------------
# Test 2: Production Boots Cleanly with Valid Provider Keys
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 2. Production Mode Boots & Isolates Providers <<${NC}"

# Jalankan instance production sementara di port PROD_PORT
APP_ENV=production \
APP_PORT="${PROD_PORT}" \
DATABASE_URL='postgres://postgres:dev@localhost:25432/booking_test?sslmode=disable' \
VALKEY_ADDR='localhost:26379' \
XENDIT_SECRET_KEY='xnd_live_mock_key' \
XENDIT_WEBHOOK_TOKEN='wh_live_mock_token' \
RESEND_API_KEY='re_live_mock_key' \
"${SERVER_BIN}" > /tmp/prod-server.log 2>&1 &
PROD_PID=$!

cleanup_prod() {
    kill "$PROD_PID" 2>/dev/null || true
    wait "$PROD_PID" 2>/dev/null || true
}
trap cleanup_prod EXIT

sleep 2

# 2a. GET /ready in Production
READY_PROD_CODE=$(curl -s -o /tmp/prod_ready.json -w "%{http_code}" "${PROD_BASE_URL}/ready")
assert_eq "Production /ready returns 200 OK" "200" "$READY_PROD_CODE"
assert_contains "Production /ready environment is production" '"environment":"production"' "$(cat /tmp/prod_ready.json)"
assert_contains "Production /ready payment_gateway is xendit" '"payment_gateway":"xendit"' "$(cat /tmp/prod_ready.json)"
assert_contains "Production /ready notifier is resend" '"notifier":"resend"' "$(cat /tmp/prod_ready.json)"

# 2b. /fake-pay in Production returns 404 Not Found
FAKEPAY_PROD_CODE=$(curl -s -o /tmp/prod_fakepay.json -w "%{http_code}" -X POST "${PROD_BASE_URL}/fake-pay/ref-hack?booking_id=01900000-0000-7000-8000-000000000001")
assert_eq "Production strictly forbids /fake-pay with 404" "404" "$FAKEPAY_PROD_CODE"

# Matikan server production uji
cleanup_prod
trap - EXIT

# ------------------------------------------------------------------------------
# Test 3: Development Server Capabilities & Hardened FakePay
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 3. Development Server Readiness & Capability Exposure <<${NC}"

READY_DEV_CODE=$(curl -s -o /tmp/dev_ready.json -w "%{http_code}" "${DEV_BASE_URL}/ready")
assert_eq "Development /ready returns 200 OK" "200" "$READY_DEV_CODE"
assert_contains "Development /ready environment is development" '"environment":"development"' "$(cat /tmp/dev_ready.json)"
assert_contains "Development /ready payment_gateway is fake" '"payment_gateway":"fake"' "$(cat /tmp/dev_ready.json)"
assert_contains "Development /ready notifier is log" '"notifier":"log"' "$(cat /tmp/dev_ready.json)"

echo -e "\n${YELLOW}>> 4. Dev FakePay Hardening: UUID Validation & Sanitization <<${NC}"

# 4a. Non-UUID booking_id parameter returns 400 Bad Request
NON_UUID_CODE=$(curl -s -o /tmp/dev_fp_non_uuid.json -w "%{http_code}" -X POST "${DEV_BASE_URL}/fake-pay/ref-test?booking_id=not-a-valid-uuid-injection")
assert_eq "FakePay rejects non-UUID with 400 Bad Request" "400" "$NON_UUID_CODE"
assert_contains "FakePay returns INVALID_BOOKING_ID code" '"code":"INVALID_BOOKING_ID"' "$(cat /tmp/dev_fp_non_uuid.json)"
# Pastikan tidak ada teks internal SQL bocor
if grep -qi "SQLSTATE\|syntax error\|invalid input syntax for type uuid" /tmp/dev_fp_non_uuid.json; then
    echo -e "  ${RED}✗${NC} SQL error leak detected in /fake-pay response!"
    FAILED=$((FAILED + 1))
else
    echo -e "  ${GREEN}✓${NC} No raw SQL error leaked"
    PASSED=$((PASSED + 1))
fi
TOTAL=$((TOTAL + 1))

# 4b. Valid UUID but non-existent booking returns 404 BOOKING_NOT_FOUND
NOT_FOUND_CODE=$(curl -s -o /tmp/dev_fp_nf.json -w "%{http_code}" -X POST "${DEV_BASE_URL}/fake-pay/ref-test?booking_id=00000000-0000-7000-8000-000000000000")
assert_eq "FakePay returns 404 for non-existent booking" "404" "$NOT_FOUND_CODE"
assert_contains "FakePay returns BOOKING_NOT_FOUND code" '"code":"BOOKING_NOT_FOUND"' "$(cat /tmp/dev_fp_nf.json)"

# 4c. Legitimate Dev FakePay Confirmation
echo -e "\n${YELLOW}>> 5. Legitimate Dev FakePay Confirmation <<${NC}"
RT="01900000-0000-7000-8000-000000000001"
CI="2026-10-25"
CO="2026-10-27"
QUOTE_RESP=$(curl -s -H "Content-Type: application/json" -d "{\"room_type_id\":\"$RT\",\"check_in\":\"$CI\",\"check_out\":\"$CO\",\"num_rooms\":1,\"num_guests\":2}" "${DEV_BASE_URL}/api/v1/quotes")
QUOTE_ID=$(echo "$QUOTE_RESP" | grep -o '"quote_id":"[^"]*' | cut -d'"' -f4)

BOOKING_PAYLOAD="{\"quote_id\":\"$QUOTE_ID\",\"terms_accepted\":true,\"privacy_accepted\":true,\"room_type_id\":\"$RT\",\"check_in\":\"$CI\",\"check_out\":\"$CO\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"R16 Tester\",\"guest_email\":\"r16@example.id\"}"
BOOKING_RESP=$(curl -s -H "Content-Type: application/json" -d "$BOOKING_PAYLOAD" "${DEV_BASE_URL}/api/v1/bookings")
BOOKING_ID=$(echo "$BOOKING_RESP" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

if [ -n "$BOOKING_ID" ]; then
    CONFIRM_CODE=$(curl -s -o /tmp/dev_fp_confirm.json -w "%{http_code}" -X POST "${DEV_BASE_URL}/fake-pay/ref-success?booking_id=${BOOKING_ID}")
    assert_eq "Legitimate fake payment returns 200 OK" "200" "$CONFIRM_CODE"
    assert_contains "Fake payment confirms booking" '"status":"confirmed"' "$(cat /tmp/dev_fp_confirm.json)"
fi

# ------------------------------------------------------------------------------
# Summary
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "   BE-R16 Results: ${TOTAL} Total | ${GREEN}${PASSED} Passed${NC} | ${RED}${FAILED} Failed${NC}"
echo -e "${BLUE}=================================================================${NC}"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
exit 0
