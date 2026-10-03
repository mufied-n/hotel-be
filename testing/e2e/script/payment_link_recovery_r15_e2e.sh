#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Payment Link Recovery Across Sessions (BE-R15)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar Pengujian:
# 1. Pemulihan Tautan Pembayaran Lintas Sesi / Perangkat (GET /bookings/:id/payment).
# 2. Idempotensi Pemulihan: Bebas Duplikasi Tagihan (Zero Duplicate Invoices).
# 3. IDOR Defense: Penolakan Akses Tanpa Token / Token Tidak Sah (HTTP 404).
# 4. Integrasi Portal Tamu Terautentikasi (GET /guest/bookings/:id memuat payment_url).
# 5. Penegakan State Machine: Penolakan Saat Hold Kedaluwarsa (HTTP 410) & Sudah Lunas (HTTP 409).
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONN="${PG_CONN:-postgres://postgres:postgres@localhost:25432/hotel_test?sslmode=disable}"
WEBHOOK_SECRET="${XENDIT_WEBHOOK_TOKEN:-e2e-xendit-webhook-token}"

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
echo -e "${BLUE}  E2E Test: BE-R15 Payment Link Recovery Across Sessions         ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Healthcheck Service ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Service Healthcheck OK" '"status":"ok"' "$HEALTH"

RT_ID="01900000-0000-7000-8000-000000000001" # Superior King
D_CHECKIN=$(date -d "+40 days" +%Y-%m-%d)
D_CHECKOUT=$(date -d "+42 days" +%Y-%m-%d) # 2 malam
GUEST_EMAIL="recovery.guest.$(date +%s%N)@example.id"

# ------------------------------------------------------------------------------
# Skenario 1: Pembuatan Booking & Pemulihan Tautan Lintas Sesi (Cross-Session)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Booking Creation & Cross-Session Payment Recovery ---${NC}"

# 1.1 Buat quote
Q1_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"${RT_ID}\",\"rate_plan_code\":\"room_only\",\"check_in\":\"${D_CHECKIN}\",\"check_out\":\"${D_CHECKOUT}\",\"num_rooms\":1,\"num_guests\":2}")
Q1_ID=$(echo "$Q1_RESP" | jq -r '.quote_id')
Q1_TOTAL=$(echo "$Q1_RESP" | jq -r '.pricing.total_price_minor')

# 1.2 Checkout booking (mendapatkan guest_token dan payment_url awal)
B1_HTTP=$(curl -s -X POST "${BASE_URL}/api/v1/bookings" \
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
        \"guest_name\": \"Recovery Guest\",
        \"guest_email\": \"${GUEST_EMAIL}\"
    }")

B1_ID=$(echo "$B1_HTTP" | jq -r '.booking.id // .id')
B1_TOKEN=$(echo "$B1_HTTP" | jq -r '.guest_access_token // .booking.guest_token // .guest_token')
INITIAL_PAY_URL=$(echo "$B1_HTTP" | jq -r '.payment_url')

assert_contains "Booking ID berhasil dibuat" "01" "$B1_ID"
assert_contains "Guest token berhasil diterbitkan" "gst_" "$B1_TOKEN"
assert_contains "Payment URL awal berhasil dikembalikan" "http" "$INITIAL_PAY_URL"

# 1.3 Simulasikan perangkat baru / browser refresh (tanpa state klien lokal)
# Tamu mengakses endpoint pemulihan dengan menyertakan X-Guest-Token
REC_RES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${B1_ID}/payment" \
    -H "X-Guest-Token: ${B1_TOKEN}")
REC_STATUS=$(echo "$REC_RES" | tail -n1)
REC_BODY=$(echo "$REC_RES" | head -n-1)

