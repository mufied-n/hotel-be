#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Payload Boundary & Unified Error Contract (BE-R18)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar Pengujian:
# 1. Batas Payload Request 1 MB:
#    - Request body > 1 MB ditolak dengan HTTP 413 Payload Too Large (PAYLOAD_TOO_LARGE).
#    - Diuji pada endpoint bookings, quotes, guest auth, dan webhook.
# 2. Validasi Header Idempotency-Key:
#    - Header Idempotency-Key > 64 karakter ditolak dengan HTTP 400 (INVALID_IDEMPOTENCY_KEY).
# 3. Penanganan JSON Rusak:
#    - Payload malformed JSON ditolak dengan HTTP 400 (INVALID_JSON).
# 4. Skema Respons Error Dual-Shape yang Kompatibel:
#    - Seluruh respons error memuat: code, error, message, detail, title, status.
# 5. Non-Regresi:
#    - Payload sah di bawah 1 MB tetap diproses normal tanpa degradasi.
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
echo -e "${BLUE}  E2E TEST: PAYLOAD BOUNDARY & UNIFIED ERROR CONTRACT (BE-R18)${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

TMP_DIR="$(mktemp -d /tmp/e2e_r18_XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Generate oversized payload (> 1 MB, misal 1.2 MB)
OVERSIZED_FILE="$TMP_DIR/oversized.json"
echo -n '{"notes":"' > "$OVERSIZED_FILE"
head -c 1200000 < /dev/zero | tr '\0' 'A' >> "$OVERSIZED_FILE"
echo -n '"}' >> "$OVERSIZED_FILE"

# ------------------------------------------------------------------------------
# 1. Skenario: POST /api/v1/bookings dengan payload > 1 MB -> HTTP 413
# ------------------------------------------------------------------------------
echo -e "${CYAN}1. Menguji batas ukuran payload pada POST /api/v1/bookings (> 1MB)...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp1.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/bookings" \
    -H "Content-Type: application/json" \
    --data-binary @"$OVERSIZED_FILE")

assert_eq "Status code untuk payload > 1MB pada /bookings adalah 413" "413" "$HTTP_STATUS"
BODY1=$(cat "$TMP_DIR/resp1.json")
assert_contains "Kode error PAYLOAD_TOO_LARGE pada /bookings" "PAYLOAD_TOO_LARGE" "$BODY1"
assert_contains "Atribut 'code' ada pada respons" '"code"' "$BODY1"
assert_contains "Atribut 'error' ada pada respons" '"error"' "$BODY1"
assert_contains "Atribut 'message' ada pada respons" '"message"' "$BODY1"
assert_contains "Atribut 'detail' ada pada respons" '"detail"' "$BODY1"
assert_contains "Atribut 'status' bernilai 413" '"status":413' "$BODY1"

# ------------------------------------------------------------------------------
# 2. Skenario: POST /api/v1/quotes dengan payload > 1 MB -> HTTP 413
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}2. Menguji batas ukuran payload pada POST /api/v1/quotes (> 1MB)...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp2.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/quotes" \
    -H "Content-Type: application/json" \
    --data-binary @"$OVERSIZED_FILE")

assert_eq "Status code untuk payload > 1MB pada /quotes adalah 413" "413" "$HTTP_STATUS"
BODY2=$(cat "$TMP_DIR/resp2.json")
assert_contains "Kode error PAYLOAD_TOO_LARGE pada /quotes" "PAYLOAD_TOO_LARGE" "$BODY2"

# ------------------------------------------------------------------------------
# 3. Skenario: POST /api/v1/auth/guest/challenge dengan payload > 1 MB -> HTTP 413
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}3. Menguji batas ukuran payload pada POST /api/v1/auth/guest/challenge (> 1MB)...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp3.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    --data-binary @"$OVERSIZED_FILE")

assert_eq "Status code untuk payload > 1MB pada /auth/guest/challenge adalah 413" "413" "$HTTP_STATUS"
BODY3=$(cat "$TMP_DIR/resp3.json")
assert_contains "Kode error PAYLOAD_TOO_LARGE pada /auth/guest/challenge" "PAYLOAD_TOO_LARGE" "$BODY3"
assert_contains "Atribut 'error' berisi PAYLOAD_TOO_LARGE pada guest auth" '"error":"PAYLOAD_TOO_LARGE"' "$BODY3"
assert_contains "Atribut 'code' berisi PAYLOAD_TOO_LARGE pada guest auth" '"code":"PAYLOAD_TOO_LARGE"' "$BODY3"

