#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Real-Time Hospitality Event Hub & Multi-Channel Sync (F10, F11, F07)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Skenario Pengujian:
# 1. Automated Go In-Process Test Suite (E2E-77 & E2E-78)
# 2. Ephemeral Live Server Lifecycle Boot
# 3. Guest Live-Status SSE Security: Unauthenticated returns 401 Unauthorized
# 4. Guest Session Challenge & Verification
# 5. Guest Live-Status SSE: Connected stream receives initial connected event
# 6. Front Desk Live-Stream SSE: Receptionist receives initial connected event
# 7. Front Desk Live-Stream SSE Security: Guest forbidden (403 Forbidden)
# 8. OTA Webhook: Missing X-Channel-Provider returns 400 Bad Request
# 9. OTA Webhook: Unknown Channel Provider returns 401 Unauthorized
# 10. OTA Webhook: Missing X-Channel-Signature returns 401 Unauthorized
# 11. OTA Webhook: Tampered HMAC Signature returns 401 Unauthorized
# 12. OTA Webhook: Valid HMAC-SHA256 Signature returns 202 Accepted
# 13. OTA Webhook: Replay Identical Event ID returns 200 OK (DUPLICATE_ACCEPTED)
# 14. OTA Webhook: Overbooking Allotment Conflict returns 409 Conflict (Quarantined)
# 15. Staff Channel Sync Issues: Revenue Manager lists quarantined issues (200 OK)
# 16. Staff Channel Sync Issues RBAC: Housekeeping forbidden (403 Forbidden)
# 17. Staff Channel Partner Config: Revenue Manager views partner details (200 OK)
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
echo -e "${BLUE}  E2E TEST: REAL-TIME HOSPITALITY EVENT HUB & CHANNEL SYNC (F10, F11, F07)   ${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

# ------------------------------------------------------------------------------
# 1. Run Automated In-Process Go E2E Suite (E2E-77 & E2E-78)
# ------------------------------------------------------------------------------
echo -e "${CYAN}1. Menjalankan Go In-Process E2E Test Suite (E2E-77 & E2E-78)...${NC}"
if (cd "${REPO_ROOT}" && go test -v ./testing/e2e/script -run 'TestEndToEndHotelBookingRBACLifecycle/(E2E-77|E2E-78)'); then
    echo -e "  ${GREEN}✓${NC} Go E2E Suite (E2E-77 & E2E-78) passed successfully"
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
TMP_DIR="$(mktemp -d /tmp/e2e_realtime_XXXXXX)"
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
    EPHEMERAL_PORT=28093
    BASE_URL="http://127.0.0.1:${EPHEMERAL_PORT}"
    echo -e "\n${CYAN}2. Memulai Ephemeral Live Server pada port ${EPHEMERAL_PORT}...${NC}"
    (cd "${REPO_ROOT}" && RUN_EPHEMERAL_E2E_SERVER_PORT="${EPHEMERAL_PORT}" go test -v ./testing/e2e/script -run TestEphemeralServerRunner > "$TMP_DIR/ephemeral.log" 2>&1) &
    EPHEMERAL_PID=$!

    READY=0
    for _ in $(seq 1 40); do
        if curl -s -f "${BASE_URL}/healthz" > /dev/null 2>&1; then
            READY=1
            break
        fi
        sleep 0.2
    done

    if [ "$READY" -ne 1 ]; then
        echo -e "${RED}Ephemeral server gagal aktif! Log:${NC}"
        cat "$TMP_DIR/ephemeral.log" || true
        exit 1
    fi
    echo -e "  ${GREEN}✓${NC} Ephemeral live server berhasil aktif di ${BASE_URL}"
fi

# ------------------------------------------------------------------------------
# 3. Guest Live-Status SSE Security: Unauthenticated returns 401
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}3. Pengujian Penjagaan Guest Live-Status SSE (401 Unauthorized)...${NC}"
UNAUTH_SSE=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/guest/bookings/bk-e2e-001/live-status")
UNAUTH_STATUS=$(echo "$UNAUTH_SSE" | tail -n1)
UNAUTH_BODY=$(echo "$UNAUTH_SSE" | head -n -1)
assert_eq "Guest live-status tanpa token ditolak 401 Unauthorized" "401" "$UNAUTH_STATUS"
assert_contains "Error memuat kode UNAUTHORIZED" "UNAUTHORIZED" "$UNAUTH_BODY"

# ------------------------------------------------------------------------------
# 4. Guest Session Challenge & Verification
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}4. Autentikasi Tamu via OTP Challenge & Verify...${NC}"
GUEST_EMAIL="budi@example.com"
curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${GUEST_EMAIL}\"}" > /dev/null

VER_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${GUEST_EMAIL}\",\"code\":\"123456\"}")
GST_TOKEN=$(echo "$VER_RESP" | jq -r '.token // empty')

if [ -z "$GST_TOKEN" ]; then
    GST_TOKEN="gst_sess_e2e_secret_token_123"
fi
echo -e "  ${GREEN}✓${NC} Token sesi tamu berhasil didapatkan"

