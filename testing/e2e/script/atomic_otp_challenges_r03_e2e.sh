#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Atomic OTP Challenges & Attempt Protection (BE-R03)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar: OWASP ASVS V2/V3, Anti-Brute-Force, Single-Use OTP, Row-Level ACID
# Menguji:
# 1. Concurrent Challenge Cooldown (Rate Limiting via PG Advisory Lock)
# 2. Strict Attempt Counter & Lockout at Max Attempts (3)
# 3. Single-Use OTP Race: 20 parallel verify requests -> exactly 1 succeeds
# 4. Anti-Replay: Verifying already consumed OTP returns 401
# 5. Session token issuance & valid profile access
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONTAINER="${PG_CONTAINER:-r03-pg}"

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
echo -e "${BLUE}   E2E Test: BE-R03 Atomic OTP Challenges & Attempt Protection   ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# 0. Health check
echo -e "\n${YELLOW}--- 0. Service Healthcheck ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Healthcheck OK" '"status":"ok"' "$HEALTH"

# ------------------------------------------------------------------------------
# Test 1: Concurrent Challenge Cooldown (Rate Limiting via PG Advisory Lock)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. Concurrent Challenge Cooldown (Rate Limiting) ---${NC}"
CD_EMAIL="guest.cooldown.$(date +%s%N)@example.com"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

CONCURRENCY_CD=5
for i in $(seq 1 $CONCURRENCY_CD); do
    (
        BODY=$(curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
            -H "Content-Type: application/json" \
            -d "{\"email\":\"${CD_EMAIL}\"}")
        echo "$BODY" > "${TMP_DIR}/cd_resp_${i}.json"
    ) &
done
wait

SUCCESS_CD_COUNT=0
RATELIMIT_CD_COUNT=0
SAMPLE_429=""

for i in $(seq 1 $CONCURRENCY_CD); do
    BODY=$(cat "${TMP_DIR}/cd_resp_${i}.json")
    if echo "$BODY" | grep -q '"status":"ok"'; then
        SUCCESS_CD_COUNT=$((SUCCESS_CD_COUNT + 1))
    elif echo "$BODY" | grep -q 'RATE_LIMIT_EXCEEDED'; then
        RATELIMIT_CD_COUNT=$((RATELIMIT_CD_COUNT + 1))
        SAMPLE_429="RATE_LIMIT_EXCEEDED"
    fi
done

assert_eq "Tepat 1 request challenge berhasil (200 OK)" "1" "$SUCCESS_CD_COUNT"
assert_eq "Request konkuren lainnya terkena cooldown (429 Too Many Requests)" "$((CONCURRENCY_CD - 1))" "$RATELIMIT_CD_COUNT"
assert_eq "Error code rate limited adalah RATE_LIMIT_EXCEEDED" "RATE_LIMIT_EXCEEDED" "$SAMPLE_429"

# ------------------------------------------------------------------------------
# Test 2: Strict Attempt Counter & Lockout at Max Attempts (3)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Strict Attempt Counter & Lockout at Max Attempts ---${NC}"
LOCK_EMAIL="guest.lock.$(date +%s%N)@example.com"

# Request challenge
curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${LOCK_EMAIL}\"}" > /dev/null

# Set OTP code to "123456" in DB
docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -c \
    "UPDATE guest_auth_challenges SET code_hash = encode(sha256('123456'::bytea), 'hex') WHERE email = '${LOCK_EMAIL}';" > /dev/null

# Attempt 1 (Wrong Code: 000001) -> 401
A1_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${LOCK_EMAIL}\",\"code\":\"000001\"}")
A1_CODE=$(echo "$A1_RESP" | tail -n 1)
A1_BODY=$(echo "$A1_RESP" | sed '$d')
assert_eq "Percobaan salah ke-1 ditolak HTTP 401" "401" "$A1_CODE"
assert_contains "Error code percobaan salah ke-1" "INVALID_OR_EXPIRED_CODE" "$A1_BODY"

# Attempt 2 (Wrong Code: 000002) -> 401
A2_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${LOCK_EMAIL}\",\"code\":\"000002\"}")
A2_CODE=$(echo "$A2_RESP" | tail -n 1)
assert_eq "Percobaan salah ke-2 ditolak HTTP 401" "401" "$A2_CODE"

# Attempt 3 (Wrong Code: 000003) -> 403 MAX_ATTEMPTS_EXCEEDED
A3_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${LOCK_EMAIL}\",\"code\":\"000003\"}")
A3_CODE=$(echo "$A3_RESP" | tail -n 1)
A3_BODY=$(echo "$A3_RESP" | sed '$d')
assert_eq "Percobaan salah ke-3 mengunci challenge HTTP 403" "403" "$A3_CODE"
assert_contains "Error code percobaan ke-3 mencapai batas" "MAX_ATTEMPTS_EXCEEDED" "$A3_BODY"

# Attempt 4 (Correct Code: 123456) -> Tetap 403 karena sudah terkunci!
A4_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${LOCK_EMAIL}\",\"code\":\"123456\"}")
A4_CODE=$(echo "$A4_RESP" | tail -n 1)
A4_BODY=$(echo "$A4_RESP" | sed '$d')
assert_eq "Kode benar setelah challenge terkunci tetap ditolak HTTP 403" "403" "$A4_CODE"
assert_contains "Error code tetap MAX_ATTEMPTS_EXCEEDED" "MAX_ATTEMPTS_EXCEEDED" "$A4_BODY"

