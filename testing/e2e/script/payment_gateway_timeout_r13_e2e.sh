#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Payment Gateway Timeout & Ledger Resilience (BE-R13)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar Pengujian:
# 1. Gateway Timeout (HTTP 504) -> Hold kamar tetap dipertahankan, status booking PENDING,
#    status attempt ledger "unknown_timeout", HTTP response 504 GATEWAY_TIMEOUT.
# 2. Webhook Recovery pasca-Timeout -> Webhook PAID berhasil mengonfirmasi booking
#    menjadi CONFIRMED setelah sempat mengalami timeout.
# 3. Gateway Definitive Failure (HTTP 400/4xx) -> Booking otomatis dibatalkan (CANCELLED),
#    inventaris kamar direstitusi, status attempt ledger "failed", HTTP response 502 PAYMENT_FAILED.
# 4. Verifikasi Integritas Buku Besar (Payment Attempts Ledger) -> Record attempt valid
#    dengan UUID v7, status ledger tidak hilang/swallowed.
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
WEBHOOK_SECRET="${XENDIT_WEBHOOK_TOKEN:-e2e-xendit-webhook-token}"
PG_CONN="${PG_CONN:-postgres://postgres:postgres@localhost:25432/hotel_test?sslmode=disable}"
MODE_FILE="/tmp/mock_gateway_mode.txt"

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
        echo -e "  ${RED}✗${NC} ${desc} — Did not find: '${needle}' in payload"
        echo -e "       Content: ${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}  E2E Test: BE-R13 Payment Gateway Timeout & Ledger Resilience   ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Healthcheck Service ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Service Healthcheck OK" '"status":"ok"' "$HEALTH"

RT_ID="01900000-0000-7000-8000-000000000001" # Superior King
D_CHECKIN=$(date -d "+30 days" +%Y-%m-%d)
D_CHECKOUT=$(date -d "+32 days" +%Y-%m-%d) # 2 malam

get_available_rooms() {
    local room_id="$1"
    local ci="$2"
    local co="$3"
    local res
    res=$(curl -s "${BASE_URL}/api/v1/availability?room_type_id=${room_id}&check_in=${ci}&check_out=${co}")
    echo "$res" | jq -r '.availability[0].available_rooms // empty'
}

# Ambil ketersediaan awal
INITIAL_AVAIL=$(get_available_rooms "$RT_ID" "$D_CHECKIN" "$D_CHECKOUT")
echo -e "  ${CYAN}ℹ${NC} Ketersediaan awal Superior King [${D_CHECKIN} s/d ${D_CHECKOUT}]: ${INITIAL_AVAIL} kamar"

# ------------------------------------------------------------------------------
# Skenario 1: Gateway Timeout (HTTP 504) -> Hold Dipelihara, Status Pending
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Gateway Timeout: Hold Preserved & HTTP 504 GATEWAY_TIMEOUT ---${NC}"

# Set mock gateway to return 504
echo "504" > "$MODE_FILE"

# 1.1 Buat quote
Q1_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${RT_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${D_CHECKIN}\",\"check_out\":\"${D_CHECKOUT}\",\"num_rooms\":1,\"num_guests\":2}")
Q1_ID=$(echo "$Q1_RESP" | jq -r '.quote_id')
Q1_TOTAL=$(echo "$Q1_RESP" | jq -r '.pricing.total_price_minor')

# 1.2 Checkout booking (akan mengalami gateway timeout)
B1_HTTP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{
        \"quote_id\": \"${Q1_ID}\",
        \"terms_accepted\": true,
        \"privacy_accepted\": true,
        \"room_type_id\": \"${RT_ID}\",
        \"check_in\": \"${D_CHECKIN}\",
        \"check_out\": \"${D_CHECKOUT}\",
        \"num_rooms\": 1,
        \"num_guests\": 2,
        \"guest_name\": \"Timeout Guest\",
        \"guest_email\": \"timeout.guest@example.id\"
    }")
B1_STATUS=$(echo "$B1_HTTP" | tail -n1)
B1_BODY=$(echo "$B1_HTTP" | head -n-1)

assert_eq "Checkout mengalami timeout menghasilkan HTTP 504" "504" "$B1_STATUS"
assert_contains "Error code GATEWAY_TIMEOUT" '"code":"GATEWAY_TIMEOUT"' "$B1_BODY"

# 1.3 Ambil ID booking dari database (karena API response 504 tidak mengembalikan DTO booking)
B1_ID=$(psql "$PG_CONN" -t -A -c "SELECT id FROM bookings WHERE guest_email = 'timeout.guest@example.id' ORDER BY created_at DESC LIMIT 1;")
assert_contains "Booking ID tersimpan di database Postgres" "01" "$B1_ID"