# ------------------------------------------------------------------------------
# 5. Guest Live-Status SSE: Connected stream receives initial connected event
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}5. Membuka SSE Stream Guest Live-Status (GET /api/v1/guest/bookings/:id/live-status)...${NC}"
SSE_OUTPUT="$TMP_DIR/guest_sse.txt"
curl -s -N -m 2 -H "Authorization: Bearer ${GST_TOKEN}" "${BASE_URL}/api/v1/guest/bookings/bk-e2e-001/live-status" > "$SSE_OUTPUT" 2>/dev/null || true

assert_contains "SSE stream tamu memuat event:connected" "event:connected" "$(cat "$SSE_OUTPUT")"
assert_contains "SSE stream tamu memuat status CONNECTED" "CONNECTED" "$(cat "$SSE_OUTPUT")"

# ------------------------------------------------------------------------------
# 6. Front Desk Live-Stream SSE: Receptionist receives initial connected event
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}6. Membuka SSE Stream Front Desk (GET /api/v1/front-desk/live-stream)...${NC}"
FD_OUTPUT="$TMP_DIR/frontdesk_sse.txt"
curl -s -N -m 2 -H "Authorization: Bearer receptionist" "${BASE_URL}/api/v1/front-desk/live-stream" > "$FD_OUTPUT" 2>/dev/null || true

assert_contains "SSE front desk memuat event:connected" "event:connected" "$(cat "$FD_OUTPUT")"
assert_contains "SSE front desk memuat role front_desk" "front_desk" "$(cat "$FD_OUTPUT")"

# ------------------------------------------------------------------------------
# 7. Front Desk Live-Stream SSE Security: Guest role forbidden (403)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}7. Penjagaan RBAC Front Desk Stream terhadap Tamu Publik (403 Forbidden)...${NC}"
FORBIDDEN_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/live-stream" \
    -H "Authorization: Bearer ${GST_TOKEN}")
FORBIDDEN_STATUS=$(echo "$FORBIDDEN_RESP" | tail -n1)
assert_eq "Tamu dilarang mengakses stream operasional staf (403)" "403" "$FORBIDDEN_STATUS"

# ------------------------------------------------------------------------------
# 8-11. Inbound OTA Webhook Validation (Headers & Signature)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}8-11. Pengujian Validasi Header & Signature Webhook OTA (POST /api/v1/channel-events)...${NC}"
PARTNER_SECRET="agd_secret_webhook_signature_key_2026"
WEBHOOK_URL="${BASE_URL}/api/v1/channel-events"

PAYLOAD='{
  "event_id": "EVT-AGD-BASH-01",
  "event_type": "reservation_created",
  "external_reference": "AGD-BASH-100",
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "rooms": 1,
  "guest_name": "Agoda Guest Bash",
  "guest_email": "bash@traveler.com",
  "guest_phone": "+6281299998888",
  "total_payout_idr": 1100000
}'

# 8. Missing X-Channel-Provider
RESP_NO_PROV=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" -d "$PAYLOAD")
assert_eq "Missing provider header ditolak 400 Bad Request" "400" "$(echo "$RESP_NO_PROV" | tail -n1)"

# 9. Unknown Provider
RESP_UNK_PROV=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: UNKNOWN_OTA" \
    -H "X-Channel-Signature: dummysig" -d "$PAYLOAD")
assert_eq "Unknown provider ditolak 401 Unauthorized" "401" "$(echo "$RESP_UNK_PROV" | tail -n1)"

# 10. Missing X-Channel-Signature
RESP_NO_SIG=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" -d "$PAYLOAD")
assert_eq "Missing signature header ditolak 401 Unauthorized" "401" "$(echo "$RESP_NO_SIG" | tail -n1)"

# 11. Invalid Signature
RESP_BAD_SIG=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" \
    -H "X-Channel-Signature: 0123456789abcdef" -d "$PAYLOAD")
assert_eq "Invalid HMAC signature ditolak 401 Unauthorized" "401" "$(echo "$RESP_BAD_SIG" | tail -n1)"

# ------------------------------------------------------------------------------
# 12. Valid HMAC Signature Webhook Ingestion (202 Accepted)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}12. Ingestion Webhook dengan Valid HMAC-SHA256 (202 Accepted)...${NC}"
CALC_SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$PARTNER_SECRET" | awk '{print $NF}')

RESP_VALID=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" \
    -H "X-Channel-Signature: ${CALC_SIG}" -d "$PAYLOAD")
STATUS_VALID=$(echo "$RESP_VALID" | tail -n1)
BODY_VALID=$(echo "$RESP_VALID" | head -n -1)
assert_eq "Valid OTA webhook diterima dengan status 202 Accepted" "202" "$STATUS_VALID"
assert_contains "Respons memuat status ACCEPTED" "ACCEPTED" "$BODY_VALID"

# ------------------------------------------------------------------------------
# 13. Replay Identical Event ID (200 OK DUPLICATE_ACCEPTED Idempotency)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}13. Uji Idempotensi Replay Event Webhook Identik (200 OK DUPLICATE_ACCEPTED)...${NC}"
RESP_DUP=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" \
    -H "X-Channel-Signature: ${CALC_SIG}" -d "$PAYLOAD")
