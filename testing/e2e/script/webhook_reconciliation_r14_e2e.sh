#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Webhook Ledger & Amount Reconciliation (BE-R14)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: Anti-Underpayment, Currency Verification, Invoice ID Ledger Matching,
# Stale Expiry Guard, Idempotent Webhook Replay.
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
WEBHOOK_SECRET="${XENDIT_WEBHOOK_TOKEN:-e2e-xendit-webhook-token}"

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
echo -e "${BLUE}   E2E Test: BE-R14 Webhook Ledger & Amount Reconciliation      ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# Helper untuk membuat booking hold baru
create_hold_booking() {
    local rt="01900000-0000-7000-8000-000000000001"
    local ci="2026-11-10"
    local co="2026-11-12"
    local q_resp
    q_resp=$(curl -s -H "Content-Type: application/json" -d "{\"room_type_id\":\"$rt\",\"check_in\":\"$ci\",\"check_out\":\"$co\",\"num_rooms\":1,\"num_guests\":2}" "${BASE_URL}/api/v1/quotes")
    local q_id
    q_id=$(echo "$q_resp" | grep -o '"quote_id":"[^"]*' | cut -d'"' -f4)

    local b_payload="{\"quote_id\":\"$q_id\",\"terms_accepted\":true,\"privacy_accepted\":true,\"room_type_id\":\"$rt\",\"check_in\":\"$ci\",\"check_out\":\"$co\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"R14 Tester\",\"guest_email\":\"r14@example.id\"}"
    local b_resp
    b_resp=$(curl -s -H "Content-Type: application/json" -d "$b_payload" "${BASE_URL}/api/v1/bookings")
    echo "$b_resp"
}

# ------------------------------------------------------------------------------
# Test 1: Amount Mismatch (Underpayment Protection)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 1. Anti-Underpayment: Amount Mismatch Rejection <<${NC}"

B1_RESP=$(create_hold_booking)
B1_ID=$(echo "$B1_RESP" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)
B1_TOTAL=$(echo "$B1_RESP" | grep -o '"total_price_minor":[0-9]*' | head -1 | cut -d':' -f2)
B1_REF=$(echo "$B1_RESP" | grep -o '"reference":"[^"]*' | head -1 | cut -d'"' -f4)