# 1.4 Verifikasi status booking: WAJIB 'pending' (TIDAK DIBATALKAN / RELEASED)
B1_DB_STATUS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM bookings WHERE id = '${B1_ID}';")
assert_eq "Status booking tetap 'pending' agar invoice gateway dapat dibayar" "pending" "$B1_DB_STATUS"

# 1.5 Verifikasi inventaris kamar: kamar tetap di-hold (-1)
AFTER_TIMEOUT_AVAIL=$(get_available_rooms "$RT_ID" "$D_CHECKIN" "$D_CHECKOUT")
EXPECTED_HELD=$((INITIAL_AVAIL - 1))
assert_eq "Inventaris kamar tetap di-hold pasca timeout" "$EXPECTED_HELD" "$AFTER_TIMEOUT_AVAIL"

# 1.6 Verifikasi buku besar payment_attempts: status 'unknown_timeout'
ATTEMPT_STATUS_1=$(psql "$PG_CONN" -t -A -c "SELECT status FROM payment_attempts WHERE booking_id = '${B1_ID}' ORDER BY created_at DESC LIMIT 1;")
assert_eq "Status attempt tercatat 'unknown_timeout'" "unknown_timeout" "$ATTEMPT_STATUS_1"

ATTEMPT_ERR_TYPE=$(psql "$PG_CONN" -t -A -c "SELECT payload->>'type' FROM payment_attempts WHERE booking_id = '${B1_ID}' ORDER BY created_at DESC LIMIT 1;")
assert_eq "Metadata attempt mencatat type 'gateway_timeout'" "gateway_timeout" "$ATTEMPT_ERR_TYPE"

# ------------------------------------------------------------------------------
# Skenario 2: Webhook Recovery Pasca Timeout -> Booking Menjadi Confirmed
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Webhook Recovery Pasca Timeout -> Confirmed ---${NC}"

# Simulasikan Xendit mengirim webhook PAID untuk booking B1 yang sempat timeout
WH_PAYLOAD="{\"id\":\"inv_mock_${B1_ID}\",\"external_id\":\"${B1_ID}\",\"status\":\"PAID\",\"amount\":${Q1_TOTAL},\"currency\":\"IDR\",\"payment_method\":\"BANK_TRANSFER\"}"
WH_HTTP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/webhooks/xendit" \
    -H "Content-Type: application/json" \
    -H "x-callback-token: ${WEBHOOK_SECRET}" \
    -d "$WH_PAYLOAD")
WH_STATUS=$(echo "$WH_HTTP" | tail -n1)

assert_eq "Webhook PAID mengembalikan HTTP 200 OK" "200" "$WH_STATUS"

# 2.1 Verifikasi status booking berubah menjadi 'confirmed'
B1_RECOVERED_STATUS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM bookings WHERE id = '${B1_ID}';")
assert_eq "Booking pulih menjadi 'confirmed' via webhook reconciliation" "confirmed" "$B1_RECOVERED_STATUS"

# 2.2 Verifikasi status payment attempt terupdate menjadi 'success'
B1_ATTEMPT_SUCCESS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM payment_attempts WHERE booking_id = '${B1_ID}' ORDER BY updated_at DESC LIMIT 1;")
assert_eq "Status payment attempt terupdate menjadi 'success'" "success" "$B1_ATTEMPT_SUCCESS"

# 2.3 Verifikasi event domain booking.confirmed tercatat di outbox
CONFIRMED_EVENT_COUNT=$(psql "$PG_CONN" -t -A -c "SELECT COUNT(*) FROM outbox WHERE topic = 'booking.confirmed' AND payload::text LIKE '%${B1_ID}%';")
assert_eq "Outbox mencatat event domain booking.confirmed" "1" "$CONFIRMED_EVENT_COUNT"

# ------------------------------------------------------------------------------
# Skenario 3: Gateway Definitive Failure (HTTP 400) -> Kompensasi & Restitusi
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Definitive Failure (HTTP 400): Compensating Cancel & Restitution ---${NC}"

# Set mock gateway to return 400 Bad Request
echo "400" > "$MODE_FILE"

# 3.1 Buat quote baru
Q2_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${RT_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${D_CHECKIN}\",\"check_out\":\"${D_CHECKOUT}\",\"num_rooms\":1,\"num_guests\":2}")
Q2_ID=$(echo "$Q2_RESP" | jq -r '.quote_id')