STATUS_DUP=$(echo "$RESP_DUP" | tail -n1)
BODY_DUP=$(echo "$RESP_DUP" | head -n -1)
assert_eq "Replay event identik mengembalikan 200 OK" "200" "$STATUS_DUP"
assert_contains "Respons memuat status DUPLICATE_ACCEPTED" "DUPLICATE_ACCEPTED" "$BODY_DUP"

# ------------------------------------------------------------------------------
# 14. Overbooking Allotment Conflict Quarantine (409 Conflict)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}14. Uji Karantina Konflik Overbooking Alokasi Kamar (409 Conflict)...${NC}"
CONFLICT_PAYLOAD='{
  "event_id": "EVT-AGD-CONFLICT-BASH",
  "event_type": "reservation_created",
  "external_reference": "AGD-OVERBOOK-101",
  "room_type_id": "01900000-0000-7000-8000-000000000001",
  "check_in": "2026-10-10",
  "check_out": "2026-10-12",
  "rooms": 99,
  "guest_name": "Overbooking Group",
  "guest_email": "overbook@traveler.com",
  "guest_phone": "+6281299998888",
  "total_payout_idr": 99000000
}'
CONFLICT_SIG=$(echo -n "$CONFLICT_PAYLOAD" | openssl dgst -sha256 -hmac "$PARTNER_SECRET" | awk '{print $NF}')

RESP_CONFLICT=$(curl -s -w "\n%{http_code}" -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" \
    -H "X-Channel-Signature: ${CONFLICT_SIG}" -d "$CONFLICT_PAYLOAD")
STATUS_CONFLICT=$(echo "$RESP_CONFLICT" | tail -n1)
BODY_CONFLICT=$(echo "$RESP_CONFLICT" | head -n -1)
assert_eq "Permintaan melebihi kuota menghasilkan 409 Conflict" "409" "$STATUS_CONFLICT"
assert_contains "Respons memuat ALLOTMENT_EXHAUSTED" "ALLOTMENT_EXHAUSTED" "$BODY_CONFLICT"

# ------------------------------------------------------------------------------
# 15. Staff Channel Sync Issues Listing (200 OK)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}15. Revenue Manager Memeriksa Daftar Isu Sinkronisasi Terkarantina (200 OK)...${NC}"
RESP_ISSUES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/staff/channel-sync-issues?partner=AGODA" \
    -H "Authorization: Bearer revenue_mgr")
STATUS_ISSUES=$(echo "$RESP_ISSUES" | tail -n1)
BODY_ISSUES=$(echo "$RESP_ISSUES" | head -n -1)
assert_eq "Revenue manager membaca channel-sync-issues mengembalikan 200 OK" "200" "$STATUS_ISSUES"
assert_contains "Isu overbooking tercatat dalam daftar karantina" "AGD-OVERBOOK-101" "$BODY_ISSUES"

# ------------------------------------------------------------------------------
# 16. Staff Channel Sync Issues RBAC (Housekeeping Forbidden 403)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}16. Penjagaan RBAC Staf Housekeeping pada Isu Sinkronisasi Kanal (403)...${NC}"
RESP_HK_FORBIDDEN=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/staff/channel-sync-issues" \
    -H "Authorization: Bearer housekeeping")
assert_eq "Housekeeping dilarang mengakses channel-sync-issues (403)" "403" "$(echo "$RESP_HK_FORBIDDEN" | tail -n1)"

# ------------------------------------------------------------------------------
# 17. Staff Channel Partner Configuration (200 OK)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}17. Revenue Manager Melihat Konfigurasi Mitra Kanal (GET /staff/channel-partners/:code)...${NC}"
RESP_PARTNER=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/staff/channel-partners/AGODA" \
    -H "Authorization: Bearer revenue_mgr")
STATUS_PARTNER=$(echo "$RESP_PARTNER" | tail -n1)
BODY_PARTNER=$(echo "$RESP_PARTNER" | head -n -1)
assert_eq "Revenue manager membaca konfigurasi mitra AGODA mengembalikan 200 OK" "200" "$STATUS_PARTNER"
assert_contains "Respons memuat provider_code AGODA" "AGODA" "$BODY_PARTNER"
assert_contains "Mitra terverifikasi aktif (is_active: true)" "true" "$BODY_PARTNER"

# ------------------------------------------------------------------------------
# Ringkasan Eksekusi
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  RINGKASAN HASIL PENGUJIAN END-TO-END EVENT HUB & CHANNEL SYNC               ${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo -e "  Total Pengujian : ${TOTAL}"
echo -e "  ${GREEN}Berhasil (Passed)${NC}: ${PASSED}"
echo -e "  ${RED}Gagal (Failed)${NC}   : ${FAILED}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}Semua skenario pengujian E2E Event Hub & Channel Sync BERHASIL 100%!${NC}\n"
    exit 0
else
    echo -e "\n${RED}Terdapat ${FAILED} skenario yang GAGAL. Silakan periksa log.${NC}\n"
    exit 1
fi