assert_eq "Pemulihan pembayaran mengembalikan HTTP 200 OK" "200" "$REC_STATUS"
assert_eq "Booking ID cocok" "$B1_ID" "$(echo "$REC_BODY" | jq -r '.booking_id')"
assert_eq "Status reservasi adalah pending" "pending" "$(echo "$REC_BODY" | jq -r '.status')"
assert_eq "Payment URL persis sama dengan invoice awal" "$INITIAL_PAY_URL" "$(echo "$REC_BODY" | jq -r '.payment_url')"
assert_eq "Total tagihan minor cocok" "$Q1_TOTAL" "$(echo "$REC_BODY" | jq -r '.amount_minor')"

# ------------------------------------------------------------------------------
# Skenario 2: Idempotensi Pemulihan (Zero Duplicate Invoices)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Idempotent Recovery: Zero Duplicate Invoices Check ---${NC}"

# Panggil pemulihan 3 kali berturut-turut
for i in {1..3}; do
    curl -s -X GET "${BASE_URL}/api/v1/bookings/${B1_ID}/payment" -H "X-Guest-Token: ${B1_TOKEN}" > /dev/null
done

# Verifikasi jumlah record percobaan pembayaran di PostgreSQL
ATTEMPT_COUNT=$(psql "$PG_CONN" -t -A -c "SELECT COUNT(*) FROM payment_attempts WHERE booking_id = '${B1_ID}';")
assert_eq "Jumlah attempt di buku besar tetap 1 (tidak ada duplikasi tagihan)" "1" "$ATTEMPT_COUNT"

# ------------------------------------------------------------------------------
# Skenario 3: IDOR Defense (Unauthorized Access Rejected)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. IDOR Defense: Unauthorized Access Rejected ---${NC}"

# 3.1 Tanpa guest token
NO_TOKEN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${B1_ID}/payment")
assert_eq "Akses tanpa token ditolak dengan HTTP 404 (IDOR Defense)" "404" "$NO_TOKEN_STATUS"

# 3.2 Dengan token milik tamu lain / token salah
WRONG_TOKEN_RES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${B1_ID}/payment" \
    -H "X-Guest-Token: gst_invalid_random_token_999")
WRONG_STATUS=$(echo "$WRONG_TOKEN_RES" | tail -n1)
WRONG_BODY=$(echo "$WRONG_TOKEN_RES" | head -n-1)

assert_eq "Akses dengan token salah ditolak dengan HTTP 404" "404" "$WRONG_STATUS"
assert_contains "Error code BOOKING_NOT_FOUND" '"code":"BOOKING_NOT_FOUND"' "$WRONG_BODY"

# ------------------------------------------------------------------------------
# Skenario 4: Integrasi Portal Tamu Terautentikasi (Guest Portal)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Authenticated Guest Portal Integration ---${NC}"

# 4.1 Request OTP challenge
CHAL_RES=$(curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${GUEST_EMAIL}\"}")
assert_contains "Challenge OTP terkirim" '"status":"ok"' "$CHAL_RES"

# 4.2 Ambil hash kode dari DB dan verifikasi menggunakan salt test atau buat sesi langsung
# Buat sesi langsung via query DB untuk kepastian pengujian portal
SESS_TOKEN="gst_sess_test_token_r15_$(date +%s)"
SESS_HASH=$(echo -n "${SESS_TOKEN}" | sha256sum | awk '{print $1}')
psql "$PG_CONN" -c "INSERT INTO guest_sessions (guest_email, token_hash, expires_at, last_active_at) VALUES ('${GUEST_EMAIL}', '${SESS_HASH}', NOW() + INTERVAL '2 hours', NOW());" > /dev/null

# 4.3 Panggil GET /api/v1/guest/bookings/:id dengan sesi terautentikasi
PORTAL_RES=$(curl -s -X GET "${BASE_URL}/api/v1/guest/bookings/${B1_ID}" \
    -H "Authorization: Bearer ${SESS_TOKEN}")

PORTAL_CAN_PAY=$(echo "$PORTAL_RES" | jq -r '.allowed_actions.can_pay')
PORTAL_PAY_URL=$(echo "$PORTAL_RES" | jq -r '.booking.payment_url')