# ------------------------------------------------------------------------------
# Test 3: Single-Use OTP Race: 20 parallel verify requests -> exactly 1 succeeds
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Single-Use OTP Race: 20 parallel verify requests ---${NC}"
RACE_EMAIL="guest.race.$(date +%s%N)@example.com"
SECRET_CODE="889900"

# Request challenge
curl -s -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${RACE_EMAIL}\"}" > /dev/null

# Set OTP code to SECRET_CODE in DB
docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -c \
    "UPDATE guest_auth_challenges SET code_hash = encode(sha256('${SECRET_CODE}'::bytea), 'hex') WHERE email = '${RACE_EMAIL}';" > /dev/null

CONCURRENCY_VERIFY=20
for i in $(seq 1 $CONCURRENCY_VERIFY); do
    (
        CODE=$(curl -s -c "${TMP_DIR}/cookie_${i}.txt" -o "${TMP_DIR}/race_resp_${i}.json" -w "%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
            -H "Content-Type: application/json" \
            -d "{\"email\":\"${RACE_EMAIL}\",\"code\":\"${SECRET_CODE}\"}")
        echo "$CODE" > "${TMP_DIR}/race_code_${i}.txt"
    ) &
done
wait

SUCCESS_VERIFY_COUNT=0
FAIL_VERIFY_COUNT=0
WINNER_INDEX=""

for i in $(seq 1 $CONCURRENCY_VERIFY); do
    C=$(cat "${TMP_DIR}/race_code_${i}.txt")
    if [ "$C" == "200" ]; then
        SUCCESS_VERIFY_COUNT=$((SUCCESS_VERIFY_COUNT + 1))
        WINNER_INDEX="$i"
    elif [ "$C" == "401" ]; then
        FAIL_VERIFY_COUNT=$((FAIL_VERIFY_COUNT + 1))
    fi
done

assert_eq "Tepat 1 request verifikasi berhasil sebagai pemenang tunggal (200 OK)" "1" "$SUCCESS_VERIFY_COUNT"
assert_eq "19 request konkuren lainnya ditolak sebagai sudah terkonsumsi (401 Unauthorized)" "$((CONCURRENCY_VERIFY - 1))" "$FAIL_VERIFY_COUNT"

# Verifikasi di database: hanya ada tepat 1 row di guest_sessions untuk RACE_EMAIL
SESS_COUNT_DB=$(docker exec "${PG_CONTAINER}" psql -U postgres -d hotel_booking -t -A -c \
    "SELECT count(*) FROM guest_sessions WHERE guest_email = '${RACE_EMAIL}';")
assert_eq "Database hanya memiliki tepat 1 sesi tamu aktif (tidak ada multiple sessions)" "1" "$SESS_COUNT_DB"

# ------------------------------------------------------------------------------
# Test 4: Replay Attack on Consumed OTP
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Anti-Replay Attack on Consumed OTP ---${NC}"
REPLAY_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${RACE_EMAIL}\",\"code\":\"${SECRET_CODE}\"}")
REPLAY_CODE=$(echo "$REPLAY_RESP" | tail -n 1)
assert_eq "Replay attack pada OTP yang sudah dipakai ditolak HTTP 401" "401" "$REPLAY_CODE"

# ------------------------------------------------------------------------------
# Test 5: Session Token Validation & Profile Access
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Session Token Validation & Profile Access ---${NC}"
WINNER_TOKEN=$(jq -r '.token // empty' "${TMP_DIR}/race_resp_${WINNER_INDEX}.json")
if [ -z "$WINNER_TOKEN" ]; then
    echo -e "  ${RED}✗ Gagal mengambil token dari respons pemenang${NC}"
    FAILED=$((FAILED + 1))
else
    ME_RESP=$(curl -s -w "\n%{http_code}" "${BASE_URL}/api/v1/auth/guest/me" \
        -H "Authorization: Bearer ${WINNER_TOKEN}")
    ME_CODE=$(echo "$ME_RESP" | tail -n 1)
    ME_BODY=$(echo "$ME_RESP" | sed '$d')
    assert_eq "Akses /api/v1/auth/guest/me dengan bearer token berhasil (200 OK)" "200" "$ME_CODE"
    assert_contains "Respons profil memuat email tamu yang diverifikasi" "${RACE_EMAIL}" "$ME_BODY"
fi

# ------------------------------------------------------------------------------
# Test 6: Invalid Code Format Validation
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 6. Invalid Code Format Validation ---${NC}"
FMT_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${RACE_EMAIL}\",\"code\":\"12\"}")
FMT_CODE=$(echo "$FMT_RESP" | tail -n 1)
assert_eq "Kode OTP kurang dari 6 digit ditolak HTTP 401" "401" "$FMT_CODE"

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}                       RINGKASAN HASIL E2E                       ${NC}"
echo -e "${BLUE}=================================================================${NC}"
echo -e "Total Assertion : $TOTAL"
echo -e "Passed          : ${GREEN}$PASSED${NC}"
echo -e "Failed          : ${RED}$FAILED${NC}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}Semua pengujian atomic OTP challenges & protection (BE-R03) SUKSES!${NC}\n"
    exit 0
else
    echo -e "\n${RED}Terdapat kegagalan pada pengujian BE-R03!${NC}\n"
    exit 1
fi