# Kirim nominal di bawah tagihan (misal: 10,000 IDR padahal jutaan)
PAYLOAD_UNDERPAY="{\"id\":\"$B1_REF\",\"external_id\":\"$B1_ID\",\"status\":\"PAID\",\"amount\":10000,\"currency\":\"IDR\",\"payment_method\":\"QRIS\"}"
CODE_1=$(curl -s -o /tmp/wh_res_1.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_UNDERPAY" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Underpayment webhook rejected with 422 Unprocessable Entity" "422" "$CODE_1"
assert_contains "Returns PAYMENT_AMOUNT_MISMATCH error code" '"code":"PAYMENT_AMOUNT_MISMATCH"' "$(cat /tmp/wh_res_1.json)"

# Verifikasi booking masih PENDING (tidak dikonfirmasi)
STATUS_B1=$(curl -s "${BASE_URL}/api/v1/bookings/${B1_ID}" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
assert_eq "Booking remains in pending status" "pending" "$STATUS_B1"

# ------------------------------------------------------------------------------
# Test 2: Currency Mismatch
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 2. Currency Mismatch Rejection <<${NC}"

PAYLOAD_WRONG_CURR="{\"id\":\"$B1_REF\",\"external_id\":\"$B1_ID\",\"status\":\"PAID\",\"amount\":${B1_TOTAL},\"currency\":\"USD\",\"payment_method\":\"CREDIT_CARD\"}"
CODE_2=$(curl -s -o /tmp/wh_res_2.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_WRONG_CURR" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Currency mismatch rejected with 422 Unprocessable Entity" "422" "$CODE_2"
assert_contains "Returns PAYMENT_CURRENCY_MISMATCH error code" '"code":"PAYMENT_CURRENCY_MISMATCH"' "$(cat /tmp/wh_res_2.json)"

# ------------------------------------------------------------------------------
# Test 3: Invoice ID Mismatch against Ledger (PaymentAttemptStore)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 3. Invoice ID Mismatch Rejection <<${NC}"

PAYLOAD_FORGED_INV="{\"id\":\"inv_forged_unknown_999\",\"external_id\":\"$B1_ID\",\"status\":\"PAID\",\"amount\":${B1_TOTAL},\"currency\":\"IDR\",\"payment_method\":\"BCA_VA\"}"
CODE_3=$(curl -s -o /tmp/wh_res_3.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_FORGED_INV" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Forged invoice ID rejected with 422 Unprocessable Entity" "422" "$CODE_3"
assert_contains "Returns INVOICE_ID_MISMATCH error code" '"code":"INVOICE_ID_MISMATCH"' "$(cat /tmp/wh_res_3.json)"

# ------------------------------------------------------------------------------
# Test 4: Legitimate PAID Webhook Confirmation
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 4. Legitimate PAID Webhook Confirmation <<${NC}"

PAYLOAD_VALID="{\"id\":\"$B1_REF\",\"external_id\":\"$B1_ID\",\"status\":\"PAID\",\"amount\":${B1_TOTAL},\"currency\":\"IDR\",\"payment_method\":\"BCA_VA\"}"
CODE_4=$(curl -s -o /tmp/wh_res_4.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_VALID" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Valid amount and currency webhook returns 200 OK" "200" "$CODE_4"
assert_contains "Webhook confirms booking" '"message":"booking confirmed"' "$(cat /tmp/wh_res_4.json)"

STATUS_B1_CONFIRMED=$(curl -s "${BASE_URL}/api/v1/bookings/${B1_ID}" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
assert_eq "Booking is now confirmed" "confirmed" "$STATUS_B1_CONFIRMED"

# ------------------------------------------------------------------------------
# Test 5: Idempotent Webhook Replay
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 5. Idempotent Webhook Replay <<${NC}"

CODE_5=$(curl -s -o /tmp/wh_res_5.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_VALID" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Replay on confirmed booking returns 200 OK" "200" "$CODE_5"
assert_contains "Indicates idempotent replay without error" "idempotent replay" "$(cat /tmp/wh_res_5.json)"

# ------------------------------------------------------------------------------
# Test 6: Out-of-Order EXPIRED Guard (Do not cancel confirmed booking)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 6. Out-of-Order EXPIRED Guard <<${NC}"

PAYLOAD_STALE_EXPIRED="{\"id\":\"$B1_REF\",\"external_id\":\"$B1_ID\",\"status\":\"EXPIRED\",\"amount\":${B1_TOTAL},\"currency\":\"IDR\"}"
CODE_6=$(curl -s -o /tmp/wh_res_6.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_STALE_EXPIRED" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Stale expiry on confirmed booking returns 200 OK" "200" "$CODE_6"
assert_contains "Stale expiry event is ignored" "stale expiry event ignored" "$(cat /tmp/wh_res_6.json)"

STATUS_B1_STILL_CONFIRMED=$(curl -s "${BASE_URL}/api/v1/bookings/${B1_ID}" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
assert_eq "Confirmed booking was NOT cancelled by stale expiry" "confirmed" "$STATUS_B1_STILL_CONFIRMED"

# ------------------------------------------------------------------------------
# Test 7: Legitimate EXPIRED on Pending Booking
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}>> 7. Legitimate EXPIRED on Pending Booking <<${NC}"

B2_RESP=$(create_hold_booking)
B2_ID=$(echo "$B2_RESP" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)
B2_REF=$(echo "$B2_RESP" | grep -o '"reference":"[^"]*' | head -1 | cut -d'"' -f4)

PAYLOAD_LEGIT_EXPIRED="{\"id\":\"$B2_REF\",\"external_id\":\"$B2_ID\",\"status\":\"EXPIRED\",\"amount\":1100000,\"currency\":\"IDR\"}"
CODE_7=$(curl -s -o /tmp/wh_res_7.json -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$PAYLOAD_LEGIT_EXPIRED" \
    "${BASE_URL}/api/v1/webhooks/xendit")

assert_eq "Expiry on pending booking returns 200 OK" "200" "$CODE_7"
assert_contains "Booking cancelled due to expiry" '"booking cancelled due to invoice expiry"' "$(cat /tmp/wh_res_7.json)"

STATUS_B2_CANCELLED=$(curl -s "${BASE_URL}/api/v1/bookings/${B2_ID}" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
assert_eq "Pending booking successfully cancelled" "cancelled" "$STATUS_B2_CANCELLED"

# ------------------------------------------------------------------------------
# Summary
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "   BE-R14 Results: ${TOTAL} Total | ${GREEN}${PASSED} Passed${NC} | ${RED}${FAILED} Failed${NC}"
echo -e "${BLUE}=================================================================${NC}"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
exit 0