assert_eq "Portal tamu melaporkan can_pay = true" "true" "$PORTAL_CAN_PAY"
assert_eq "Portal tamu memuat payment_url aktif" "$INITIAL_PAY_URL" "$PORTAL_PAY_URL"

# 4.4 Panggil endpoint shortcut GET /api/v1/guest/bookings/:id/payment
SHORTCUT_RES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/guest/bookings/${B1_ID}/payment" \
    -H "Authorization: Bearer ${SESS_TOKEN}")
SHORTCUT_STATUS=$(echo "$SHORTCUT_RES" | tail -n1)
SHORTCUT_BODY=$(echo "$SHORTCUT_RES" | head -n-1)

assert_eq "Shortcut portal pembayaran mengembalikan HTTP 200 OK" "200" "$SHORTCUT_STATUS"
assert_eq "Shortcut payment_url cocok" "$INITIAL_PAY_URL" "$(echo "$SHORTCUT_BODY" | jq -r '.payment_url')"

# ------------------------------------------------------------------------------
# Skenario 5: Penegakan Batas Waktu Hold & Status Non-Pending
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Hold Expiration & State Machine Guards ---${NC}"

# 5.1 Simulasikan hold telah kedaluwarsa di database
psql "$PG_CONN" -c "UPDATE bookings SET expires_at = NOW() - INTERVAL '5 minutes' WHERE id = '${B1_ID}';" > /dev/null

EXPIRED_RES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${B1_ID}/payment" \
    -H "X-Guest-Token: ${B1_TOKEN}")
EXPIRED_STATUS=$(echo "$EXPIRED_RES" | tail -n1)
EXPIRED_BODY=$(echo "$EXPIRED_RES" | head -n-1)

assert_eq "Pemulihan pada hold kedaluwarsa mengembalikan HTTP 410 Gone" "410" "$EXPIRED_STATUS"
assert_contains "Error code HOLD_EXPIRED" '"code":"HOLD_EXPIRED"' "$EXPIRED_BODY"

# Verifikasi pada portal tamu: can_pay harus false dan payment_url kosong
PORTAL_EXPIRED_RES=$(curl -s -X GET "${BASE_URL}/api/v1/guest/bookings/${B1_ID}" \
    -H "Authorization: Bearer ${SESS_TOKEN}")
PORTAL_EXP_CAN_PAY=$(echo "$PORTAL_EXPIRED_RES" | jq -r '.allowed_actions.can_pay')
PORTAL_EXP_PAY_URL=$(echo "$PORTAL_EXPIRED_RES" | jq -r '.booking.payment_url // empty')

assert_eq "can_pay menjadi false setelah hold kedaluwarsa" "false" "$PORTAL_EXP_CAN_PAY"
assert_eq "payment_url dikosongkan setelah hold kedaluwarsa" "" "$PORTAL_EXP_PAY_URL"

# 5.2 Simulasikan booking telah dikonfirmasi (CONFIRMED)
psql "$PG_CONN" -c "UPDATE bookings SET status = 'confirmed', expires_at = NOW() + INTERVAL '1 hour' WHERE id = '${B1_ID}';" > /dev/null

CONFIRMED_RES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/${B1_ID}/payment" \
    -H "X-Guest-Token: ${B1_TOKEN}")
CONFIRMED_STATUS=$(echo "$CONFIRMED_RES" | tail -n1)
CONFIRMED_BODY=$(echo "$CONFIRMED_RES" | head -n-1)

assert_eq "Pemulihan pada booking confirmed mengembalikan HTTP 409 Conflict" "409" "$CONFIRMED_STATUS"
assert_contains "Error code BOOKING_NOT_PENDING" '"code":"BOOKING_NOT_PENDING"' "$CONFIRMED_BODY"

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

echo -e "${GREEN}SELURUH PENGUJIAN E2E BE-R15 BERHASIL DENGAN SEMPURNA! (100% PASS)${NC}\n"
