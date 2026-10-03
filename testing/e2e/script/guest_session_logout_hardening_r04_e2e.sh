#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Guest Session Logout Hardening & Cookie Security (BE-R04)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: OWASP ASVS V3, Session Revocation Honesty, Private Cache-Control
# Menguji:
# 1. Private Cache-Control (no-store, private) & Pragma (no-cache)
# 2. Cookie Security: HttpOnly, SameSite=Lax, dan Secure saat HTTPS/X-Forwarded-Proto
# 3. Honest Logout: Pencabutan sesi di DB dan penghapusan cookie
# 4. Invalidation Guarantee: Token yang telah logout ditolak 401 pada seluruh endpoint
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONTAINER="${PG_CONTAINER:-r04-pg}"

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
echo -e "${BLUE}  E2E Test: BE-R04 Guest Session Logout Hardening & Cookie Sec   ${NC}"
echo -e "${BLUE}=================================================================${NC}"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Service Healthcheck ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Healthcheck OK" '"status":"ok"' "$HEALTH"

# ------------------------------------------------------------------------------
# Test 1: Verify Endpoint Security Headers & Cookie Hardening
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Verify Endpoint Security Headers & Cookie Flags ---${NC}"
TEST_EMAIL="guest.sec.$(date +%s%N)@example.com"
SECRET_CODE="771122"

# 1a. Request OTP Challenge
curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${TEST_EMAIL}\"}" > /dev/null

# 1b. Injeksi OTP code hash ke DB
docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -c \
    "UPDATE guest_auth_challenges SET code_hash = encode(sha256('${SECRET_CODE}'::bytea), 'hex') WHERE email = '${TEST_EMAIL}';" > /dev/null

# 1c. Verify OTP dengan simulasi HTTPS (X-Forwarded-Proto: https)
VERIFY_HEADER_FILE="${TMP_DIR}/verify_headers.txt"
VERIFY_RESP=$(curl -s -i -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -H "X-Forwarded-Proto: https" \
    -d "{\"email\":\"${TEST_EMAIL}\",\"code\":\"${SECRET_CODE}\"}" > "${VERIFY_HEADER_FILE}")

VERIFY_STATUS=$(grep -E "^HTTP/" "${VERIFY_HEADER_FILE}" | tail -n 1 | awk '{print $2}')
assert_eq "Status verifikasi OTP sukses (200 OK)" "200" "$VERIFY_STATUS"

# Periksa header Cache-Control dan Pragma
assert_contains "Verify response menyertakan Cache-Control: no-store" "Cache-Control: no-store, private" "$(cat "${VERIFY_HEADER_FILE}")"
assert_contains "Verify response menyertakan Pragma: no-cache" "Pragma: no-cache" "$(cat "${VERIFY_HEADER_FILE}")"

# Periksa atribut Set-Cookie
SET_COOKIE=$(grep -i "^Set-Cookie:" "${VERIFY_HEADER_FILE}" | head -n 1 || true)
assert_contains "Cookie menyertakan nama guest_session" "guest_session=" "$SET_COOKIE"
assert_contains "Cookie menyertakan flag HttpOnly" "HttpOnly" "$SET_COOKIE"
assert_contains "Cookie menyertakan flag SameSite=Lax" "SameSite=Lax" "$SET_COOKIE"
assert_contains "Cookie menyertakan flag Secure saat HTTPS aktif" "Secure" "$SET_COOKIE"

# Ambil token dari body JSON response
VERIFY_BODY=$(sed -e '1,/^\r$/d' "${VERIFY_HEADER_FILE}")
SESSION_TOKEN=$(echo "$VERIFY_BODY" | jq -r '.token // empty')
if [ -z "$SESSION_TOKEN" ]; then
    echo -e "  ${RED}✗ Gagal mengambil token sesi dari response verify${NC}"
    FAILED=$((FAILED + 1))
    exit 1
fi
echo -e "  ${GREEN}✓ Token sesi berhasil diperoleh:${NC} ${SESSION_TOKEN:0:16}..."