# 3.2 Checkout booking (akan mengalami kegagalan pasti / 400 Bad Request)
B2_HTTP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -d "{
        \"quote_id\": \"${Q2_ID}\",
        \"terms_accepted\": true,
        \"privacy_accepted\": true,
        \"room_type_id\": \"${RT_ID}\",
        \"check_in\": \"${D_CHECKIN}\",
        \"check_out\": \"${D_CHECKOUT}\",
        \"num_rooms\": 1,
        \"num_guests\": 2,
        \"guest_name\": \"Definitive Fail Guest\",
        \"guest_email\": \"failed.guest@example.id\"
    }")
B2_STATUS=$(echo "$B2_HTTP" | tail -n1)
B2_BODY=$(echo "$B2_HTTP" | head -n-1)

assert_eq "Checkout kegagalan pasti menghasilkan HTTP 502" "502" "$B2_STATUS"
assert_contains "Error code PAYMENT_FAILED" '"code":"PAYMENT_FAILED"' "$B2_BODY"

# 3.3 Ambil ID booking dari database
B2_ID=$(psql "$PG_CONN" -t -A -c "SELECT id FROM bookings WHERE guest_email = 'failed.guest@example.id' ORDER BY created_at DESC LIMIT 1;")
assert_contains "Booking ID tersimpan di database Postgres" "01" "$B2_ID"

# 3.4 Verifikasi status booking: WAJIB 'cancelled' (kompensasi pembatalan otomatis berjalan)
B2_DB_STATUS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM bookings WHERE id = '${B2_ID}';")
assert_eq "Status booking terbatalkan 'cancelled' secara otomatis" "cancelled" "$B2_DB_STATUS"

# 3.5 Verifikasi inventaris kamar: direstitusi kembali (+1)
AFTER_FAIL_AVAIL=$(get_available_rooms "$RT_ID" "$D_CHECKIN" "$D_CHECKOUT")
# Karena B1 confirmed (-1) dan B2 cancelled (kembali utuh), ketersediaan harus tetap EXPECTED_HELD
assert_eq "Inventaris kamar direstitusi utuh pasca kegagalan pasti" "$EXPECTED_HELD" "$AFTER_FAIL_AVAIL"

# 3.6 Verifikasi buku besar payment_attempts: status 'failed'
ATTEMPT_STATUS_2=$(psql "$PG_CONN" -t -A -c "SELECT status FROM payment_attempts WHERE booking_id = '${B2_ID}' ORDER BY created_at DESC LIMIT 1;")
assert_eq "Status attempt tercatat 'failed'" "failed" "$ATTEMPT_STATUS_2"

ATTEMPT_ERR_TYPE_2=$(psql "$PG_CONN" -t -A -c "SELECT payload->>'type' FROM payment_attempts WHERE booking_id = '${B2_ID}' ORDER BY created_at DESC LIMIT 1;")
assert_eq "Metadata attempt mencatat type 'definitive_failure'" "definitive_failure" "$ATTEMPT_ERR_TYPE_2"

# ------------------------------------------------------------------------------
# Skenario 4: Verifikasi Integritas Buku Besar & UUID v7 Id
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Payment Attempts Ledger Integrity & UUID v7 Check ---${NC}"

# Verifikasi kolom id pada payment_attempts tidak NULL dan berformat UUID v7
ATTEMPT_ID_COUNT=$(psql "$PG_CONN" -t -A -c "SELECT count(*) FROM payment_attempts WHERE booking_id IN ('${B1_ID}', '${B2_ID}') AND id IS NOT NULL AND id::text LIKE '01%';")
assert_eq "Semua payment attempt memiliki ID berformat UUID v7" "2" "$ATTEMPT_ID_COUNT"

# Verifikasi tidak ada error ledger yang menyebabkan inkonsistensi
TOTAL_ATTEMPTS=$(psql "$PG_CONN" -t -A -c "SELECT count(*) FROM payment_attempts WHERE booking_id IN ('${B1_ID}', '${B2_ID}');")
assert_eq "Total attempt tercatat 2 kali di buku besar" "2" "$TOTAL_ATTEMPTS"

# ------------------------------------------------------------------------------
# Hasil Akhir
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "Total Asersi: ${TOTAL}"
echo -e "Lolos (Pass): ${GREEN}${PASSED}${NC}"
echo -e "Gagal (Fail): ${RED}${FAILED}${NC}"
echo -e "${BLUE}=================================================================${NC}"

if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}E2E TEST GAGAL! Periksa log kesalahan di atas.${NC}"
    exit 1
fi

echo -e "${GREEN}SELURUH PENGUJIAN E2E BE-R13 BERHASIL DENGAN SEMPURNA! (100% PASS)${NC}\n"