# ------------------------------------------------------------------------------
# 4. Skenario: POST /api/v1/webhooks/xendit dengan payload > 1 MB -> HTTP 413
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}4. Menguji batas ukuran payload pada POST /api/v1/webhooks/xendit (> 1MB)...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp4.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/webhooks/xendit" \
    -H "Content-Type: application/json" \
    -H "x-callback-token: test-token" \
    --data-binary @"$OVERSIZED_FILE")

assert_eq "Status code untuk payload > 1MB pada /webhooks/xendit adalah 413" "413" "$HTTP_STATUS"
BODY4=$(cat "$TMP_DIR/resp4.json")
assert_contains "Kode error PAYLOAD_TOO_LARGE pada /webhooks/xendit" "PAYLOAD_TOO_LARGE" "$BODY4"

# ------------------------------------------------------------------------------
# 5. Skenario: Header Idempotency-Key > 64 karakter -> HTTP 400 INVALID_IDEMPOTENCY_KEY
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}5. Menguji validasi panjang header Idempotency-Key (> 64 karakter)...${NC}"
LONG_KEY="key_$(head -c 70 < /dev/zero | tr '\0' 'x')"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp5.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: $LONG_KEY" \
    -d '{"quote_id":"some-quote"}')

assert_eq "Status code untuk Idempotency-Key > 64 karakter adalah 400" "400" "$HTTP_STATUS"
BODY5=$(cat "$TMP_DIR/resp5.json")
assert_contains "Kode error INVALID_IDEMPOTENCY_KEY pada header berlebih" "INVALID_IDEMPOTENCY_KEY" "$BODY5"

# ------------------------------------------------------------------------------
# 6. Skenario: Payload JSON rusak (malformed JSON) -> HTTP 400 INVALID_JSON
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}6. Menguji penanganan malformed JSON pada POST /api/v1/bookings...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp6.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d '{ "guest_name": "broken json without closing brace ')

assert_eq "Status code untuk malformed JSON pada /bookings adalah 400" "400" "$HTTP_STATUS"
BODY6=$(cat "$TMP_DIR/resp6.json")
assert_contains "Kode error INVALID_JSON pada malformed JSON" "INVALID_JSON" "$BODY6"

# ------------------------------------------------------------------------------
# 7. Skenario: Dual-shape error format pada portal tamu (invalid email)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}7. Menguji dual-shape error response pada portal tamu (/auth/guest/challenge)...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp7.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d '{"email":"invalid-email-format"}')

assert_eq "Status code untuk email invalid adalah 400" "400" "$HTTP_STATUS"
BODY7=$(cat "$TMP_DIR/resp7.json")
assert_contains "Kode error INVALID_EMAIL pada guest auth" "INVALID_EMAIL" "$BODY7"
assert_contains "Atribut 'code' ada pada respons" '"code":"INVALID_EMAIL"' "$BODY7"
assert_contains "Atribut 'error' ada pada respons" '"error":"INVALID_EMAIL"' "$BODY7"
assert_contains "Atribut 'message' ada pada respons" '"message"' "$BODY7"
assert_contains "Atribut 'detail' ada pada respons" '"detail"' "$BODY7"
assert_contains "Atribut 'status' bernilai 400" '"status":400' "$BODY7"

# ------------------------------------------------------------------------------
# 8. Skenario: Non-regresi untuk request valid di bawah 1 MB
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}8. Menguji non-regresi: request normal (< 1 MB) tetap diproses...${NC}"
HTTP_STATUS=$(curl -s -o "$TMP_DIR/resp8.json" -w "%{http_code}" -X POST "$BASE_URL/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d '{
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "rate_plan_code": "room_only",
        "check_in": "2026-11-01",
        "check_out": "2026-11-03",
        "num_rooms": 1,
        "adults": 2,
        "children": 0
    }')

assert_eq "Status code untuk request normal kuotasi adalah 200" "200" "$HTTP_STATUS"
BODY8=$(cat "$TMP_DIR/resp8.json")
assert_contains "Respons kuotasi memuat quote_id" "quote_id" "$BODY8"
assert_contains "Respons kuotasi memuat total_price_minor" "total_price_minor" "$BODY8"

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