# ------------------------------------------------------------------------------
# Test 2: Protected Resource Private Cache Control
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Protected Resource Private Cache Control ---${NC}"
ME_HEADER_FILE="${TMP_DIR}/me_headers.txt"
curl -s -i "${BASE_URL}/api/v1/auth/guest/me" \
    -H "Authorization: Bearer ${SESSION_TOKEN}" > "${ME_HEADER_FILE}"

ME_STATUS=$(grep -E "^HTTP/" "${ME_HEADER_FILE}" | tail -n 1 | awk '{print $2}')
assert_eq "Akses /api/v1/auth/guest/me berhasil (200 OK)" "200" "$ME_STATUS"
assert_contains "/me response menyertakan Cache-Control: no-store, private" "Cache-Control: no-store, private" "$(cat "${ME_HEADER_FILE}")"
assert_contains "/me response menyertakan Pragma: no-cache" "Pragma: no-cache" "$(cat "${ME_HEADER_FILE}")"

# ------------------------------------------------------------------------------
# Test 3: Honest Logout Success & Cookie Expiry
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Honest Logout & Cookie Expiry ---${NC}"
LOGOUT_HEADER_FILE="${TMP_DIR}/logout_headers.txt"
curl -s -i -X POST "${BASE_URL}/api/v1/auth/guest/logout" \
    -H "Authorization: Bearer ${SESSION_TOKEN}" \
    -H "X-Forwarded-Proto: https" > "${LOGOUT_HEADER_FILE}"

LOGOUT_STATUS=$(grep -E "^HTTP/" "${LOGOUT_HEADER_FILE}" | tail -n 1 | awk '{print $2}')
assert_eq "Status logout sukses (200 OK)" "200" "$LOGOUT_STATUS"
assert_contains "Logout menyertakan Cache-Control: no-store, private" "Cache-Control: no-store, private" "$(cat "${LOGOUT_HEADER_FILE}")"

LOGOUT_COOKIE=$(grep -i "^Set-Cookie:" "${LOGOUT_HEADER_FILE}" | head -n 1 || true)
assert_contains "Logout menghapus cookie (Max-Age=-1 atau Max-Age=0)" "Max-Age=" "$LOGOUT_COOKIE"
assert_contains "Logout menyertakan flag HttpOnly pada penghapusan cookie" "HttpOnly" "$LOGOUT_COOKIE"
assert_contains "Logout menyertakan flag Secure pada penghapusan cookie" "Secure" "$LOGOUT_COOKIE"

# Verifikasi di Database bahwa token sesi telah benar-benar terhapus
DB_SESSION_COUNT=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c \
    "SELECT count(*) FROM guest_sessions WHERE guest_email = '${TEST_EMAIL}';")
assert_eq "Sesi terhapus permanen dari database PostgreSQL (0 sesi)" "0" "$DB_SESSION_COUNT"

# ------------------------------------------------------------------------------
# Test 4: Post-Logout Token Rejection (Session Invalidation Guarantee)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Post-Logout Token Rejection (Invalidation Guarantee) ---${NC}"

# Panggil /auth/guest/me dengan token yang telah di-revoke -> Harus 401 Unauthorized
REVOKED_ME_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/auth/guest/me" \
    -H "Authorization: Bearer ${SESSION_TOKEN}")
assert_eq "Token yang dicabut ditolak pada /auth/guest/me dengan HTTP 401" "401" "$REVOKED_ME_CODE"

# Panggil /guest/bookings dengan token yang telah di-revoke -> Harus 401 Unauthorized
REVOKED_BK_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/guest/bookings" \
    -H "Authorization: Bearer ${SESSION_TOKEN}")
assert_eq "Token yang dicabut ditolak pada /guest/bookings dengan HTTP 401" "401" "$REVOKED_BK_CODE"

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}                       RINGKASAN HASIL E2E                       ${NC}"
echo -e "${BLUE}=================================================================${NC}"
echo -e "Total Assertion : $TOTAL"
echo -e "Passed          : ${GREEN}$PASSED${NC}"
echo -e "Failed          : ${RED}$FAILED${NC}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}Semua pengujian Guest Session Logout Hardening (BE-R04) SUKSES!${NC}\n"
    exit 0
else
    echo -e "\n${RED}Terdapat kegagalan pada pengujian BE-R04!${NC}\n"
    exit 1
fi
